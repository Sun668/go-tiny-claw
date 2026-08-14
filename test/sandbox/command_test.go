package sandbox_test

import (
	"strings"
	"testing"

	"github.com/Sun668/go-tiny-claw/internal/sandbox"
)

func TestIsDestructiveCommand(t *testing.T) {
	denied := []string{
		"rm -rf /",
		"sudo rm -fr /tmp",
		"rm -r -f ./build",
		"mkfs.ext4 /dev/sda",
		"dd of=/dev/sda if=/dev/zero",
		"shutdown -h now",
		"reboot",
	}
	for _, command := range denied {
		if !sandbox.IsDestructiveCommand(command) {
			t.Fatalf("命令 %q 应判定为破坏性", command)
		}
	}

	if sandbox.IsDestructiveCommand("go test ./...") {
		t.Fatal("普通命令不应判定为破坏性")
	}
}

func TestCommandEscapesWorkspace(t *testing.T) {
	allowed := []string{
		"go test ./...",
		"ls -la",
		"cat a.go",
	}
	for _, command := range allowed {
		if err := sandbox.CommandEscapesWorkspace(command); err != nil {
			t.Fatalf("命令 %q 不应判定为逃出工作区: %v", command, err)
		}
	}

	denied := []string{
		"cat /etc/passwd",
		"cd ..",
		"cd /tmp",
		"ls ../../",
		"cat foo/../../outside",
	}
	for _, command := range denied {
		err := sandbox.CommandEscapesWorkspace(command)
		if err == nil {
			t.Fatalf("命令 %q 应判定为逃出工作区", command)
		}
		if !strings.Contains(err.Error(), "工作区外") {
			t.Fatalf("错误信息应说明访问工作区外，实际: %v", err)
		}
	}
}
