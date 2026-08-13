package sandbox_test

import (
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
