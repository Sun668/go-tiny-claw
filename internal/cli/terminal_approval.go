package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Sun668/go-tiny-claw/internal/approval"
)

var (
	ErrApprovalRequestNotFound = errors.New("审批请求不存在或已结束")
	ErrApprovalAlreadyResolved = errors.New("审批请求已经响应")
	ErrApprovalAlreadyPending  = errors.New("已有审批请求正在等待响应")
)

type TerminalApprovalHandler struct {
	out io.Writer

	mu       sync.Mutex
	response chan approval.Decision
}

func NewTerminalApprovalHandler(out io.Writer) (*TerminalApprovalHandler, error) {
	if out == nil {
		return nil, errors.New("终端输出不能为空")
	}

	return &TerminalApprovalHandler{
		out: out,
	}, nil
}

func (h *TerminalApprovalHandler) Approve(
	ctx context.Context,
	request approval.Request,
) (approval.Decision, error) {
	if request.ID == "" {
		return "", errors.New("审批请求 ID 不能为空")
	}

	response := make(chan approval.Decision, 1)

	h.mu.Lock()
	if h.response != nil {
		h.mu.Unlock()
		return "", ErrApprovalAlreadyPending
	}
	h.response = response
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		h.response = nil
		h.mu.Unlock()
	}()

	fmt.Fprintf(h.out, "\n需要确认执行工具: %s\n", request.ToolCall.Name)
	fmt.Fprintf(h.out, "参数: %s\n", request.ToolCall.Arguments)
	fmt.Fprint(h.out, "[y]允许本次 [a]本会话允许 [n]拒绝: ")

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case decision := <-response:
		return decision, nil
	}
}

func (h *TerminalApprovalHandler) Respond(decision approval.Decision) error {
	switch decision {
	case approval.AllowOnce, approval.AllowSession, approval.Deny:
	default:
		return errors.New("不支持的审批决策")
	}

	h.mu.Lock()
	response := h.response
	h.mu.Unlock()

	if response == nil {
		return ErrApprovalRequestNotFound
	}

	select {
	case response <- decision:
		return nil
	default:
		return ErrApprovalAlreadyResolved
	}
}

func (h *TerminalApprovalHandler) HasPending() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.response != nil
}
