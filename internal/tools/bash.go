package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	"github.com/Sun668/go-tiny-claw/internal/sandbox"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

// ErrToolTimeout 表示单个工具自身的执行时限到达。
// 它不是 context.DeadlineExceeded，因此不会把整次 Run 映射为 TaskTimedOut；
// Engine 会把它写成 Observation 后继续对话。
var ErrToolTimeout = errors.New("命令执行超时")

type BashTool struct {
	workDir string
	timeout time.Duration
}

func NewBashTool(workDir string) *BashTool {
	return NewBashToolWithTimeout(workDir, 30*time.Second)
}

func NewBashToolWithTimeout(workDir string, timeout time.Duration) *BashTool {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &BashTool{
		workDir: workDir,
		timeout: timeout,
	}
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "在当前工作区执行任意的 bash 命令。支持链式命令(如 &&)。返回标准输出(stdout)和标准错误(stderr)。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "要执行的 bash 命令",
				},
			},
			"required": []string{"command"},
		},
	}
}

type bashArgs struct {
	Command string `json:"command"`
}

func (t *BashTool) toolTimeout() time.Duration {
	if t.timeout > 0 {
		return t.timeout
	}
	return 30 * time.Second
}

func (t *BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input bashArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, t.toolTimeout())
	defer cancel()

	if err := sandbox.CommandEscapesWorkspace(input.Command); err != nil {
		return "", fmt.Errorf("命令会逃离工作区: %w", err)
	}

	cmd := exec.CommandContext(timeoutCtx, "bash", "-c", input.Command)
	cmd.Dir = t.workDir

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	// 1) 父 ctx 取消 / Run 级超时：向上冒泡，由 Runtime 映射为 canceled / timed_out
	if parentErr := ctx.Err(); parentErr != nil {
		return "", parentErr
	}

	// 2) 仅工具自身 30s（或配置时限）到期：写成可观察结果，不冒泡 DeadlineExceeded
	if timeoutCtx.Err() == context.DeadlineExceeded {
		msg := outputStr
		if msg != "" {
			msg += "\n"
		}
		msg += fmt.Sprintf("[警告: 命令执行超时(%s)，已被系统强制终止。]", t.toolTimeout())
		return msg, ErrToolTimeout
	}

	if err != nil {
		return fmt.Sprintf("执行报错: %v\n输出:\n%s", err, outputStr), nil
	}

	if outputStr == "" {
		return "命令执行成功，无终端输出。", nil
	}

	const maxLen = 8000
	if len(outputStr) > maxLen {
		return fmt.Sprintf("%s\n\n...[终端输出过长，已截断至前 %d 字节]...", outputStr[:maxLen], maxLen), nil
	}

	return outputStr, nil
}

func (t *BashTool) RiskLevel() approval.RiskLevel {
	return approval.RiskDangerous
}
