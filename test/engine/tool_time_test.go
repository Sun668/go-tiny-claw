package engine_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/engine"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

type hangTool struct{}

func (hangTool) Name() string { return "noop" }

func (hangTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: "noop", Description: "test"}
}

func (hangTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (hangTool) RiskLevel() approval.RiskLevel {
	return approval.RiskSafe
}

func newToolTimeEngine(t *testing.T, p provider.LLMProvider, tool tools.BaseTool, max time.Duration) *engine.AgentEngine {
	t.Helper()
	registry := tools.NewRegistry()
	registry.Register(tool)
	gate := approval.NewGate(
		approval.DefaultPolicy{},
		nil,
		approval.NewMemoryGrantStore(),
	)
	eng := engine.NewAgentEngine(p, registry, gate, false, false)
	eng.MaxToolTime = max
	return eng
}

func TestRunStopsWhenToolTimeBudgetExceeded(t *testing.T) {
	p := &usageProvider{
		usages: []schema.Usage{{PromptTokens: 1, CompletionTokens: 1}},
	}
	eng := newToolTimeEngine(t, p, hangTool{}, 30*time.Millisecond)
	session := ctxpkg.NewSession("tool-time-1", t.TempDir())
	session.Append(schema.Message{Role: schema.RoleUser, Content: "hi"})

	err := eng.Run(context.Background(), session, nil)
	if err == nil {
		t.Fatal("超过工具时间预算应失败")
	}
	if !strings.Contains(err.Error(), "工具时间预算") {
		t.Fatalf("错误信息应说明工具时间预算，实际: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("工具预算用尽后不应再打模型，实际调用: %d", p.calls)
	}
}

func TestRunSucceedsWhenToolsFinishWithinBudget(t *testing.T) {
	p := &usageProvider{
		usages: []schema.Usage{
			{PromptTokens: 1, CompletionTokens: 1},
			{PromptTokens: 1, CompletionTokens: 1},
		},
	}
	eng := newToolTimeEngine(t, p, budgetNoopTool{}, time.Second)
	session := ctxpkg.NewSession("tool-time-2", t.TempDir())
	session.Append(schema.Message{Role: schema.RoleUser, Content: "hi"})

	if err := eng.Run(context.Background(), session, nil); err != nil {
		t.Fatalf("工具在预算内结束应成功: %v", err)
	}
	if p.calls != 2 {
		t.Fatalf("应跑完工具后再打一枪，实际调用: %d", p.calls)
	}
}
