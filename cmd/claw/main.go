// cmd/claw/main.go
package main

import (
	"bufio"
	"context"
	"log"
	"os"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	"github.com/Sun668/go-tiny-claw/internal/cli"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/engine"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	runtime "github.com/Sun668/go-tiny-claw/internal/runtime"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

func main() {
	if os.Getenv("ZHIPU_API_KEY") == "" {
		log.Fatal("请先导出 ZHIPU_API_KEY 环境变量")
	}

	workDir, _ := os.Getwd()
	workDir += "/workspace"
	llmProvider := provider.NewOpenAICompatibleProvider("glm-5-2-260617")

	registry := tools.NewRegistry()
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	reader := bufio.NewReader(os.Stdin)

	handler := approval.NewTerminalApprovalHandler(reader, os.Stdout)

	grantStore := approval.NewMemoryGrantStore()

	gate := approval.NewGate(approval.DefaultPolicy{}, handler, grantStore)

	eng := engine.NewAgentEngine(llmProvider, registry, gate, false, false)
	rep := reporter.NewTerminalReporter()
	sess := ctxpkg.GlobalSessionMgr.GetOrCreate("terminal_default", workDir)
	rt := runtime.NewRuntime(eng, sess)

	repl := cli.NewREPL(reader, os.Stdout, rt, rep)

	if err := repl.Run(context.Background()); err != nil {
		log.Fatalf("引擎崩溃: %v", err)
	}
}
