package engine

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/observability"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

func (e *AgentEngine) Run(ctx context.Context, session *ctxpkg.Session, rep reporter.Reporter) error {
	log.Printf("[Engine] 唤醒会话 [%s]，工作区: %s\n", session.ID, session.WorkDir)

	ctx, rootSpan := observability.StartSpan(ctx, "Agent.Run")
	rootSpan.AddAttribute("SessionID", session.ID)
	rootSpan.AddAttribute("WorkDir", session.WorkDir)

	defer func() {
		rootSpan.EndSpan()
		_ = observability.ExportTraceToFile(rootSpan, session.WorkDir, session.ID)
		log.Printf("📊 [Tracing] 本次任务的执行回放链路已保存至工作区的 .claw/traces 目录下\n")
	}()

	composer := ctxpkg.NewPromptComposer(session.WorkDir, e.PlanMode)
	systemMsg := composer.Build()

	e.beginRunBudget(session)
	defer e.endRunBudget()
	e.subagentMu.Lock()
	e.subagentUsed = 0
	e.subagentMu.Unlock()

	turnCount := 0

	for {
		turnCount++

		if e.MaxTurns > 0 && turnCount > e.MaxTurns {
			return fmt.Errorf("Agent 超过最大执行轮数: %d", e.MaxTurns)
		}

		turnCtx, turnSpan := observability.StartSpan(ctx, fmt.Sprintf("Turn-%d", turnCount))
		done, err := func() (bool, error) {
			defer turnSpan.EndSpan()

			availableTools := e.registry.GetAvailableTools()
			workingMemory := session.GetWorkingMemory(20)

			var contextHistory []schema.Message
			contextHistory = append(contextHistory, systemMsg)
			contextHistory = append(contextHistory, workingMemory...)
			compactedContext := e.compactor.Compact(contextHistory)

			// 用于存放本轮 Turn 合并后的内容
			var currentTurnThinkingContent string

			// ================= Phase 1: Thinking =================
			if e.EnableThinking {
				if rep != nil {
					if err := rep.OnThinking(ctx); err != nil {
						return false, err
					}
				}

				if err := e.checkBudget(session); err != nil {
					return false, err
				}

				if err := e.checkRunBudget(session, e.runStart); err != nil {
					return false, err
				}

				thinkCtx, thinkSpan := observability.StartSpan(turnCtx, "LLM.Thinking")
				thinkResp, streamed, err := e.generate(thinkCtx, compactedContext, nil, rep, true)

				defer thinkSpan.EndSpan() // 结束思考跨度
				// 汇报给用户
				if err != nil {
					return false, fmt.Errorf("Thinking 阶段失败: %w", err)
				}
				e.recordUsage(session, thinkResp)
				recordSpanUsage(thinkSpan, thinkResp)

				if thinkResp.Content != "" && rep != nil && !streamed {
					if err := rep.OnMessage(ctx, thinkResp.Content); err != nil {
						return false, err
					}
				}

				if thinkResp.Content != "" {
					// 【修改点】：思考内容暂存，先不 Append 到 session
					currentTurnThinkingContent = thinkResp.Content

					// 为了让 Phase 2 能看到刚才的思考，我们临时将其加入 contextHistory
					// 注意：这里仅用于本次 API 请求，不代表最终 Session 结构
					compactedContext = append(compactedContext, *thinkResp)
				}
			}

			// ================= Phase 2: Action =================
			if err := e.checkBudget(session); err != nil {
				return false, err
			}
			if err := e.checkRunBudget(session, e.runStart); err != nil {
				return false, err
			}
			actCtx, actSpan := observability.StartSpan(turnCtx, "LLM.Action")
			actionResp, streamed, err := e.generate(actCtx, compactedContext, availableTools, rep, true)
			defer actSpan.EndSpan() // 结束行动跨度

			if err != nil {
				return false, fmt.Errorf("Action 阶段失败: %w", err)
			}
			e.recordUsage(session, actionResp)
			recordSpanUsage(actSpan, actionResp)

			// 【核心修正】：合并 Thinking 和 Action 的内容
			// 构造一条唯一的、合规的 Assistant 消息
			finalAssistantMsg := schema.Message{
				Role:      schema.RoleAssistant,
				Content:   strings.TrimSpace(currentTurnThinkingContent + "\n" + actionResp.Content),
				ToolCalls: actionResp.ToolCalls,
			}

			// 将合并后的合规消息存入持久化 Session
			session.Append(finalAssistantMsg)

			// 汇报给用户
			if actionResp.Content != "" && rep != nil && !streamed {
				if err := rep.OnMessage(ctx, actionResp.Content); err != nil {
					if len(actionResp.ToolCalls) > 0 {
						observationMsgs := make([]schema.Message, len(actionResp.ToolCalls))
						EnsureToolObservations(
							actionResp.ToolCalls,
							observationMsgs,
							"工具调用已取消",
						)
						session.Append(observationMsgs...)
					}
					return false, err
				}
			}

			// 如果没有工具调用，结束本轮对话
			if len(actionResp.ToolCalls) == 0 {
				return true, nil
			}

			// ================= 执行工具并记录 Observation =================
			observationMsgs := make([]schema.Message, len(actionResp.ToolCalls))
			approvedCalls := make([]IndexedToolCall, 0, len(actionResp.ToolCalls))

			for i, toolCall := range actionResp.ToolCalls {
				req := approval.Request{
					ID:        approval.NewRequestID(),
					SessionID: session.ID,
					WorkDir:   session.WorkDir,
					ToolCall:  toolCall,
					Risk:      e.registry.GetRiskLevel(toolCall.Name),
				}
				decision, err := e.approvalGate.Check(ctx, req)

				if err != nil {
					// Assistant 已带 ToolCalls 入库；必须补齐 Observation，避免下一轮上下文断裂
					EnsureToolObservations(
						actionResp.ToolCalls,
						observationMsgs,
						"工具调用已取消",
					)
					session.Append(observationMsgs...)
					return false, fmt.Errorf("审批请求失败: %w", err)
				}

				if decision == approval.Deny {
					observationMsgs[i] = schema.Message{
						Role:       schema.RoleUser,
						Content:    fmt.Sprintf("工具调用被拒绝: %s", toolCall.Name),
						ToolCallID: toolCall.ID,
					}
					continue
				}

				approvedCalls = append(approvedCalls, IndexedToolCall{index: i, call: toolCall})
			}

			batch := e.runToolBatch(
				ctx,
				ctx,
				actionResp.ToolCalls,
				approvedCalls,
				observationMsgs,
				e.registry.Execute,
				e.currentToolElapsed(),
				e.addToolElapsed,
				rep,
				"",
			)
			if batch.err != nil {
				session.Append(batch.observations...)
				return false, batch.err
			}

			session.Append(batch.observations...)

			for i, result := range batch.results {
				reminderMsg := e.injector.CheckAndInject(
					actionResp.ToolCalls[i],
					result,
				)
				if reminderMsg != nil {
					session.Append(*reminderMsg)
				}
			}

			return false, nil
		}()
		if err != nil {
			return err
		}
		if done {
			break
		}
	}

	return nil
}
