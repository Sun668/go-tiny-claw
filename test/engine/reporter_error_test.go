package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/engine"
	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

var errReporterClosed = errors.New("事件输出已关闭")

type failingReporter struct {
	onMessage  error
	onToolCall error
}

func (r *failingReporter) OnThinking(context.Context) error { return nil }

func (r *failingReporter) OnToolCall(context.Context, string, string) error {
	return r.onToolCall
}

func (r *failingReporter) OnToolResult(context.Context, string, string, bool) error {
	return nil
}

func (r *failingReporter) OnMessage(context.Context, string) error {
	return r.onMessage
}

type countingTool struct {
	name      string
	risk      approval.RiskLevel
	execCount atomic.Int32
}

func (t *countingTool) Name() string { return t.name }

func (t *countingTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.name,
		Description: "count executions",
	}
}

func (t *countingTool) RiskLevel() approval.RiskLevel { return t.risk }

func (t *countingTool) Execute(context.Context, json.RawMessage) (string, error) {
	t.execCount.Add(1)
	return "ok", nil
}

func newReporterErrorEngine(t *testing.T, tool tools.BaseTool, action *schema.Message) *engine.AgentEngine {
	t.Helper()

	reg := tools.NewRegistry()
	reg.Register(tool)

	gate := approval.NewGate(
		approval.DefaultPolicy{},
		&blockingApprovalHandler{},
		approval.NewMemoryGrantStore(),
	)

	return engine.NewAgentEngine(
		&fakeProvider{action: action},
		reg,
		gate,
		false,
		false,
	)
}

func TestOnMessageErrorFillsToolObservations(t *testing.T) {
	const toolCallID = "call-report-msg"

	tool := &countingTool{name: "read_file", risk: approval.RiskSafe}
	eng := newReporterErrorEngine(t, tool, &schema.Message{
		Role:    schema.RoleAssistant,
		Content: "准备读取",
		ToolCalls: []schema.ToolCall{
			{
				ID:        toolCallID,
				Name:      "read_file",
				Arguments: json.RawMessage(`{}`),
			},
		},
	})

	session := ctxpkg.NewSession("report-on-message", t.TempDir())
	session.Append(schema.Message{
		Role:    schema.RoleUser,
		Content: "读文件",
	})

	err := eng.Run(context.Background(), session, &failingReporter{
		onMessage: errReporterClosed,
	})
	if !errors.Is(err, errReporterClosed) {
		t.Fatalf("期望事件输出错误，实际: %v", err)
	}
	if tool.execCount.Load() != 0 {
		t.Fatal("OnMessage 失败后不应执行工具")
	}

	history := session.GetWorkingMemory(0)
	if findAssistantWithToolCall(history, toolCallID) == nil {
		t.Fatal("Session 应保留带 ToolCall 的 Assistant 消息")
	}
	observation := findObservation(history, toolCallID)
	if observation == nil {
		t.Fatal("OnMessage 失败后应补齐 Observation")
	}
	if observation.Content != "工具调用已取消" {
		t.Fatalf("Observation 内容错误: %q", observation.Content)
	}
}

func TestOnToolCallErrorSkipsExecuteAndFillsObservation(t *testing.T) {
	const toolCallID = "call-report-tool"

	tool := &countingTool{name: "read_file", risk: approval.RiskSafe}
	eng := newReporterErrorEngine(t, tool, &schema.Message{
		Role:    schema.RoleAssistant,
		Content: "准备读取",
		ToolCalls: []schema.ToolCall{
			{
				ID:        toolCallID,
				Name:      "read_file",
				Arguments: json.RawMessage(`{}`),
			},
		},
	})

	session := ctxpkg.NewSession("report-on-tool-call", t.TempDir())
	session.Append(schema.Message{
		Role:    schema.RoleUser,
		Content: "读文件",
	})

	err := eng.Run(context.Background(), session, &failingReporter{
		onToolCall: errReporterClosed,
	})
	if !errors.Is(err, errReporterClosed) {
		t.Fatalf("期望事件输出错误，实际: %v", err)
	}
	if tool.execCount.Load() != 0 {
		t.Fatal("OnToolCall 失败后不应执行工具")
	}

	observation := findObservation(session.GetWorkingMemory(0), toolCallID)
	if observation == nil {
		t.Fatal("OnToolCall 失败后应补齐 Observation")
	}
	if observation.Content != "工具调用已取消" {
		t.Fatalf("Observation 内容错误: %q", observation.Content)
	}
}
