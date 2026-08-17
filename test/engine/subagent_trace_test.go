package engine_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	"github.com/Sun668/go-tiny-claw/internal/observability"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

type subUsageProvider struct {
	messages []*schema.Message
	calls    int
}

func (p *subUsageProvider) Generate(
	_ context.Context,
	_ []schema.Message,
	_ []schema.ToolDefinition,
) (*schema.Message, error) {
	if p.calls >= len(p.messages) {
		return &schema.Message{
			Role:    schema.RoleAssistant,
			Content: "done",
		}, nil
	}
	msg := *p.messages[p.calls]
	p.calls++
	return &msg, nil
}

var _ provider.LLMProvider = (*subUsageProvider)(nil)

type subNoopTool struct{}

func (subNoopTool) Name() string { return "noop" }

func (subNoopTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: "noop", Description: "test"}
}

func (subNoopTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "ok", nil
}

func (subNoopTool) RiskLevel() approval.RiskLevel {
	return approval.RiskSafe
}

func findSpan(t *testing.T, root *observability.Span, name string) *observability.Span {
	t.Helper()
	if root == nil {
		t.Fatalf("找不到 Span %s：根节点为空", name)
	}
	if root.Name == name {
		return root
	}
	for _, child := range root.Children {
		if found := findSpanQuiet(child, name); found != nil {
			return found
		}
	}
	t.Fatalf("找不到 Span %s", name)
	return nil
}

func findSpanQuiet(root *observability.Span, name string) *observability.Span {
	if root == nil {
		return nil
	}
	if root.Name == name {
		return root
	}
	for _, child := range root.Children {
		if found := findSpanQuiet(child, name); found != nil {
			return found
		}
	}
	return nil
}

func assertSpanTokens(t *testing.T, span *observability.Span, prompt, completion int) {
	t.Helper()
	gotPrompt, ok := span.Attributes["prompt_tokens"].(int)
	if !ok || gotPrompt != prompt {
		t.Fatalf("%s.prompt_tokens = %v，期望 %d", span.Name, span.Attributes["prompt_tokens"], prompt)
	}
	gotCompletion, ok := span.Attributes["completion_tokens"].(int)
	if !ok || gotCompletion != completion {
		t.Fatalf("%s.completion_tokens = %v，期望 %d", span.Name, span.Attributes["completion_tokens"], completion)
	}
}

func TestRunSubRecordsTokenUsageOnActionSpan(t *testing.T) {
	p := &subUsageProvider{
		messages: []*schema.Message{{
			Role:    schema.RoleAssistant,
			Content: "found it",
			Usage:   &schema.Usage{PromptTokens: 3, CompletionTokens: 5},
		}},
	}
	eng := newStreamTestEngine(t, p)

	parentCtx, parent := observability.StartSpan(context.Background(), "Tool.Execute")
	summary, err := eng.RunSub(parentCtx, "find foo", tools.NewRegistry(), nil)
	parent.EndSpan()

	if err != nil {
		t.Fatalf("RunSub 失败: %v", err)
	}
	if summary != "found it" {
		t.Fatalf("汇报内容错误: %q", summary)
	}

	action := findSpan(t, findSpan(t, findSpan(t, parent, "Subagent.Run"), "Subagent.Turn-1"), "LLM.Action")
	assertSpanTokens(t, action, 3, 5)
}

func TestRunSubNestsToolSpanUnderTurn(t *testing.T) {
	p := &subUsageProvider{
		messages: []*schema.Message{
			{
				Role:    schema.RoleAssistant,
				Content: "searching",
				Usage:   &schema.Usage{PromptTokens: 2, CompletionTokens: 1},
				ToolCalls: []schema.ToolCall{{
					ID:        "call-1",
					Name:      "noop",
					Arguments: json.RawMessage(`{}`),
				}},
			},
			{
				Role:    schema.RoleAssistant,
				Content: "done",
				Usage:   &schema.Usage{PromptTokens: 4, CompletionTokens: 6},
			},
		},
	}
	eng := newStreamTestEngine(t, p)

	registry := tools.NewRegistry()
	registry.Register(subNoopTool{})

	parentCtx, parent := observability.StartSpan(context.Background(), "Tool.Execute")
	summary, err := eng.RunSub(parentCtx, "explore", registry, nil)
	parent.EndSpan()

	if err != nil {
		t.Fatalf("RunSub 失败: %v", err)
	}
	if summary != "done" {
		t.Fatalf("汇报内容错误: %q", summary)
	}

	run := findSpan(t, parent, "Subagent.Run")
	turn1 := findSpan(t, run, "Subagent.Turn-1")
	assertSpanTokens(t, findSpan(t, turn1, "LLM.Action"), 2, 1)
	if findSpanQuiet(turn1, "Tool.Execute") == nil {
		t.Fatal("Turn-1 下应有 Tool.Execute")
	}

	turn2 := findSpan(t, run, "Subagent.Turn-2")
	assertSpanTokens(t, findSpan(t, turn2, "LLM.Action"), 4, 6)
}
