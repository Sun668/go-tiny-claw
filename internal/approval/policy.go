package approval

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
	switch req.Risk {
	case RiskSafe:
		return PolicyAutoAllow
	case RiskMutating, RiskDangerous:
		return PolicyAsk
	default:
		return PolicyDeny
	}
}
