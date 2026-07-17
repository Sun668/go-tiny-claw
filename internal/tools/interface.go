package tools

import "github.com/Sun668/go-tiny-claw/internal/approval"

type RiskedTool interface {
	RiskLevel() approval.RiskLevel
}
