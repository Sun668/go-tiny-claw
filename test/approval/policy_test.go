package approval_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

func bashRequest(t *testing.T, command string) approval.Request {
	t.Helper()

	args, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		t.Fatalf("序列化 bash 参数失败: %v", err)
	}

	return approval.Request{
		SessionID: "session-1",
		WorkDir:   "/work",
		ToolCall: schema.ToolCall{
			Name:      "bash",
			Arguments: args,
		},
		Risk: approval.RiskDangerous,
	}
}

func TestDefaultPolicyDeniesDestructiveBash(t *testing.T) {
	policy := approval.DefaultPolicy{}

	cases := []string{
		"rm -rf /",
		"sudo rm -fr /tmp",
		"rm -r -f ./build",
		"mkfs.ext4 /dev/sda",
		"dd of=/dev/sda if=/dev/zero",
		"shutdown -h now",
		"reboot",
	}

	for _, command := range cases {
		decision := policy.Evaluate(bashRequest(t, command))
		if decision != approval.PolicyDeny {
			t.Fatalf("命令 %q 应被拒绝，实际: %s", command, decision)
		}
	}
}

func TestDefaultPolicyDeniesEscapingBash(t *testing.T) {
	policy := approval.DefaultPolicy{}

	cases := []string{
		"cat /etc/passwd",
		"cd ..",
		"cd /tmp",
		"ls ../../",
		"cat foo/../../outside",
	}

	for _, command := range cases {
		decision := policy.Evaluate(bashRequest(t, command))
		if decision != approval.PolicyDeny {
			t.Fatalf("命令 %q 应被拒绝，实际: %s", command, decision)
		}
	}
}

func TestDefaultPolicyAsksOrdinaryBash(t *testing.T) {
	policy := approval.DefaultPolicy{}

	decision := policy.Evaluate(bashRequest(t, "go test ./..."))
	if decision != approval.PolicyAsk {
		t.Fatalf("普通 bash 应询问，实际: %s", decision)
	}
}

func TestDefaultPolicyKeepsToolRiskForNonBash(t *testing.T) {
	policy := approval.DefaultPolicy{}

	read := approval.Request{
		ToolCall: schema.ToolCall{Name: "read_file"},
		Risk:     approval.RiskSafe,
	}
	if decision := policy.Evaluate(read); decision != approval.PolicyAutoAllow {
		t.Fatalf("read_file 应自动允许，实际: %s", decision)
	}

	write := approval.Request{
		ToolCall: schema.ToolCall{Name: "write_file"},
		Risk:     approval.RiskMutating,
	}
	if decision := policy.Evaluate(write); decision != approval.PolicyAsk {
		t.Fatalf("write_file 应询问，实际: %s", decision)
	}
}

func TestGateDeniesDestructiveBashWithoutHandler(t *testing.T) {
	handler := &recordingHandler{decision: approval.AllowOnce}
	gate := newAskGate(handler, approval.NewMemoryGrantStore())

	decision, err := gate.Check(context.Background(), bashRequest(t, "rm -rf /"))
	if err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	if decision != approval.Deny {
		t.Fatalf("破坏性 bash 应直接拒绝，实际: %s", decision)
	}
	if handler.calls != 0 {
		t.Fatalf("PolicyDeny 不应询问用户，Handler 调用次数: %d", handler.calls)
	}
}

func TestGateDeniesEscapingBashWithoutHandler(t *testing.T) {
	handler := &recordingHandler{decision: approval.AllowOnce}
	gate := newAskGate(handler, approval.NewMemoryGrantStore())

	decision, err := gate.Check(context.Background(), bashRequest(t, "cat /etc/passwd"))
	if err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	if decision != approval.Deny {
		t.Fatalf("逃出工作区的 bash 应直接拒绝，实际: %s", decision)
	}
	if handler.calls != 0 {
		t.Fatalf("PolicyDeny 不应询问用户，Handler 调用次数: %d", handler.calls)
	}
}
