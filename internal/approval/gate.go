package approval

import (
	"context"
	"time"
)

type Gate struct {
	policy  Policy
	handler Handler
	grants  GrantStore
}

func (g *Gate) Check(ctx context.Context, request Request) (Decision, error) {
	ploicyDecision := g.policy.Evaluate(request)

	switch ploicyDecision {
	case PolicyAutoAllow:
		return AllowOnce, nil
	case PolicyDeny:
		return Deny, nil
	}

	allowed, err := g.grants.Has(ctx, request)
	if err != nil {
		return "", err
	}
	if allowed {
		return AllowOnce, nil
	}

	decision, err := g.handler.Approve(ctx, request)
	if err != nil {
		return "", err
	}

	if decision == AllowSession {
		err = g.grants.Save(ctx, Grant{
			SessionID:      request.SessionID,
			WorkDir:        request.WorkDir,
			ToolName:       request.ToolCall.Name,
			ArgumentDigest: DigestArguments(request.ToolCall.Arguments),
			RequestID:      request.ID,
			Decision:       decision,
			ApprovedAt:     time.Now(),
		})
	}

	return decision, err
}

func NewGate(policy Policy, handler Handler, grants GrantStore) *Gate {
	return &Gate{
		policy:  policy,
		handler: handler,
		grants:  grants,
	}
}
