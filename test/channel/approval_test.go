package channel_test

import (
	"context"
	"testing"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	"github.com/Sun668/go-tiny-claw/internal/channel"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type eventSink struct {
	events chan reporter.Event
}

func (s *eventSink) Publish(_ context.Context, event reporter.Event) error {
	s.events <- event
	return nil
}

func TestChannelApprovalHandlerRoutesResponse(t *testing.T) {
	sink := &eventSink{events: make(chan reporter.Event, 1)}
	handler, err := channel.NewChannelApprovalHandler(sink)
	if err != nil {
		t.Fatalf("创建通道审批处理器失败: %v", err)
	}

	request := approval.Request{
		ID: "request-1",
		ToolCall: schema.ToolCall{
			Name:      "write_file",
			Arguments: []byte(`{"path":"a.txt"}`),
		},
		Risk: approval.RiskMutating,
	}

	decisionCh := make(chan approval.Decision, 1)
	errorCh := make(chan error, 1)
	go func() {
		decision, err := handler.Approve(context.Background(), request)
		decisionCh <- decision
		errorCh <- err
	}()

	event := <-sink.events
	if event.Type != reporter.EventApprovalRequest || event.RequestID != request.ID {
		t.Fatalf("审批事件错误: %+v", event)
	}

	if err := handler.Respond(request.ID, approval.AllowOnce); err != nil {
		t.Fatalf("发送审批响应失败: %v", err)
	}

	if decision := <-decisionCh; decision != approval.AllowOnce {
		t.Fatalf("审批决策错误: %s", decision)
	}

	if err := <-errorCh; err != nil {
		t.Fatalf("审批处理失败: %v", err)
	}
}
