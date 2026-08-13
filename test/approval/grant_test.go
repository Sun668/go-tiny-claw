package approval_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type recordingHandler struct {
	decision approval.Decision
	calls    int
}

func (h *recordingHandler) Approve(
	context.Context,
	approval.Request,
) (approval.Decision, error) {
	h.calls++
	return h.decision, nil
}

func mutatingRequest(
	sessionID, workDir, toolName string,
	args json.RawMessage,
) approval.Request {
	return approval.Request{
		SessionID: sessionID,
		WorkDir:   workDir,
		ToolCall: schema.ToolCall{
			Name:      toolName,
			Arguments: args,
		},
		Risk: approval.RiskMutating,
	}
}

func newAskGate(handler approval.Handler, store approval.GrantStore) *approval.Gate {
	return approval.NewGate(approval.DefaultPolicy{}, handler, store)
}

func TestAllowSessionReusesSameArguments(t *testing.T) {
	handler := &recordingHandler{decision: approval.AllowSession}
	gate := newAskGate(handler, approval.NewMemoryGrantStore())

	req := mutatingRequest(
		"session-1",
		"/work",
		"bash",
		json.RawMessage(`{"command":"go test"}`),
	)

	first, err := gate.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("第一次审批失败: %v", err)
	}
	if first != approval.AllowSession {
		t.Fatalf("第一次决策错误: %s", first)
	}

	second, err := gate.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("第二次审批失败: %v", err)
	}
	if second != approval.AllowOnce {
		t.Fatalf("命中 Grant 时应返回 AllowOnce，实际: %s", second)
	}
	if handler.calls != 1 {
		t.Fatalf("相同参数应复用 Grant，Handler 调用次数: %d", handler.calls)
	}
}

func TestAllowSessionDoesNotReuseDifferentArguments(t *testing.T) {
	handler := &recordingHandler{decision: approval.AllowSession}
	gate := newAskGate(handler, approval.NewMemoryGrantStore())

	firstReq := mutatingRequest(
		"session-1",
		"/work",
		"bash",
		json.RawMessage(`{"command":"go test"}`),
	)
	if _, err := gate.Check(context.Background(), firstReq); err != nil {
		t.Fatalf("第一次审批失败: %v", err)
	}

	secondReq := mutatingRequest(
		"session-1",
		"/work",
		"bash",
		json.RawMessage(`{"command":"rm -rf /"}`),
	)
	decision, err := gate.Check(context.Background(), secondReq)
	if err != nil {
		t.Fatalf("第二次审批失败: %v", err)
	}
	if decision != approval.AllowSession {
		t.Fatalf("不同参数不应复用 Grant，决策: %s", decision)
	}
	if handler.calls != 2 {
		t.Fatalf("不同参数应再次询问，Handler 调用次数: %d", handler.calls)
	}
}

func TestAllowSessionTreatsCanonicalJSONAsSameGrant(t *testing.T) {
	handler := &recordingHandler{decision: approval.AllowSession}
	gate := newAskGate(handler, approval.NewMemoryGrantStore())

	firstReq := mutatingRequest(
		"session-1",
		"/work",
		"write_file",
		json.RawMessage(`{"path":"a.go","content":"x"}`),
	)
	if _, err := gate.Check(context.Background(), firstReq); err != nil {
		t.Fatalf("第一次审批失败: %v", err)
	}

	secondReq := mutatingRequest(
		"session-1",
		"/work",
		"write_file",
		json.RawMessage(`{"content":"x","path":"a.go"}`),
	)
	decision, err := gate.Check(context.Background(), secondReq)
	if err != nil {
		t.Fatalf("第二次审批失败: %v", err)
	}
	if decision != approval.AllowOnce {
		t.Fatalf("键顺序不同但内容相同应命中 Grant，决策: %s", decision)
	}
	if handler.calls != 1 {
		t.Fatalf("规范化后应视为同一 Grant，Handler 调用次数: %d", handler.calls)
	}
}

func TestAllowSessionDoesNotReuseDifferentWorkDir(t *testing.T) {
	handler := &recordingHandler{decision: approval.AllowSession}
	gate := newAskGate(handler, approval.NewMemoryGrantStore())

	firstReq := mutatingRequest(
		"session-1",
		"/work-a",
		"bash",
		json.RawMessage(`{"command":"go test"}`),
	)
	if _, err := gate.Check(context.Background(), firstReq); err != nil {
		t.Fatalf("第一次审批失败: %v", err)
	}

	secondReq := mutatingRequest(
		"session-1",
		"/work-b",
		"bash",
		json.RawMessage(`{"command":"go test"}`),
	)
	if _, err := gate.Check(context.Background(), secondReq); err != nil {
		t.Fatalf("第二次审批失败: %v", err)
	}
	if handler.calls != 2 {
		t.Fatalf("不同工作区不应复用 Grant，Handler 调用次数: %d", handler.calls)
	}
}

func TestExpiredGrantIsNotReused(t *testing.T) {
	store := approval.NewMemoryGrantStore()
	req := mutatingRequest(
		"session-1",
		"/work",
		"bash",
		json.RawMessage(`{"command":"go test"}`),
	)

	err := store.Save(context.Background(), approval.Grant{
		SessionID:      req.SessionID,
		WorkDir:        req.WorkDir,
		ToolName:       req.ToolCall.Name,
		ArgumentDigest: approval.DigestArguments(req.ToolCall.Arguments),
		ExpiresAt:      time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("保存 Grant 失败: %v", err)
	}

	allowed, err := store.Has(context.Background(), req)
	if err != nil {
		t.Fatalf("查询 Grant 失败: %v", err)
	}
	if allowed {
		t.Fatal("过期 Grant 不应命中")
	}
}

func TestSaveRejectsEmptyWorkDir(t *testing.T) {
	store := approval.NewMemoryGrantStore()
	err := store.Save(context.Background(), approval.Grant{
		SessionID: "session-1",
		ToolName:  "bash",
	})
	if err == nil {
		t.Fatal("WorkDir 为空时应拒绝保存")
	}
}
