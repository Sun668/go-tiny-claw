package sandbox_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sun668/go-tiny-claw/internal/sandbox"
)

func TestResolveWithinWorkspaceAllowsRelativePath(t *testing.T) {
	workDir := t.TempDir()

	resolved, err := sandbox.ResolveWithinWorkspace(workDir, "a.go")
	if err != nil {
		t.Fatalf("相对路径应解析成功: %v", err)
	}

	want, err := filepath.Abs(filepath.Join(workDir, "a.go"))
	if err != nil {
		t.Fatalf("计算期望路径失败: %v", err)
	}
	if resolved != filepath.Clean(want) {
		t.Fatalf("解析结果错误: got %s want %s", resolved, want)
	}
}

func TestResolveWithinWorkspaceAllowsNestedRelativePath(t *testing.T) {
	workDir := t.TempDir()

	resolved, err := sandbox.ResolveWithinWorkspace(workDir, "subdir/a.go")
	if err != nil {
		t.Fatalf("子目录相对路径应解析成功: %v", err)
	}

	want, err := filepath.Abs(filepath.Join(workDir, "subdir", "a.go"))
	if err != nil {
		t.Fatalf("计算期望路径失败: %v", err)
	}
	if resolved != filepath.Clean(want) {
		t.Fatalf("解析结果错误: got %s want %s", resolved, want)
	}
}

func TestResolveWithinWorkspaceAllowsWorkspaceRoot(t *testing.T) {
	workDir := t.TempDir()

	resolved, err := sandbox.ResolveWithinWorkspace(workDir, ".")
	if err != nil {
		t.Fatalf("工作区根路径应解析成功: %v", err)
	}

	want, err := filepath.Abs(workDir)
	if err != nil {
		t.Fatalf("计算期望路径失败: %v", err)
	}
	if resolved != filepath.Clean(want) {
		t.Fatalf("解析结果错误: got %s want %s", resolved, want)
	}
}

func TestResolveWithinWorkspaceRejectsParentEscape(t *testing.T) {
	workDir := t.TempDir()

	_, err := sandbox.ResolveWithinWorkspace(workDir, "../outside.txt")
	if err == nil {
		t.Fatal("逃逸到父目录应失败")
	}
	if !strings.Contains(err.Error(), "超出工作区") {
		t.Fatalf("错误信息应说明超出工作区，实际: %v", err)
	}
}

func TestResolveWithinWorkspaceRejectsNestedParentEscape(t *testing.T) {
	workDir := t.TempDir()

	_, err := sandbox.ResolveWithinWorkspace(workDir, "foo/../../outside.txt")
	if err == nil {
		t.Fatal("经子目录逃逸应失败")
	}
	if !strings.Contains(err.Error(), "超出工作区") {
		t.Fatalf("错误信息应说明超出工作区，实际: %v", err)
	}
}

func TestResolveWithinWorkspaceRejectsAbsolutePath(t *testing.T) {
	workDir := t.TempDir()

	_, err := sandbox.ResolveWithinWorkspace(workDir, "/etc/passwd")
	if err == nil {
		t.Fatal("绝对路径应失败")
	}
	if !strings.Contains(err.Error(), "绝对路径") {
		t.Fatalf("错误信息应说明不允许绝对路径，实际: %v", err)
	}
}

func TestResolveWithinWorkspaceRejectsEmptyUserPath(t *testing.T) {
	workDir := t.TempDir()

	_, err := sandbox.ResolveWithinWorkspace(workDir, "  ")
	if err == nil {
		t.Fatal("空路径应失败")
	}
	if !strings.Contains(err.Error(), "路径不能为空") {
		t.Fatalf("错误信息应为中文，实际: %v", err)
	}
}

func TestResolveWithinWorkspaceRejectsEmptyWorkDir(t *testing.T) {
	_, err := sandbox.ResolveWithinWorkspace("  ", "a.go")
	if err == nil {
		t.Fatal("空工作区应失败")
	}
	if !strings.Contains(err.Error(), "工作区路径不能为空") {
		t.Fatalf("错误信息应说明工作区为空，实际: %v", err)
	}
}
