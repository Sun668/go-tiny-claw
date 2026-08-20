package engine

import (
	"context"
	"fmt"
	"time"

	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/observability"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

func (e *AgentEngine) checkBudget(session *ctxpkg.Session) error {
	if e.MaxTokens <= 0 || session == nil {
		return nil
	}
	used := session.TotalTokens()
	if used >= e.MaxTokens {
		return fmt.Errorf("已超过本会话的 Token 预算（已用 %d，上限 %d）", used, e.MaxTokens)
	}
	return nil
}

func (e *AgentEngine) checkRunBudget(session *ctxpkg.Session, runStart int) error {
	if e.MaxTokensPerRun <= 0 || session == nil {
		return nil
	}
	used := session.TotalTokens() - runStart
	if used >= e.MaxTokensPerRun {
		return fmt.Errorf("已超过本次运行的 Token 预算（已用 %d，上限 %d）", used, e.MaxTokensPerRun)
	}
	return nil
}

func (e *AgentEngine) checkSubagentBudget() error {
	if e.MaxSubagents <= 0 {
		return nil
	}
	e.subagentMu.Lock()
	defer e.subagentMu.Unlock()
	if e.subagentUsed >= e.MaxSubagents {
		return fmt.Errorf("已超过子智能体的数量上限（已用 %d，上限 %d）", e.subagentUsed, e.MaxSubagents)
	}
	e.subagentUsed++
	return nil
}

func (e *AgentEngine) beginRunBudget(session *ctxpkg.Session) {
	e.budgetMu.Lock()
	defer e.budgetMu.Unlock()
	e.budgetSession = session
	e.runStart = 0
	if session != nil {
		e.runStart = session.TotalTokens()
	}
	e.toolElapsed = 0
}

func (e *AgentEngine) endRunBudget() {
	e.budgetMu.Lock()
	defer e.budgetMu.Unlock()
	e.budgetSession = nil
	e.runStart = 0
	e.toolElapsed = 0
}

func (e *AgentEngine) currentRunBudget() (*ctxpkg.Session, int) {
	e.budgetMu.Lock()
	defer e.budgetMu.Unlock()
	return e.budgetSession, e.runStart
}

func (e *AgentEngine) currentToolElapsed() time.Duration {
	e.budgetMu.Lock()
	defer e.budgetMu.Unlock()
	return e.toolElapsed
}

func (e *AgentEngine) addToolElapsed(d time.Duration) time.Duration {
	e.budgetMu.Lock()
	defer e.budgetMu.Unlock()
	e.toolElapsed += d
	return e.toolElapsed
}

func (e *AgentEngine) toolConcurrency() int {
	if e.MaxToolConcurrency > 0 {
		return e.MaxToolConcurrency
	}
	return 8
}

func (e *AgentEngine) toolBatchContext(ctx context.Context, elapsed time.Duration) (context.Context, context.CancelFunc, error) {
	if e.MaxToolTime <= 0 {
		return ctx, func() {}, nil
	}
	remaining := e.MaxToolTime - elapsed
	if remaining <= 0 {
		return nil, nil, fmt.Errorf("已超过本次运行的工具时间预算（已用 %s，上限 %s）", elapsed, e.MaxToolTime)
	}
	ctx, cancel := context.WithTimeout(ctx, remaining)
	return ctx, cancel, nil
}

func (e *AgentEngine) recordUsage(session *ctxpkg.Session, msg *schema.Message) {
	if session == nil || msg == nil || msg.Usage == nil {
		return
	}
	session.RecordUsage(msg.Usage.PromptTokens, msg.Usage.CompletionTokens, 0)
}

func recordSpanUsage(span *observability.Span, msg *schema.Message) {
	if span == nil || msg == nil || msg.Usage == nil {
		return
	}
	span.AddAttribute("prompt_tokens", msg.Usage.PromptTokens)
	span.AddAttribute("completion_tokens", msg.Usage.CompletionTokens)
}
