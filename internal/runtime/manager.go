package runtime

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

var ErrRuntimeExists = errors.New("Runtime 已经存在")
var ErrRuntimeNotFound = errors.New("Runtime 不存在")
var ErrFactoryNotConfigured = errors.New("RuntimeFactory 未配置")
var ErrNilRuntime = errors.New("Runtime 不能为空")
var ErrRuntimeCreating = errors.New("Runtime 正在创建")

type Manager struct {
	mu       sync.RWMutex
	factory  *RuntimeFactory
	runtimes map[string]*Runtime
	creating map[string]struct{}
}

func NewManager() *Manager {
	return &Manager{
		runtimes: make(map[string]*Runtime),
		creating: make(map[string]struct{}),
	}
}

func NewManagerWithFactory(factory *RuntimeFactory) *Manager {
	manager := NewManager()
	manager.factory = factory
	return manager
}

func (m *Manager) Create(sessionID string, options RuntimeOptions) (*RuntimeBundle, error) {
	if m.factory == nil {
		return nil, ErrFactoryNotConfigured
	}

	sessionID = strings.TrimSpace(sessionID)

	if sessionID == "" {
		return nil, errors.New("session ID 不能为空")
	}

	m.mu.Lock()

	if _, exists := m.runtimes[sessionID]; exists {
		m.mu.Unlock()
		return nil, ErrRuntimeExists
	}

	if _, exists := m.creating[sessionID]; exists {
		m.mu.Unlock()
		return nil, ErrRuntimeCreating
	}

	m.creating[sessionID] = struct{}{}

	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.creating, sessionID)
		m.mu.Unlock()
	}()

	bundle, err := m.factory.NewRuntime(sessionID, options)

	if err != nil {
		return nil, err
	}

	if err := m.Add(sessionID, bundle.Runtime); err != nil {
		bundle.Runtime.Cancel()
		return nil, err
	}

	return bundle, nil

}

func (m *Manager) Add(sessionID string, runtime *Runtime) error {
	sessionID = strings.TrimSpace(sessionID)

	if sessionID == "" {
		return errors.New("session ID 不能为空")
	}

	if runtime == nil {
		return ErrNilRuntime
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.runtimes[sessionID]; exists {
		return ErrRuntimeExists
	}

	m.runtimes[sessionID] = runtime
	return nil
}

func (m *Manager) Get(sessionID string) (*Runtime, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if runtime, exists := m.runtimes[sessionID]; exists {
		return runtime, nil
	}
	return nil, ErrRuntimeNotFound
}

func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionIDs := make([]string, 0, len(m.runtimes))

	for sessionID := range m.runtimes {
		sessionIDs = append(sessionIDs, sessionID)
	}

	sort.Strings(sessionIDs)

	return sessionIDs
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.runtimes)
}

func (m *Manager) Destroy(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)

	m.mu.Lock()

	agentRuntime, exists := m.runtimes[sessionID]
	if !exists {
		m.mu.Unlock()
		return ErrRuntimeNotFound
	}

	delete(m.runtimes, sessionID)
	m.mu.Unlock()

	agentRuntime.Cancel()

	return nil
}

func (m *Manager) Remove(sessionID string) error {
	return m.Destroy(sessionID)
}
