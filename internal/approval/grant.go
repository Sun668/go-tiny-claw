package approval

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Grant struct {
	SessionID      string
	ToolName       string
	WorkDir        string
	ExpiresAt      time.Time
	ArgumentDigest string
}

type GrantStore interface {
	Has(ctx context.Context, request Request) (bool, error)
	Save(ctx context.Context, grant Grant) error
	Revoke(ctx context.Context, grant Grant) error
}

type grantKey struct {
	sessionID      string
	workDir        string
	toolName       string
	argumentDigest string
}

type MemoryGrantStore struct {
	mu     sync.RWMutex
	grants map[grantKey]Grant
}

func NewMemoryGrantStore() *MemoryGrantStore {
	return &MemoryGrantStore{
		grants: make(map[grantKey]Grant),
	}
}

func keyFromGrant(grant Grant) grantKey {
	return grantKey{
		sessionID:      grant.SessionID,
		workDir:        grant.WorkDir,
		toolName:       grant.ToolName,
		argumentDigest: grant.ArgumentDigest,
	}
}
func keyFromRequest(req Request) grantKey {
	return grantKey{
		sessionID:      req.SessionID,
		workDir:        req.WorkDir,
		toolName:       req.ToolCall.Name,
		argumentDigest: DigestArguments(req.ToolCall.Arguments),
	}
}

func (s *MemoryGrantStore) Has(ctx context.Context, request Request) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	key := keyFromRequest(request)

	s.mu.Lock()
	defer s.mu.Unlock()

	grant, exists := s.grants[key]
	if !exists {
		return false, nil
	}

	if !grant.ExpiresAt.IsZero() &&
		!time.Now().Before(grant.ExpiresAt) {
		delete(s.grants, key)
		return false, nil
	}

	return true, nil
}

func (s *MemoryGrantStore) Save(
	ctx context.Context,
	grant Grant,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if grant.SessionID == "" {
		return fmt.Errorf("grant session ID 不能为空")
	}

	if grant.ToolName == "" {
		return fmt.Errorf("grant tool name 不能为空")
	}

	if grant.WorkDir == "" {
		return fmt.Errorf("grant 工作区不能为空")
	}

	key := keyFromGrant(grant)

	s.mu.Lock()
	s.grants[key] = grant
	s.mu.Unlock()

	return nil
}

func (s *MemoryGrantStore) Revoke(
	ctx context.Context,
	grant Grant,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := keyFromGrant(grant)

	s.mu.Lock()
	delete(s.grants, key)
	s.mu.Unlock()

	return nil
}
