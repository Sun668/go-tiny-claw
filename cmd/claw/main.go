// cmd/claw/main.go
package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/Sun668/go-tiny-claw/internal/cli"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	runtime "github.com/Sun668/go-tiny-claw/internal/runtime"
)

func main() {
	if os.Getenv("ZHIPU_API_KEY") == "" {
		log.Fatal("请先导出 ZHIPU_API_KEY 环境变量")
	}

	workDir, _ := os.Getwd()
	workDir += "/workspace"
	llmProvider := provider.NewOpenAICompatibleProvider("glm-5-2-260617")

	reader := bufio.NewReader(os.Stdin)

	factory := runtime.NewRuntimeFactory(llmProvider, workDir, nil)

	manager := runtime.NewManagerWithFactory(factory)

	runtimeBundle, err := manager.Create("terminal_default", reader, os.Stdout)

	if err != nil {
		log.Fatalf("创建 Runtime 失败: %v", err)
	}

	defer func() {
		if err := manager.Destroy("terminal_default"); err != nil {
			log.Printf("销毁 Runtime 失败: %v", err)
		}
	}()

	repl := cli.NewREPL(reader, os.Stdout, runtimeBundle.Runtime, runtimeBundle.Reporter)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)

	go func() {
		for range signals {
			repl.Interrupt()
		}
	}()

	if err := repl.Run(context.Background()); err != nil {
		log.Fatalf("引擎崩溃: %v", err)
	}
}
