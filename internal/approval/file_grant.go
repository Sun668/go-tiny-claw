package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FileGrantStore struct {
	path   string
	mu     sync.Mutex
	grants map[grantKey]Grant
}

func NewFileGrantStore(workDir string) (*FileGrantStore, error) {
	if strings.TrimSpace(workDir) == "" {
		return nil, fmt.Errorf("工作区路径不能为空")
	}

	root, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("解析工作区路径失败: %w", err)
	}

	store := &FileGrantStore{
		path:   filepath.Join(root, ".claw", "grants.json"),
		grants: make(map[grantKey]Grant),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *FileGrantStore) Has(ctx context.Context, request Request) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := keyFromRequest(request)
	grant, exists := s.grants[key]
	if !exists {
		return false, nil
	}

	if !grant.ExpiresAt.IsZero() &&
		!time.Now().Before(grant.ExpiresAt) {
		delete(s.grants, key)
		if err := s.persistLocked(); err != nil {
			return false, err
		}
		return false, nil
	}

	return true, nil
}

func (s *FileGrantStore) Save(ctx context.Context, grant Grant) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateGrant(grant); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := keyFromGrant(grant)
	previous, hadPrevious := s.grants[key]
	s.grants[key] = grant
	if err := s.persistLocked(); err != nil {
		if hadPrevious {
			s.grants[key] = previous
		} else {
			delete(s.grants, key)
		}
		return err
	}
	return nil
}

func (s *FileGrantStore) Revoke(ctx context.Context, grant Grant) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := keyFromGrant(grant)
	previous, existed := s.grants[key]
	if !existed {
		return nil
	}
	delete(s.grants, key)
	if err := s.persistLocked(); err != nil {
		s.grants[key] = previous
		return err
	}
	return nil
}

func (s *FileGrantStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取授权文件失败: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	var grants []Grant
	if err := json.Unmarshal(data, &grants); err != nil {
		return fmt.Errorf("授权文件格式无效: %w", err)
	}

	s.grants = make(map[grantKey]Grant, len(grants))
	for _, grant := range grants {
		s.grants[keyFromGrant(grant)] = grant
	}
	return nil
}

func (s *FileGrantStore) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("创建授权目录失败: %w", err)
	}

	grants := make([]Grant, 0, len(s.grants))
	for _, grant := range s.grants {
		grants = append(grants, grant)
	}

	data, err := json.MarshalIndent(grants, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化授权失败: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("写入授权文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("写入授权文件失败: %w", err)
	}
	return nil
}
