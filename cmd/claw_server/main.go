package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"

	"github.com/Sun668/go-tiny-claw/internal/provider"
	runtimepkg "github.com/Sun668/go-tiny-claw/internal/runtime"
	"github.com/Sun668/go-tiny-claw/internal/server"
)

func main() {
	if os.Getenv("ZHIPU_API_KEY") == "" {
		log.Fatal("请先导出 ZHIPU_API_KEY 环境变量")
	}

	workDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("获取工作目录失败: %v", err)
	}
	workDir += "/workspace"

	llmProvider := provider.NewRetryingProvider(provider.NewOpenAICompatibleProvider("glm-5-2-260617"))

	factory := runtimepkg.NewRuntimeFactory(
		llmProvider,
		workDir,
		nil,
	)

	manager := runtimepkg.NewManagerWithFactory(factory)

	listener, err := net.Listen(
		"tcp",
		":8080",
	)
	if err != nil {
		log.Fatalf("监听 TCP 端口失败: %v", err)
	}

	defer listener.Close()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
	)
	defer stop()

	log.Println("TCP Server 监听在 :8080")

	tcpServer := server.NewTCPServer(
		listener,
		manager,
	)

	webSocketServer := server.NewWebSocketServer(manager)
	go func() {
		if err := webSocketServer.Serve(ctx, ":8081"); err != nil {
			log.Printf("WebSocket Server 运行失败: %v", err)
		}
	}()
	defer webSocketServer.Close()

	log.Println("WebSocket Server 监听在 :8081，路径为 /ws")

	if err := tcpServer.Serve(ctx); err != nil {
		log.Fatalf("TCP Server 运行失败: %v", err)
	}
}
