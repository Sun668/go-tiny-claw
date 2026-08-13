package approval

import (
	"encoding/json"

	"github.com/Sun668/go-tiny-claw/internal/sandbox"
)

type PolicyDecision string

const (
	PolicyAutoAllow PolicyDecision = "auto_allow"
	PolicyAsk       PolicyDecision = "ask"
	PolicyDeny      PolicyDecision = "deny"
)

type Policy interface {
	Evaluate(Request) PolicyDecision
}

type DefaultPolicy struct{}

func (DefaultPolicy) Evaluate(req Request) PolicyDecision {
	if req.ToolCall.Name == "bash" && isDeniedBash(req.ToolCall.Arguments) {
		return PolicyDeny
	}

	switch req.Risk {
	case RiskSafe:
		return PolicyAutoAllow
	case RiskMutating, RiskDangerous:
		return PolicyAsk
	default:
		return PolicyDeny
	}
}

func isDeniedBash(args json.RawMessage) bool {
	var parsed struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &parsed); err != nil {
		return false
	}
	return sandbox.IsDestructiveCommand(parsed.Command)
}
