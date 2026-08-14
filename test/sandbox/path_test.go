package sandbox_test

import (
	"os"
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

func TestResolveWithinWorkspaceAllowsSymlinkInsideWorkspace(t *testing.T) {
	workDir := t.TempDir()
	target := filepath.Join(workDir, "a.txt")
	if err := os.WriteFile(target, []byte("inside"), 0644); err != nil {
		t.Fatalf("写入区内文件失败: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(workDir, "link.txt")); err != nil {
		t.Fatalf("创建区内符号链接失败: %v", err)
	}

	resolved, err := sandbox.ResolveWithinWorkspace(workDir, "link.txt")
	if err != nil {
		t.Fatalf("指向工作区内的符号链接应解析成功: %v", err)
	}

	want, err := filepath.Abs(filepath.Join(workDir, "link.txt"))
	if err != nil {
		t.Fatalf("计算期望路径失败: %v", err)
	}
	if resolved != filepath.Clean(want) {
		t.Fatalf("解析结果错误: got %s want %s", resolved, want)
	}
}

func TestResolveWithinWorkspaceRejectsSymlinkEscape(t *testing.T) {
	workDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatalf("写入区外文件失败: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workDir, "link.txt")); err != nil {
		t.Fatalf("创建符号链接失败: %v", err)
	}

	_, err := sandbox.ResolveWithinWorkspace(workDir, "link.txt")
	if err == nil {
		t.Fatal("指向工作区外的符号链接应失败")
	}
	if !strings.Contains(err.Error(), "超出工作区") {
		t.Fatalf("错误信息应说明超出工作区，实际: %v", err)
	}
}

func TestResolveWithinWorkspaceReportsSymlinkResolveFailure(t *testing.T) {
	workDir := t.TempDir()
	if err := os.Symlink("b", filepath.Join(workDir, "a")); err != nil {
		t.Fatalf("创建符号链接失败: %v", err)
	}
	if err := os.Symlink("a", filepath.Join(workDir, "b")); err != nil {
		t.Fatalf("创建符号链接失败: %v", err)
	}

	_, err := sandbox.ResolveWithinWorkspace(workDir, "a")
	if err == nil {
		t.Fatal("无法解析的符号链接应失败")
	}
	if !strings.Contains(err.Error(), "解析符号链接失败") {
		t.Fatalf("错误信息应说明解析符号链接失败，实际: %v", err)
	}
	if strings.Contains(err.Error(), "超出工作区") {
		t.Fatalf("解析失败不应报成超出工作区，实际: %v", err)
	}
}

func TestResolveWithinWorkspaceRejectsParentDirSymlinkEscape(t *testing.T) {
	workDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(workDir, "out")); err != nil {
		t.Fatalf("创建指向区外目录的符号链接失败: %v", err)
	}

	_, err := sandbox.ResolveWithinWorkspace(workDir, "out/new.go")
	if err == nil {
		t.Fatal("经区外符号链接目录写入新文件应失败")
	}
	if !strings.Contains(err.Error(), "超出工作区") {
		t.Fatalf("错误信息应说明超出工作区，实际: %v", err)
	}
}
