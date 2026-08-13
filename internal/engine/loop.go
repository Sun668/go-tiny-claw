package engine

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/observability"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

type AgentEngine struct {
	provider           provider.LLMProvider
	registry           tools.Registry
	EnableThinking     bool
	PlanMode           bool // 【新增】计划模式开关
	compactor          *ctxpkg.Compactor
	recovery           *RecoveryManager
	injector           *ReminderInjector
	MaxTurns           int
	approvalGate       *approval.Gate
	MaxToolConcurrency int
}

type IndexedToolCall struct {
	index int
	call  schema.ToolCall
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, approvalGate *approval.Gate, enableThinking bool, planMode bool) *AgentEngine {
	return &AgentEngine{
		provider:           p,
		registry:           r,
		EnableThinking:     enableThinking,
		PlanMode:           planMode,
		compactor:          ctxpkg.NewCompactor(20000, 6),
		recovery:           NewRecoveryManager(),
		injector:           NewReminderInjector(),
		MaxTurns:           20,
		approvalGate:       approvalGate,
		MaxToolConcurrency: 8,
	}
}

// internal/engine/loop.go

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
					rep.OnThinking(ctx)
				}

				thinkCtx, thinkSpan := observability.StartSpan(turnCtx, "LLM.Thinking")
				thinkResp, streamed, err := e.generate(thinkCtx, compactedContext, nil, rep, true)
				// 汇报给用户
				if thinkResp.Content != "" && rep != nil && !streamed {
					rep.OnMessage(ctx, thinkResp.Content)
				}
				thinkSpan.EndSpan() // 结束思考跨度

				if err != nil {
					return false, fmt.Errorf("Thinking 阶段失败: %w", err)
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
			actCtx, actSpan := observability.StartSpan(turnCtx, "LLM.Action")
			actionResp, streamed, err := e.generate(actCtx, compactedContext, availableTools, rep, true)
			actSpan.EndSpan() // 结束行动跨度

			if err != nil {
				return false, fmt.Errorf("Action 阶段失败: %w", err)
			}

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
				rep.OnMessage(ctx, actionResp.Content)
			}

			// 如果没有工具调用，结束本轮对话
			if len(actionResp.ToolCalls) == 0 {
				return true, nil
			}

			// ================= 执行工具并记录 Observation =================
			observationMsgs := make([]schema.Message, len(actionResp.ToolCalls))
			approvedCalls := make([]IndexedToolCall, 0, len(actionResp.ToolCalls))
			toolResults := make([]schema.ToolResult, len(actionResp.ToolCalls))
			var wg sync.WaitGroup

			for i, toolCall := range actionResp.ToolCalls {
				req := approval.Request{
					ID:        approval.NewRequestID(),
					SessionID: session.ID,
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

			limit := e.MaxToolConcurrency
			if limit <= 0 {
				limit = 8
			}

			sem := make(chan struct{}, limit)

			for _, item := range approvedCalls {
				wg.Add(1)
				go func(item IndexedToolCall) {
					defer wg.Done()

					select {
					case <-ctx.Done():
						observationMsgs[item.index] = schema.Message{
							Role:       schema.RoleUser,
							Content:    "工具调用已取消",
							ToolCallID: item.call.ID,
						}
						return
					case sem <- struct{}{}:
						defer func() {
							<-sem
						}()
					}

					if rep != nil {
						rep.OnToolCall(ctx, item.call.Name, string(item.call.Arguments))
					}
					result := e.registry.Execute(ctx, item.call)

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
						rep.OnToolResult(ctx, item.call.Name, displayOutput, result.IsError)
					}

					observationMsgs[item.index] = schema.Message{
						Role:       schema.RoleUser,
						Content:    finalOutput,
						ToolCallID: item.call.ID,
					}

					toolResults[item.index] = schema.ToolResult{
						ToolCallID: result.ToolCallID,
						Output:     finalOutput,
						IsError:    result.IsError,
					}
				}(item)
			}

			wg.Wait()

			// 用户取消或 Run 级超时：保留已完成结果，未完成的补取消 Observation，保持 ToolCall 成对
			if err := ctx.Err(); err != nil {
				EnsureToolObservations(
					actionResp.ToolCalls,
					observationMsgs,
					"工具调用已取消",
				)
				session.Append(observationMsgs...)
				return false, err
			}

			// 工具级超时/失败：作为 Observation 写入，Run 继续
			session.Append(observationMsgs...)

			for i, result := range toolResults {
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

// internal/engine/loop.go (续加在末尾)

// RunSub 是专为 Subagent 拉起的一次性受限循环。
// 它不依赖外部 Session，打完就跑。
// Reporter：为了让用户在终端看到子智能体的工作轨迹，我们将主线程的 Reporter 透传进来，并打上特殊标记。
func (e *AgentEngine) RunSub(ctx context.Context, taskPrompt string, readOnlyRegistry tools.Registry, rep reporter.Reporter) (string, error) {

	// 【核心优化】：子智能体极其容易偷懒。我们必须在 System Prompt 中严厉警告它必须使用工具！
	contextHistory := []schema.Message{
		{
			Role: schema.RoleSystem,
			Content: `你是一个专门负责深度探索的探路者 (Explorer Subagent)。
你的任务是根据主架构师的指令，在当前工作区内仔细阅读代码、查阅日志，搜集足够的信息。

【核心纪律】
1. 你必须、且只能依靠内置工具（如 bash 的 find/grep，或 read_file）去寻找答案。绝对不允许凭空捏造或猜测！
2. 如果你没有找到确切的答案，你必须继续使用工具深入搜索。
3. 当且仅当你找到了确切的线索后，停止调用工具，直接输出一段纯文本作为你的终极汇报。主架构师会根据你的汇报来做下一步决策。`,
		},
		{
			Role:    schema.RoleUser,
			Content: taskPrompt,
		},
	}

	// 限制子智能体最多只能跑 10 个 Turn，防止它自己卡死
	const maxSubTurns = 10
	turnCount := 0

	for {
		turnCount++
		if turnCount > maxSubTurns {
			return "", fmt.Errorf("子智能体探索过于深入，超过 %d 轮被强制召回，请主 Agent 给它更明确的指令", maxSubTurns)
		}

		// 【驾驭底线】：子智能体仅能获取传入的只读工具注册表
		availableTools := readOnlyRegistry.GetAvailableTools()

		compactedContext := e.compactor.Compact(contextHistory)

		// 子任务要求急速响应，强制关闭主体的慢思考，直接预测行动
		actionResp, streamed, err := e.generate(ctx, compactedContext, availableTools, rep, false)
		if err != nil {
			return "", fmt.Errorf("子智能体推理失败: %w", err)
		}

		if actionResp.Content != "" && rep != nil && !streamed {
			rep.OnMessage(ctx, actionResp.Content)
		}

		contextHistory = append(contextHistory, *actionResp)

		// 【核心退出条件】：子智能体一旦不调用工具了，说明它做好了总结汇报
		if len(actionResp.ToolCalls) == 0 {
			// 直接将它的这段汇报内容剥离出来返回给上层
			return actionResp.Content, nil
		}

		// 执行只读工具的并发循环
		observationMsgs := make([]schema.Message, len(actionResp.ToolCalls))
		var wg sync.WaitGroup

		for i, toolCall := range actionResp.ToolCalls {
			wg.Add(1)
			go func(idx int, call schema.ToolCall) {
				defer wg.Done()

				// 【可视化的关键】：让终端用户看到 Subagent 正在干嘛
				if rep != nil {
					rep.OnToolCall(ctx, fmt.Sprintf("[Subagent] %s", call.Name), string(call.Arguments))
				}

				result := readOnlyRegistry.Execute(ctx, call)

				finalOutput := result.Output
				if result.IsError {
					finalOutput = e.recovery.AnalyzeAndInject(call.Name, result.Output)
				}

				if rep != nil {
					display := finalOutput
					if len(display) > 200 {
						display = display[:200] + "... (已截断)"
					}
					rep.OnToolResult(ctx, fmt.Sprintf("[Subagent] %s", call.Name), display, result.IsError)
				}

				observationMsgs[idx] = schema.Message{
					Role:       schema.RoleUser,
					Content:    finalOutput,
					ToolCallID: call.ID,
				}
			}(i, toolCall)
		}

		wg.Wait()
		if err := ctx.Err(); err != nil {
			return "", err
		}
		contextHistory = append(contextHistory, observationMsgs...)
	}
}

// EnsureToolObservations 为尚未写入的 ToolCall 补齐 Observation。
// 用于取消/中断路径：Assistant 消息若已带 ToolCalls 入库，必须成对补全，避免下一轮上下文断裂。
func EnsureToolObservations(
	toolCalls []schema.ToolCall,
	observationMsgs []schema.Message,
	content string,
) {
	for i, toolCall := range toolCalls {
		if i >= len(observationMsgs) {
			return
		}
		if observationMsgs[i].ToolCallID != "" {
			continue
		}
		observationMsgs[i] = schema.Message{
			Role:       schema.RoleUser,
			Content:    content,
			ToolCallID: toolCall.ID,
		}
	}
}
