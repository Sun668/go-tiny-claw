package reporter

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
)

type TerminalReporter struct {
	out io.Writer
	mu  sync.Mutex
}

func NewTerminalReporter(out io.Writer) *TerminalReporter {
	if out == nil {
		out = io.Discard
	}
	return &TerminalReporter{
		out: out,
	}
}

func (r *TerminalReporter) OnThinking(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprint(r.out, "\n[🤔 思考中] 模型正在推理...\n")
}

func (r *TerminalReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	// 清理参数中的换行符和特殊字符
	displayArgs := strings.ReplaceAll(args, "\n", "\\n")
	displayArgs = strings.ReplaceAll(displayArgs, "\r", "\\r")
	if len(displayArgs) > 150 {
		displayArgs = displayArgs[:150] + "... (已截断)"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	fmt.Fprintf(r.out, "[🛠️ 调用工具] %s\n", toolName)
	fmt.Fprintf(r.out, "   参数: %s\n", displayArgs)
}

func (r *TerminalReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if isError {
		fmt.Fprintf(r.out, "[❌ 执行失败] %s\n", toolName)
		// 显示错误信息
		if result != "" {
			fmt.Fprintf(r.out, "   错误: %s\n", result)
		}
	} else {
		fmt.Fprintf(r.out, "[✅ 执行成功] %s\n", toolName)
	}
}

func (r *TerminalReporter) OnMessage(ctx context.Context, content string) {
	if content == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.out, "\n🤖 Agent 回复:\n%s\n\n", content)
}

func (r *TerminalReporter) OnTextDelta(ctx context.Context, delta string) {
	if delta == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	fmt.Fprint(r.out, delta)
}

func (r *TerminalReporter) OnTextComplete(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	fmt.Fprint(r.out, "\n\n")
}
