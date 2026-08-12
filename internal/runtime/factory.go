package runtime

import (
	"bufio"
	"errors"
	"io"
	"strings"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/engine"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

type RuntimeFactory struct {
	provider provider.LLMProvider
	workDir  string
	sessions *ctxpkg.SessionManager
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

	grantStore := approval.NewMemoryGrantStore()

	approvalGate := approval.NewGate(
		approval.DefaultPolicy{},
		options.ApprovalHandler,
		grantStore,
	)

	agentEngine := engine.NewAgentEngine(f.provider, registry, approvalGate, false, false)

	agentRuntime := NewRuntime(agentEngine, session)

	return &RuntimeBundle{
		Runtime:  agentRuntime,
		Reporter: options.Reporter,
	}, nil
}

func (f *RuntimeFactory) NewTerminalRuntime(sessionID string, reader *bufio.Reader, out io.Writer) (*RuntimeBundle, error) {
	if reader == nil {
		return nil, errors.New("终端读取器不能为空")
	}

	if out == nil {
		return nil, errors.New("终端输出不能为空")
	}

	return f.NewRuntime(sessionID, RuntimeOptions{
		ApprovalHandler: approval.NewTerminalApprovalHandler(reader, out),
		Reporter:        reporter.NewTerminalReporter(out),
	})
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
