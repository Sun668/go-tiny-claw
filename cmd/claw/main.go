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
	llmProvider := provider.NewRetryingProvider(provider.NewOpenAICompatibleProvider("glm-5-2-260617"))

	factory := runtime.NewRuntimeFactory(llmProvider, workDir, nil)

	manager := runtime.NewManagerWithFactory(factory)

	session, err := cli.NewTerminalSession(
		"terminal_default",
		manager,
		bufio.NewReader(os.Stdin),
		os.Stdout,
	)

	if err != nil {
		log.Fatalf("创建终端会话失败: %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			log.Printf("关闭终端会话失败: %v", err)
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)

	go func() {
		for range signals {
			session.Interrupt()
		}
	}()

	if err := session.Run(context.Background()); err != nil {
		log.Fatalf("引擎崩溃: %v", err)
	}
}
