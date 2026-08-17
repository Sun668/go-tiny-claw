package runtime

import (
	"errors"
	"strings"
	"sync"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/engine"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

type RuntimeFactory struct {
	provider   provider.LLMProvider
	workDir    string
	sessions   *ctxpkg.SessionManager
	grantMu    sync.Mutex
	grantStore approval.GrantStore
}

type RuntimeBundle struct {
	Runtime  *Runtime
	Reporter reporter.Reporter
}

func NewRuntimeFactory(provider provider.LLMProvider, workDir string, sessionManager *ctxpkg.SessionManager) *RuntimeFactory {
	if sessionManager == nil {
		sessionManager = ctxpkg.GlobalSessionMgr
	}

	return &RuntimeFactory{
		provider: provider,
		workDir:  workDir,
		sessions: sessionManager,
	}
}

func (f *RuntimeFactory) NewRuntime(sessionID string, options RuntimeOptions) (*RuntimeBundle, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("会话 ID 不能为空")
	}

	if f.provider == nil {
		return nil, errors.New("LLM Provider 不能为空")
	}

	if err := options.validate(); err != nil {
		return nil, err
	}

	session := f.sessions.GetOrCreate(sessionID, f.workDir)

	registry := f.newToolRegistry()

	grantStore, err := f.sharedGrantStore()
	if err != nil {
		return nil, err
	}

	approvalGate := approval.NewGate(
		approval.DefaultPolicy{},
		options.ApprovalHandler,
		grantStore,
	)

	agentEngine := engine.NewAgentEngine(f.provider, registry, approvalGate, false, false)
	agentEngine.MaxTokens = 200000

	agentRuntime := NewRuntime(agentEngine, session)

	return &RuntimeBundle{
		Runtime:  agentRuntime,
		Reporter: options.Reporter,
	}, nil
}

func (f *RuntimeFactory) sharedGrantStore() (approval.GrantStore, error) {
	f.grantMu.Lock()
	defer f.grantMu.Unlock()

	if f.grantStore != nil {
		return f.grantStore, nil
	}

	store, err := approval.NewFileGrantStore(f.workDir)
	if err != nil {
		return nil, err
	}

	f.grantStore = store
	return store, nil
}

func (f *RuntimeFactory) newToolRegistry() tools.Registry {
	registry := tools.NewRegistry()

	registry.Register(
		tools.NewBashTool(f.workDir),
	)

	registry.Register(
		tools.NewWriteFileTool(f.workDir),
	)

	registry.Register(
		tools.NewReadFileTool(f.workDir),
	)

	registry.Register(
		tools.NewReadSkillTool(f.workDir),
	)

	registry.Register(
		tools.NewEditFileTool(f.workDir),
	)

	return registry
}
