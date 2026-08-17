package engine_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/engine"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

type usageProvider struct {
	usages []schema.Usage
	calls  int
}

func (p *usageProvider) Generate(
	_ context.Context,
	_ []schema.Message,
	_ []schema.ToolDefinition,
) (*schema.Message, error) {
	usage := schema.Usage{}
	if p.calls < len(p.usages) {
		usage = p.usages[p.calls]
	}
	p.calls++

	msg := &schema.Message{
		Role:    schema.RoleAssistant,
		Content: "ok",
		Usage:   &usage,
	}
	if p.calls == 1 {
		msg.ToolCalls = []schema.ToolCall{{
			ID:        "call-1",
			Name:      "noop",
			Arguments: json.RawMessage(`{}`),
		}}
	}
	return msg, nil
}

type budgetNoopTool struct{}

func (budgetNoopTool) Name() string { return "noop" }

func (budgetNoopTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: "noop", Description: "test"}
}

func (budgetNoopTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "ok", nil
}

func (budgetNoopTool) RiskLevel() approval.RiskLevel {
	return approval.RiskSafe
}

func newBudgetEngine(t *testing.T, p provider.LLMProvider, maxTokens int) *engine.AgentEngine {
	t.Helper()

	registry := tools.NewRegistry()
	registry.Register(budgetNoopTool{})

	gate := approval.NewGate(
		approval.DefaultPolicy{},
		nil,
		approval.NewMemoryGrantStore(),
	)
	eng := engine.NewAgentEngine(p, registry, gate, false, false)
	eng.MaxTokens = maxTokens
	return eng
}

func TestRunStopsBeforeNextGenerateWhenBudgetExceeded(t *testing.T) {
	p := &usageProvider{
		usages: []schema.Usage{
			{PromptTokens: 6, CompletionTokens: 4},
			{PromptTokens: 6, CompletionTokens: 4},
		},
	}
	eng := newBudgetEngine(t, p, 10)
	session := ctxpkg.NewSession("budget-1", t.TempDir())
	session.Append(schema.Message{Role: schema.RoleUser, Content: "hi"})

	err := eng.Run(context.Background(), session, nil)
	if err == nil {
		t.Fatal("超过预算应失败")
	}
	if !strings.Contains(err.Error(), "Token 预算") {
		t.Fatalf("错误信息应说明超过预算，实际: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("超预算后不应再打模型，实际调用: %d", p.calls)
	}
}

type finalUsageProvider struct {
	usage schema.Usage
	calls int
}

func (p *finalUsageProvider) Generate(
	_ context.Context,
	_ []schema.Message,
	_ []schema.ToolDefinition,
) (*schema.Message, error) {
	p.calls++
	return &schema.Message{
		Role:    schema.RoleAssistant,
		Content: "done",
		Usage:   &p.usage,
	}, nil
}

func TestRunAllowsCompletionWhenNoFurtherGenerate(t *testing.T) {
	p := &finalUsageProvider{usage: schema.Usage{PromptTokens: 6, CompletionTokens: 4}}
	eng := newBudgetEngine(t, p, 10)
	session := ctxpkg.NewSession("budget-2", t.TempDir())
	session.Append(schema.Message{Role: schema.RoleUser, Content: "hi"})

	if err := eng.Run(context.Background(), session, nil); err != nil {
		t.Fatalf("最后一轮答完即使用满预算也应成功: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("应只打 1 次模型，实际: %d", p.calls)
	}
}
