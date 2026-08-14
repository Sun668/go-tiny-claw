package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

func TestBashToolTimeoutReturnsObservationError(t *testing.T) {
	tool := tools.NewBashToolWithTimeout(t.TempDir(), 50*time.Millisecond)

	output, err := tool.Execute(
		context.Background(),
		json.RawMessage(`{"command":"sleep 2"}`),
	)

	if !errors.Is(err, tools.ErrToolTimeout) {
		t.Fatalf("期望 ErrToolTimeout，实际: %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("工具超时不能是 context.DeadlineExceeded")
	}
	if !strings.Contains(output, "超时") {
		t.Fatalf("期望警告写入返回文案，实际: %q", output)
	}

	registry := tools.NewRegistry()
	registry.Register(tool)

	result := registry.Execute(context.Background(), schema.ToolCall{
		ID:        "call-1",
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":"sleep 2"}`),
	})

	if !result.IsError {
		t.Fatal("Registry 应将工具超时标记为 IsError Observation")
	}
	if !strings.Contains(result.Output, "超时") {
		t.Fatalf("Observation 应包含超时警告，实际: %q", result.Output)
	}
}

func TestBashToolParentCancelBubbles(t *testing.T) {
	tool := tools.NewBashToolWithTimeout(t.TempDir(), time.Minute)

	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		close(started)
		_, err := tool.Execute(ctx, json.RawMessage(`{"command":"sleep 30"}`))
		errCh <- err
	}()

	<-started
	time.Sleep(20 * time.Millisecond)
	cancel()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("父 ctx 取消应冒泡 context.Canceled，实际: %v", err)
	}
}

func TestBashToolRunDeadlineBubbles(t *testing.T) {
	tool := tools.NewBashToolWithTimeout(t.TempDir(), time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := tool.Execute(ctx, json.RawMessage(`{"command":"sleep 30"}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run 级超时应冒泡 DeadlineExceeded，实际: %v", err)
	}
}

func TestBashToolRejectsWorkspaceEscape(t *testing.T) {
	tool := tools.NewBashTool(t.TempDir())

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"cat /etc/passwd"}`))
	if err == nil {
		t.Fatal("逃出工作区的命令应失败")
	}
	if !strings.Contains(err.Error(), "工作区外") {
		t.Fatalf("错误信息应说明访问工作区外，实际: %v", err)
	}
}
