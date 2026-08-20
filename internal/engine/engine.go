package engine

import (
	"sync"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

type AgentEngine struct {
	provider           provider.LLMProvider
	registry           tools.Registry
	EnableThinking     bool
	PlanMode           bool // 【新增】计划模式开关
	compactor          *ctxpkg.Compactor
	recovery           *RecoveryManager
	injector           *ReminderInjector
	MaxTurns           int
	approvalGate       *approval.Gate
	MaxToolConcurrency int
	MaxTokens          int
	MaxTokensPerRun    int
	MaxSubagents       int
	subagentMu         sync.Mutex
	subagentUsed       int
	MaxToolTime        time.Duration
	budgetMu           sync.Mutex
	budgetSession      *ctxpkg.Session
	runStart           int
	toolElapsed        time.Duration
}

type IndexedToolCall struct {
	index int
	call  schema.ToolCall
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, approvalGate *approval.Gate, enableThinking bool, planMode bool) *AgentEngine {
	return &AgentEngine{
		provider:           p,
		registry:           r,
		EnableThinking:     enableThinking,
		PlanMode:           planMode,
		compactor:          ctxpkg.NewCompactor(20000, 6),
		recovery:           NewRecoveryManager(),
		injector:           NewReminderInjector(),
		MaxTurns:           20,
		approvalGate:       approvalGate,
		MaxToolConcurrency: 8,
	}
}
