package approval_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/approval"
)

func fileGrantFromRequest(req approval.Request) approval.Grant {
	return approval.Grant{
		SessionID:      req.SessionID,
		WorkDir:        req.WorkDir,
		ToolName:       req.ToolCall.Name,
		ArgumentDigest: approval.DigestArguments(req.ToolCall.Arguments),
	}
}

func TestFileGrantStorePersistsAcrossReload(t *testing.T) {
	workDir := t.TempDir()
	store, err := approval.NewFileGrantStore(workDir)
	if err != nil {
		t.Fatalf("创建 FileGrantStore 失败: %v", err)
	}

	req := mutatingRequest(
		"session-1",
		workDir,
		"bash",
		json.RawMessage(`{"command":"go test"}`),
	)
	if err := store.Save(context.Background(), fileGrantFromRequest(req)); err != nil {
		t.Fatalf("保存 Grant 失败: %v", err)
	}

	reloaded, err := approval.NewFileGrantStore(workDir)
	if err != nil {
		t.Fatalf("重新加载 FileGrantStore 失败: %v", err)
	}

	allowed, err := reloaded.Has(context.Background(), req)
	if err != nil {
		t.Fatalf("查询 Grant 失败: %v", err)
	}
	if !allowed {
		t.Fatal("重新加载后应命中相同参数的 Grant")
	}

	miss := mutatingRequest(
		"session-1",
		workDir,
		"bash",
		json.RawMessage(`{"command":"go test ./..."}`),
	)
	allowed, err = reloaded.Has(context.Background(), miss)
	if err != nil {
		t.Fatalf("查询 Grant 失败: %v", err)
	}
	if allowed {
		t.Fatal("不同参数不应命中 Grant")
	}
}

func TestFileGrantStoreRejectsEmptyWorkDir(t *testing.T) {
	_, err := approval.NewFileGrantStore("  ")
	if err == nil {
		t.Fatal("空工作区应拒绝创建 FileGrantStore")
	}
}

func TestFileGrantStoreExpiredGrantIsNotReusedAfterReload(t *testing.T) {
	workDir := t.TempDir()
	store, err := approval.NewFileGrantStore(workDir)
	if err != nil {
		t.Fatalf("创建 FileGrantStore 失败: %v", err)
	}

	req := mutatingRequest(
		"session-1",
		workDir,
		"bash",
		json.RawMessage(`{"command":"go test"}`),
	)
	grant := fileGrantFromRequest(req)
	grant.ExpiresAt = time.Now().Add(-time.Minute)
	if err := store.Save(context.Background(), grant); err != nil {
		t.Fatalf("保存 Grant 失败: %v", err)
	}

	reloaded, err := approval.NewFileGrantStore(workDir)
	if err != nil {
		t.Fatalf("重新加载 FileGrantStore 失败: %v", err)
	}

	allowed, err := reloaded.Has(context.Background(), req)
	if err != nil {
		t.Fatalf("查询 Grant 失败: %v", err)
	}
	if allowed {
		t.Fatal("过期 Grant 重新加载后不应命中")
	}
}

func TestFileGrantStoreAllowsMissingFile(t *testing.T) {
	workDir := t.TempDir()
	store, err := approval.NewFileGrantStore(workDir)
	if err != nil {
		t.Fatalf("没有授权文件时应创建成功: %v", err)
	}

	req := mutatingRequest(
		"session-1",
		workDir,
		"bash",
		json.RawMessage(`{"command":"go test"}`),
	)
	allowed, err := store.Has(context.Background(), req)
	if err != nil {
		t.Fatalf("查询 Grant 失败: %v", err)
	}
	if allowed {
		t.Fatal("空存储不应命中 Grant")
	}
	if _, err := os.Stat(filepath.Join(workDir, ".claw", "grants.json")); !os.IsNotExist(err) {
		t.Fatal("未保存时不应创建授权文件")
	}
}

func TestFileGrantStoreWritesJSONFile(t *testing.T) {
	workDir := t.TempDir()
	store, err := approval.NewFileGrantStore(workDir)
	if err != nil {
		t.Fatalf("创建 FileGrantStore 失败: %v", err)
	}

	req := mutatingRequest(
		"session-1",
		workDir,
		"write_file",
		json.RawMessage(`{"path":"a.go","content":"x"}`),
	)
	if err := store.Save(context.Background(), fileGrantFromRequest(req)); err != nil {
		t.Fatalf("保存 Grant 失败: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(workDir, ".claw", "grants.json"))
	if err != nil {
		t.Fatalf("读取授权文件失败: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("授权文件不应为空")
	}
}

func TestFileGrantStorePersistsAuditFields(t *testing.T) {
	workDir := t.TempDir()
	store, err := approval.NewFileGrantStore(workDir)
	if err != nil {
		t.Fatalf("创建 FileGrantStore 失败: %v", err)
	}

	handler := &recordingHandler{decision: approval.AllowSession}
	gate := newAskGate(handler, store)

	req := mutatingRequest(
		"session-1",
		workDir,
		"write_file",
		json.RawMessage(`{"path":"a.go","content":"x"}`),
	)
	req.ID = "req-audit-1"

	before := time.Now()
	decision, err := gate.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	if decision != approval.AllowSession {
		t.Fatalf("决策错误: %s", decision)
	}

	allowed, err := store.Has(context.Background(), req)
	if err != nil {
		t.Fatalf("查询 Grant 失败: %v", err)
	}
	if !allowed {
		t.Fatal("AllowSession 后应能命中 Grant")
	}

	data, err := os.ReadFile(filepath.Join(workDir, ".claw", "grants.json"))
	if err != nil {
		t.Fatalf("读取授权文件失败: %v", err)
	}

	var grants []approval.Grant
	if err := json.Unmarshal(data, &grants); err != nil {
		t.Fatalf("解析授权文件失败: %v", err)
	}
	if len(grants) != 1 {
		t.Fatalf("应写入 1 条 Grant，实际: %d", len(grants))
	}

	grant := grants[0]
	if grant.RequestID != "req-audit-1" {
		t.Fatalf("RequestID 错误: %s", grant.RequestID)
	}
	if grant.Decision != approval.AllowSession {
		t.Fatalf("Decision 错误: %s", grant.Decision)
	}
	if grant.ApprovedAt.IsZero() {
		t.Fatal("ApprovedAt 不能为零值")
	}
	if grant.ApprovedAt.Before(before.Add(-time.Second)) ||
		grant.ApprovedAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("ApprovedAt 不在预期范围内: %s", grant.ApprovedAt)
	}
	if grant.ArgumentDigest != approval.DigestArguments(req.ToolCall.Arguments) {
		t.Fatal("ArgumentDigest 应与请求参数一致")
	}
}
