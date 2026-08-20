package engine

import (
	"context"
	"fmt"
	"time"

	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/observability"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

// RunSub 是专为 Subagent 拉起的一次性受限循环。
// 它不依赖外部 Session，打完就跑。
// Reporter：为了让用户在终端看到子智能体的工作轨迹，我们将主线程的 Reporter 透传进来，并打上特殊标记。
func (e *AgentEngine) RunSub(ctx context.Context, taskPrompt string, readOnlyRegistry tools.Registry, rep reporter.Reporter) (string, error) {
	if err := e.checkSubagentBudget(); err != nil {
		return "", err
	}

	session, runStart := e.currentRunBudget()
	inParentRun := session != nil
	if !inParentRun {
		session = ctxpkg.NewSession("run-sub", "")
		runStart = 0
	}
	var localToolElapsed time.Duration
	toolElapsed := func() time.Duration {
		if inParentRun {
			return e.currentToolElapsed()
		}
		return localToolElapsed
	}
	addToolElapsed := func(d time.Duration) time.Duration {
		if inParentRun {
			return e.addToolElapsed(d)
		}
		localToolElapsed += d
		return localToolElapsed
	}

	ctx, rootSpan := observability.StartSpan(ctx, "Subagent.Run")
	defer rootSpan.EndSpan()

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

		turnCtx, turnSpan := observability.StartSpan(ctx, fmt.Sprintf("Subagent.Turn-%d", turnCount))
		summary, done, err := func() (string, bool, error) {
			defer turnSpan.EndSpan()

			// 【驾驭底线】：子智能体仅能获取传入的只读工具注册表
			availableTools := readOnlyRegistry.GetAvailableTools()

			compactedContext := e.compactor.Compact(contextHistory)

			if err := e.checkBudget(session); err != nil {
				return "", false, err
			}
			if err := e.checkRunBudget(session, runStart); err != nil {
				return "", false, err
			}

			// 子任务要求急速响应，强制关闭主体的慢思考，直接预测行动
			actCtx, actSpan := observability.StartSpan(turnCtx, "LLM.Action")
			actionResp, streamed, err := e.generate(actCtx, compactedContext, availableTools, rep, false)
			defer actSpan.EndSpan()
			if err != nil {
				return "", false, fmt.Errorf("子智能体推理失败: %w", err)
			}
			e.recordUsage(session, actionResp)
			recordSpanUsage(actSpan, actionResp)

			if actionResp.Content != "" && rep != nil && !streamed {
				if err := rep.OnMessage(ctx, actionResp.Content); err != nil {
					return "", false, err
				}
			}

			contextHistory = append(contextHistory, *actionResp)

			// 【核心退出条件】：子智能体一旦不调用工具了，说明它做好了总结汇报
			if len(actionResp.ToolCalls) == 0 {
				return actionResp.Content, true, nil
			}

			toRun := make([]IndexedToolCall, len(actionResp.ToolCalls))
			for i, call := range actionResp.ToolCalls {
				toRun[i] = IndexedToolCall{index: i, call: call}
			}

			batch := e.runToolBatch(
				ctx,
				turnCtx,
				actionResp.ToolCalls,
				toRun,
				make([]schema.Message, len(actionResp.ToolCalls)),
				readOnlyRegistry.Execute,
				toolElapsed(),
				addToolElapsed,
				rep,
				"[Subagent] ",
			)
			if batch.err != nil {
				contextHistory = append(contextHistory, batch.observations...)
				return "", false, batch.err
			}
			contextHistory = append(contextHistory, batch.observations...)
			return "", false, nil
		}()
		if err != nil {
			return "", err
		}
		if done {
			return summary, nil
		}
	}
}
