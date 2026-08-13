package approval

import (
	"context"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type Decision string

const (
	AllowOnce    Decision = "allow_once"
	AllowSession Decision = "allow_session"
	Deny         Decision = "deny"
)

type RiskLevel string

const (
	RiskSafe      RiskLevel = "safe"
	RiskMutating  RiskLevel = "mutating"
	RiskDangerous RiskLevel = "dangerous"
)

type Request struct {
	ID        string
	SessionID string
	WorkDir   string
	ToolCall  schema.ToolCall
	Risk      RiskLevel
	Reason    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Handler interface {
	Approve(ctx context.Context, request Request) (Decision, error)
}
