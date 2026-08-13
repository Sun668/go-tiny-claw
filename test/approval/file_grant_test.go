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
