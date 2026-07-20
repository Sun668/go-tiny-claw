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

func (f *RuntimeFactory) NewTerminalRuntime(sessionID string, reader *bufio.Reader, out io.Writer) (*RuntimeBundle, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("SessionID can not be empty")
	}

	if reader == nil {
		return nil, errors.New("Reader can not be nil")
	}

	if out == nil {
		return nil, errors.New("Output writer can not be nil")
	}

	if f.provider == nil {
		return nil, errors.New("LLM Provider 不能为空")
	}

	session := f.sessions.GetOrCreate(sessionID, f.workDir)

	registry := f.newToolRegistry()

	approvalHandler := approval.NewTerminalApprovalHandler(reader, out)

	grantStore := approval.NewMemoryGrantStore()

	approvalGate := approval.NewGate(approval.DefaultPolicy{}, approvalHandler, grantStore)

	agentEngine := engine.NewAgentEngine(f.provider, registry, approvalGate, false, false)

	agentRuntime := NewRuntime(agentEngine, session)

	terminalReporter := reporter.NewTerminalReporter(out)

	return &RuntimeBundle{
		Runtime:  agentRuntime,
		Reporter: terminalReporter,
	}, nil
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
		tools.NewEditFileTool(f.workDir),
	)

	return registry
}
