package approval

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

type TerminalApprovalHandler struct {
	reader *bufio.Reader
	out    io.Writer
}

func (h *TerminalApprovalHandler) Approve(ctx context.Context, request Request) (Decision, error) {
	fmt.Fprintf(h.out, "\n需要确认执行工具: %s\n", request.ToolCall.Name)
	fmt.Fprintf(h.out, "参数: %s\n", request.ToolCall.Arguments)
	fmt.Fprint(h.out, "[y]允许本次 [a]本会话允许 [n]拒绝: ")

	inputCh := make(chan string, 1)

	go func() {
		line, _ := h.reader.ReadString('\n')
		inputCh <- strings.TrimSpace(line)
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case input := <-inputCh:
		switch strings.ToLower(input) {
		case "y":
			return AllowOnce, nil
		case "a":
			return AllowSession, nil
		default:
			return Deny, nil
		}
	}
}

func NewTerminalApprovalHandler(reader *bufio.Reader, out io.Writer) *TerminalApprovalHandler {
	return &TerminalApprovalHandler{
		reader: reader,
		out:    out,
	}
}
