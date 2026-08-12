package channel

import (
	"context"
	"errors"
	"sync"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
)

var ErrApprovalRequestNotFound = errors.New("审批请求不存在或已结束")
var ErrApprovalAlreadyResolved = errors.New("审批请求已经响应")

type ChannelApprovalHandler struct {
	sink reporter.EventSink

	mu      sync.Mutex
	pending map[string]chan approval.Decision
}

func NewChannelApprovalHandler(sink reporter.EventSink) (*ChannelApprovalHandler, error) {
	if sink == nil {
		return nil, errors.New("审批事件输出不能为空")
	}

	return &ChannelApprovalHandler{
		sink:    sink,
		pending: make(map[string]chan approval.Decision),
	}, nil
}

func (h *ChannelApprovalHandler) Approve(
	ctx context.Context,
	request approval.Request,
) (approval.Decision, error) {
	if request.ID == "" {
		return "", errors.New("审批请求 ID 不能为空")
	}

	response := make(chan approval.Decision, 1)

	h.mu.Lock()
	h.pending[request.ID] = response
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.pending, request.ID)
		h.mu.Unlock()
	}()

	err := h.sink.Publish(ctx, reporter.Event{
		Type:      reporter.EventApprovalRequest,
		RequestID: request.ID,
		ToolName:  request.ToolCall.Name,
		Content:   string(request.ToolCall.Arguments),
		Risk:      string(request.Risk),
		Reason:    request.Reason,
	})
	if err != nil {
		return "", err
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case decision := <-response:
		return decision, nil
	}
}

func (h *ChannelApprovalHandler) Respond(
	requestID string,
	decision approval.Decision,
) error {
	if requestID == "" {
		return errors.New("审批请求 ID 不能为空")
	}

	switch decision {
	case approval.AllowOnce, approval.AllowSession, approval.Deny:
	default:
		return errors.New("不支持的审批决策")
	}

	h.mu.Lock()
	response, exists := h.pending[requestID]
	h.mu.Unlock()

	if !exists {
		return ErrApprovalRequestNotFound
	}

	select {
	case response <- decision:
		return nil
	default:
		return ErrApprovalAlreadyResolved
	}
}
