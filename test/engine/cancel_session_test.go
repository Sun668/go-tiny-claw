package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/engine"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

type blockingApprovalHandler struct {
	entered chan struct{}
}

func (h *blockingApprovalHandler) Approve(
	ctx context.Context,
	_ approval.Request,
) (approval.Decision, error) {
	if h.entered != nil {
		select {
		case <-h.entered:
		default:
			close(h.entered)
		}
	}
	<-ctx.Done()
	return "", ctx.Err()
}

type fakeProvider struct {
	action *schema.Message
}

func (p *fakeProvider) Generate(
	_ context.Context,
	_ []schema.Message,
	availableTools []schema.ToolDefinition,
) (*schema.Message, error) {
	if len(availableTools) > 0 && p.action != nil {
		msg := *p.action
		return &msg, nil
	}
	return &schema.Message{
		Role:    schema.RoleAssistant,
		Content: "done",
	}, nil
}

var _ provider.LLMProvider = (*fakeProvider)(nil)

type fakeTool struct {
	name    string
	risk    approval.RiskLevel
	started chan struct{}
	block   bool
}

func (t *fakeTool) Name() string { return t.name }

func (t *fakeTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.name,
		Description: "test tool",
	}
}

func (t *fakeTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if t.started != nil {
		select {
		case <-t.started:
		default:
			close(t.started)
		}
	}
	if t.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return "ok", nil
}

func (t *fakeTool) RiskLevel() approval.RiskLevel { return t.risk }

func newCancelTestEngine(
	t *testing.T,
	p provider.LLMProvider,
	handler approval.Handler,
	tool tools.BaseTool,
) *engine.AgentEngine {
	t.Helper()

	reg := tools.NewRegistry()
	reg.Register(tool)

	gate := approval.NewGate(
		approval.DefaultPolicy{},
		handler,
		approval.NewMemoryGrantStore(),
	)

	return engine.NewAgentEngine(p, reg, gate, false, false)
}

func findAssistantWithToolCall(history []schema.Message, toolCallID string) *schema.Message {
	for i := range history {
		msg := &history[i]
		if msg.Role != schema.RoleAssistant {
			continue
		}
		for _, call := range msg.ToolCalls {
			if call.ID == toolCallID {
				return msg
			}
		}
	}
	return nil
}

func findObservation(history []schema.Message, toolCallID string) *schema.Message {
	for i := range history {
		msg := &history[i]
		if msg.ToolCallID == toolCallID {
			return msg
		}
	}
	return nil
}

func TestCancelDuringApprovalKeepsSessionConsistent(t *testing.T) {
	const toolCallID = "call-approve-1"

	handler := &blockingApprovalHandler{entered: make(chan struct{})}
	tool := &fakeTool{
		name: "write_file",
		risk: approval.RiskMutating,
	}
	p := &fakeProvider{
		action: &schema.Message{
			Role:    schema.RoleAssistant,
			Content: "准备写文件",
			ToolCalls: []schema.ToolCall{
				{
					ID:        toolCallID,
					Name:      "write_file",
					Arguments: json.RawMessage(`{"path":"a.txt","content":"x"}`),
				},
			},
		},
	}

	eng := newCancelTestEngine(t, p, handler, tool)
	session := ctxpkg.NewSession("cancel-approval", t.TempDir())
	session.Append(schema.Message{
		Role:    schema.RoleUser,
		Content: "写一个文件",
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- eng.Run(ctx, session, nil)
	}()

	select {
	case <-handler.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("超时：审批 Handler 未进入等待")
	}

	cancel()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled，实际: %v", err)
	}

	history := session.GetWorkingMemory(0)
	if findAssistantWithToolCall(history, toolCallID) == nil {
		t.Fatal("取消后 Session 应保留带 ToolCall 的 Assistant 消息")
	}

	observation := findObservation(history, toolCallID)
	if observation == nil {
		t.Fatal("取消后 Session 应为 ToolCall 补齐 Observation")
	}
	if observation.Content != "工具调用已取消" {
		t.Fatalf("Observation 内容错误: %q", observation.Content)
	}
}

func TestCancelDuringToolExecutionKeepsCompletedAndFillsRest(t *testing.T) {
	const (
		fastCallID = "call-fast"
		slowCallID = "call-slow"
	)

	slowStarted := make(chan struct{})
	reg := tools.NewRegistry()
	reg.Register(&fakeTool{
		name:  "read_file",
		risk:  approval.RiskSafe,
		block: false,
	})
	// 测试里将 bash 标为 Safe，跳过审批，直接进入并发执行
	reg.Register(&fakeTool{
		name:    "bash",
		risk:    approval.RiskSafe,
		started: slowStarted,
		block:   true,
	})

	p := &fakeProvider{
		action: &schema.Message{
			Role:    schema.RoleAssistant,
			Content: "先读再执行",
			ToolCalls: []schema.ToolCall{
				{
					ID:        fastCallID,
					Name:      "read_file",
					Arguments: json.RawMessage(`{}`),
				},
				{
					ID:        slowCallID,
					Name:      "bash",
					Arguments: json.RawMessage(`{"command":"sleep 30"}`),
				},
			},
		},
	}

	gate := approval.NewGate(
		approval.DefaultPolicy{},
		&blockingApprovalHandler{},
		approval.NewMemoryGrantStore(),
	)
	eng := engine.NewAgentEngine(p, reg, gate, false, false)

	session := ctxpkg.NewSession("cancel-tools", t.TempDir())
	session.Append(schema.Message{
		Role:    schema.RoleUser,
		Content: "执行两个工具",
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.Run(ctx, session, nil)
	}()

	select {
	case <-slowStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("超时：慢工具未开始执行")
	}

	time.Sleep(20 * time.Millisecond)
	cancel()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled，实际: %v", err)
	}

	history := session.GetWorkingMemory(0)

	if findAssistantWithToolCall(history, fastCallID) == nil ||
		findAssistantWithToolCall(history, slowCallID) == nil {
		t.Fatal("取消后应保留包含两个 ToolCall 的 Assistant 消息")
	}

	fastObs := findObservation(history, fastCallID)
	if fastObs == nil {
		t.Fatal("快工具应有 Observation")
	}
	if fastObs.Content != "ok" {
		t.Fatalf("快工具结果应保留，实际: %q", fastObs.Content)
	}

	slowObs := findObservation(history, slowCallID)
	if slowObs == nil {
		t.Fatal("慢工具应有 Observation，不能留下孤儿 ToolCall")
	}
	// 执行 goroutine 若已抢先写入，可能是 Recovery 的中断文案；
	// 若未写入则由 EnsureToolObservations 补「工具调用已取消」。
	if slowObs.Content != "工具调用已取消" &&
		slowObs.Content != "工具执行已中断" {
		t.Fatalf("慢工具 Observation 应表示取消/中断，实际: %q", slowObs.Content)
	}
}
