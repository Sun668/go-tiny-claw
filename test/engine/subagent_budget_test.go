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
	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

func TestRunSubStopsWhenRunBudgetExceeded(t *testing.T) {
	p := &subUsageProvider{
		messages: []*schema.Message{
			{
				Role:    schema.RoleAssistant,
				Content: "searching",
				Usage:   &schema.Usage{PromptTokens: 6, CompletionTokens: 4},
				ToolCalls: []schema.ToolCall{{
					ID:        "call-1",
					Name:      "noop",
					Arguments: json.RawMessage(`{}`),
				}},
			},
			{
				Role:    schema.RoleAssistant,
				Content: "done",
				Usage:   &schema.Usage{PromptTokens: 6, CompletionTokens: 4},
			},
		},
	}
	eng := newStreamTestEngine(t, p)
	eng.MaxTokensPerRun = 10

	registry := tools.NewRegistry()
	registry.Register(subNoopTool{})

	_, err := eng.RunSub(context.Background(), "explore", registry, nil)
	if err == nil {
		t.Fatal("子循环超过本次运行预算应失败")
	}
	if !strings.Contains(err.Error(), "本次运行的 Token 预算") {
		t.Fatalf("错误信息应说明本次运行预算，实际: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("超预算后不应再打模型，实际调用: %d", p.calls)
	}
}

func TestRunSubStopsWhenToolTimeBudgetExceeded(t *testing.T) {
	p := &subUsageProvider{
		messages: []*schema.Message{{
			Role:    schema.RoleAssistant,
			Content: "searching",
			Usage:   &schema.Usage{PromptTokens: 1, CompletionTokens: 1},
			ToolCalls: []schema.ToolCall{{
				ID:        "call-1",
				Name:      "noop",
				Arguments: json.RawMessage(`{}`),
			}},
		}},
	}
	eng := newStreamTestEngine(t, p)
	eng.MaxToolTime = 30 * time.Millisecond

	registry := tools.NewRegistry()
	registry.Register(hangTool{})

	_, err := eng.RunSub(context.Background(), "explore", registry, nil)
	if err == nil {
		t.Fatal("子循环超过工具时间预算应失败")
	}
	if !strings.Contains(err.Error(), "工具时间预算") {
		t.Fatalf("错误信息应说明工具时间预算，实际: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("工具预算用尽后不应再打模型，实际调用: %d", p.calls)
	}
}

type parentSpawnTool struct {
	eng      *engine.AgentEngine
	registry tools.Registry
}

func (t *parentSpawnTool) Name() string { return "noop" }

func (t *parentSpawnTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: "noop", Description: "test"}
}

func (t *parentSpawnTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	summary, err := t.eng.RunSub(ctx, "explore", t.registry, nil)
	if err != nil {
		return "", err
	}
	return summary, nil
}

func (t *parentSpawnTool) RiskLevel() approval.RiskLevel {
	return approval.RiskSafe
}

func TestRunCountsSubagentTokensTowardRunBudget(t *testing.T) {
	p := &usageProvider{
		usages: []schema.Usage{
			{PromptTokens: 1, CompletionTokens: 1},
			{PromptTokens: 6, CompletionTokens: 4},
			{PromptTokens: 1, CompletionTokens: 1},
		},
	}

	readOnly := tools.NewRegistry()
	readOnly.Register(subNoopTool{})

	registry := tools.NewRegistry()
	gate := approval.NewGate(
		approval.DefaultPolicy{},
		nil,
		approval.NewMemoryGrantStore(),
	)
	eng := engine.NewAgentEngine(p, registry, gate, false, false)
	eng.MaxTokensPerRun = 10
	registry.Register(&parentSpawnTool{eng: eng, registry: readOnly})

	session := ctxpkg.NewSession("sub-budget-1", t.TempDir())
	session.Append(schema.Message{Role: schema.RoleUser, Content: "hi"})

	err := eng.Run(context.Background(), session, nil)
	if err == nil {
		t.Fatal("子循环用量计入本次运行后应拦住下一枪")
	}
	if !strings.Contains(err.Error(), "本次运行的 Token 预算") {
		t.Fatalf("错误信息应说明本次运行预算，实际: %v", err)
	}
	if p.calls != 2 {
		t.Fatalf("应只打主循环一枪和子循环一枪，实际: %d", p.calls)
	}
	if session.TotalTokens() != 12 {
		t.Fatalf("子循环 Token 应记入主 Session，实际: %d", session.TotalTokens())
	}
}
