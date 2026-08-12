package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/engine"
	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

type concurrencyProbeTool struct {
	mu      sync.Mutex
	current int
	peak    int
}

func (t *concurrencyProbeTool) Name() string { return "probe" }

func (t *concurrencyProbeTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        "probe",
		Description: "concurrency probe",
	}
}

func (t *concurrencyProbeTool) RiskLevel() approval.RiskLevel {
	return approval.RiskSafe
}

func (t *concurrencyProbeTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	t.mu.Lock()
	t.current++
	if t.current > t.peak {
		t.peak = t.current
	}
	t.mu.Unlock()

	select {
	case <-ctx.Done():
		t.mu.Lock()
		t.current--
		t.mu.Unlock()
		return "", ctx.Err()
	case <-time.After(50 * time.Millisecond):
	}

	t.mu.Lock()
	t.current--
	peak := t.peak
	t.mu.Unlock()
	_ = peak
	return "ok", nil
}

func manyToolCalls(n int) *schema.Message {
	calls := make([]schema.ToolCall, n)
	for i := 0; i < n; i++ {
		calls[i] = schema.ToolCall{
			ID:        fmt.Sprintf("call-%d", i),
			Name:      "probe",
			Arguments: json.RawMessage(`{}`),
		}
	}
	return &schema.Message{
		Role:      schema.RoleAssistant,
		Content:   "并行探测",
		ToolCalls: calls,
	}
}

// oneShotProvider 第一次 Action 返回 ToolCall，之后返回无工具消息以结束循环。
type oneShotProvider struct {
	action *schema.Message
	mu     sync.Mutex
	used   bool
}

func (p *oneShotProvider) Generate(
	_ context.Context,
	_ []schema.Message,
	availableTools []schema.ToolDefinition,
) (*schema.Message, error) {
	if len(availableTools) == 0 {
		return &schema.Message{Role: schema.RoleAssistant, Content: "thinking"}, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.used {
		p.used = true
		copied := *p.action
		return &copied, nil
	}
	return &schema.Message{
		Role:    schema.RoleAssistant,
		Content: "完成",
	}, nil
}

func TestMaxToolConcurrencyIsRespected(t *testing.T) {
	probe := &concurrencyProbeTool{}
	reg := tools.NewRegistry()
	reg.Register(probe)

	p := &oneShotProvider{action: manyToolCalls(6)}
	gate := approval.NewGate(
		approval.DefaultPolicy{},
		&blockingApprovalHandler{},
		approval.NewMemoryGrantStore(),
	)

	eng := engine.NewAgentEngine(p, reg, gate, false, false)
	eng.MaxToolConcurrency = 2

	session := ctxpkg.NewSession("concurrency", t.TempDir())
	session.Append(schema.Message{
		Role:    schema.RoleUser,
		Content: "测并发",
	})

	if err := eng.Run(context.Background(), session, nil); err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	probe.mu.Lock()
	peak := probe.peak
	probe.mu.Unlock()

	if peak > 2 {
		t.Fatalf("并发峰值 %d 超过 MaxToolConcurrency=2", peak)
	}
	if peak < 1 {
		t.Fatal("工具似乎没有执行")
	}
	if peak != 2 {
		t.Fatalf("期望峰值恰好为 2（6 个调用、limit=2），实际: %d", peak)
	}
}
