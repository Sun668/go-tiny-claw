package engine_test

import (
	"context"
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

type cancelStreamProvider struct {
	started chan struct{}
	exited  chan struct{}
}

func (p *cancelStreamProvider) Generate(
	context.Context,
	[]schema.Message,
	[]schema.ToolDefinition,
) (*schema.Message, error) {
	return nil, errors.New("不应走同步 Generate")
}

func (p *cancelStreamProvider) GenerateStream(
	ctx context.Context,
	_ []schema.Message,
	_ []schema.ToolDefinition,
) (<-chan provider.StreamEvent, error) {
	events := make(chan provider.StreamEvent)
	go func() {
		defer close(p.exited)
		defer close(events)
		close(p.started)
		<-ctx.Done()
	}()
	return events, nil
}

var _ provider.StreamingProvider = (*cancelStreamProvider)(nil)

type deltaThenHoldProvider struct {
	started chan struct{}
	exited  chan struct{}
}

func (p *deltaThenHoldProvider) Generate(
	context.Context,
	[]schema.Message,
	[]schema.ToolDefinition,
) (*schema.Message, error) {
	return nil, errors.New("不应走同步 Generate")
}

func (p *deltaThenHoldProvider) GenerateStream(
	ctx context.Context,
	_ []schema.Message,
	_ []schema.ToolDefinition,
) (<-chan provider.StreamEvent, error) {
	events := make(chan provider.StreamEvent, 1)
	go func() {
		defer close(p.exited)
		defer close(events)
		events <- provider.StreamEvent{
			Type: provider.StreamTextDelta,
			Text: "hello",
		}
		close(p.started)
		<-ctx.Done()
	}()
	return events, nil
}

var _ provider.StreamingProvider = (*deltaThenHoldProvider)(nil)

type failingStreamReporter struct {
	failingReporter
}

func (r *failingStreamReporter) OnTextDelta(context.Context, string) error {
	return errReporterClosed
}

func (r *failingStreamReporter) OnTextComplete(context.Context) error {
	return nil
}

func newStreamTestEngine(t *testing.T, p provider.LLMProvider) *engine.AgentEngine {
	t.Helper()

	gate := approval.NewGate(
		approval.DefaultPolicy{},
		&blockingApprovalHandler{},
		approval.NewMemoryGrantStore(),
	)
	return engine.NewAgentEngine(p, tools.NewRegistry(), gate, false, false)
}

func waitClosed(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal(msg)
	}
}

func TestStreamCancelExitsProviderGoroutine(t *testing.T) {
	p := &cancelStreamProvider{
		started: make(chan struct{}),
		exited:  make(chan struct{}),
	}
	eng := newStreamTestEngine(t, p)

	session := ctxpkg.NewSession("stream-cancel", t.TempDir())
	session.Append(schema.Message{
		Role:    schema.RoleUser,
		Content: "流式输出",
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.Run(ctx, session, nil)
	}()

	waitClosed(t, p.started, "超时：流式 Provider 未启动")
	cancel()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled，实际: %v", err)
	}
	waitClosed(t, p.exited, "超时：流式 Provider goroutine 未退出")
}

func TestStreamReporterErrorCancelsProviderGoroutine(t *testing.T) {
	p := &deltaThenHoldProvider{
		started: make(chan struct{}),
		exited:  make(chan struct{}),
	}
	eng := newStreamTestEngine(t, p)

	session := ctxpkg.NewSession("stream-reporter-error", t.TempDir())
	session.Append(schema.Message{
		Role:    schema.RoleUser,
		Content: "流式输出",
	})

	err := eng.Run(context.Background(), session, &failingStreamReporter{})
	if !errors.Is(err, errReporterClosed) {
		t.Fatalf("期望事件输出错误，实际: %v", err)
	}
	waitClosed(t, p.started, "超时：流式 Provider 未送出 delta")
	waitClosed(t, p.exited, "超时：Reporter 失败后 Provider goroutine 未退出")
}
