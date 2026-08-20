package engine

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/reporter"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type toolBatchResult struct {
	observations []schema.Message
	results      []schema.ToolResult
	elapsed      time.Duration
	err          error
}

func (e *AgentEngine) runToolBatch(
	parentCtx context.Context,
	batchCtx context.Context,
	allCalls []schema.ToolCall,
	toRun []IndexedToolCall,
	observations []schema.Message,
	exec func(ctx context.Context, call schema.ToolCall) schema.ToolResult,
	elapsedSoFar time.Duration,
	addElapsed func(time.Duration) time.Duration,
	rep reporter.Reporter,
	namePrefix string,
) toolBatchResult {
	if observations == nil {
		observations = make([]schema.Message, len(allCalls))
	}
	results := make([]schema.ToolResult, len(allCalls))

	toolCtx, toolCancel, err := e.toolBatchContext(batchCtx, elapsedSoFar)
	if err != nil {
		EnsureToolObservations(allCalls, observations, "已超过本次运行的工具时间预算")
		return toolBatchResult{observations: observations, results: results, err: err}
	}
	defer toolCancel()

	limit := e.toolConcurrency()
	batchStart := time.Now()

	var wg sync.WaitGroup
	sem := make(chan struct{}, limit)

	var (
		reportMu  sync.Mutex
		reportErr error
	)
	setReportErr := func(err error) {
		if err == nil {
			return
		}
		reportMu.Lock()
		defer reportMu.Unlock()
		if reportErr == nil {
			reportErr = err
		}
	}

	for _, item := range toRun {
		wg.Add(1)
		go func(item IndexedToolCall) {
			defer wg.Done()

			select {
			case <-toolCtx.Done():
				observations[item.index] = schema.Message{
					Role:       schema.RoleUser,
					Content:    "工具调用已取消",
					ToolCallID: item.call.ID,
				}
				return
			case sem <- struct{}{}:
				defer func() { <-sem }()
			}

			displayName := item.call.Name
			if namePrefix != "" {
				displayName = namePrefix + item.call.Name
			}

			if rep != nil {
				if err := rep.OnToolCall(toolCtx, displayName, string(item.call.Arguments)); err != nil {
					setReportErr(err)
					observations[item.index] = schema.Message{
						Role:       schema.RoleUser,
						Content:    "工具调用已取消",
						ToolCallID: item.call.ID,
					}
					return
				}
			}

			result := exec(toolCtx, item.call)

			finalOutput := result.Output
			if result.IsError {
				finalOutput = e.recovery.AnalyzeAndInject(item.call.Name, finalOutput)
				log.Printf(" -> [Go-%d] ❌ 注入救援指南: %s\n", item.index, finalOutput)
			} else {
				log.Printf(" -> [Go-%d] ✅ 工具执行成功 (返回 %d 字节)\n", item.index, len(result.Output))
			}

			if rep != nil {
				displayOutput := result.Output
				if len(displayOutput) > 200 {
					displayOutput = displayOutput[:200] + "... (已截断)"
				}
				if err := rep.OnToolResult(toolCtx, displayName, displayOutput, result.IsError); err != nil {
					setReportErr(err)
				}
			}

			observations[item.index] = schema.Message{
				Role:       schema.RoleUser,
				Content:    finalOutput,
				ToolCallID: item.call.ID,
			}
			results[item.index] = schema.ToolResult{
				ToolCallID: result.ToolCallID,
				Output:     finalOutput,
				IsError:    result.IsError,
			}
		}(item)
	}

	wg.Wait()
	elapsed := addElapsed(time.Since(batchStart))

	if err := parentCtx.Err(); err != nil {
		EnsureToolObservations(allCalls, observations, "工具调用已取消")
		return toolBatchResult{observations: observations, results: results, elapsed: elapsed, err: err}
	}
	if reportErr != nil {
		EnsureToolObservations(allCalls, observations, "工具调用已取消")
		return toolBatchResult{observations: observations, results: results, elapsed: elapsed, err: reportErr}
	}
	if err := toolCtx.Err(); err != nil {
		EnsureToolObservations(allCalls, observations, "已超过本次运行的工具时间预算")
		return toolBatchResult{
			observations: observations,
			results:      results,
			elapsed:      elapsed,
			err:          fmt.Errorf("已超过本次运行的工具时间预算（已用 %s，上限 %s）", elapsed, e.MaxToolTime),
		}
	}

	return toolBatchResult{observations: observations, results: results, elapsed: elapsed}
}
