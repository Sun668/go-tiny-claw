package channel_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/channel"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/provider"
	runtimepkg "github.com/Sun668/go-tiny-claw/internal/runtime"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type channelFakeProvider struct{}

func (p *channelFakeProvider) Generate(
	context.Context,
	[]schema.Message,
	[]schema.ToolDefinition,
) (*schema.Message, error) {
	return &schema.Message{}, nil
}

var _ provider.LLMProvider = (*channelFakeProvider)(nil)

func TestChannelSessionRunsProtocolMessages(t *testing.T) {
	serverConn, clientConn := net.Pipe()

	factory := runtimepkg.NewRuntimeFactory(
		&channelFakeProvider{},
		t.TempDir(),
		ctxpkg.GlobalSessionMgr,
	)
	manager := runtimepkg.NewManagerWithFactory(factory)

	session, err := channel.NewChannelSession(
		"channel-session-test",
		serverConn,
		manager,
	)
	if err != nil {
		t.Fatalf("创建 ChannelSession 失败: %v", err)
	}

	runErr := make(chan error, 1)
	go func() {
		runErr <- session.Run(context.Background())
	}()

	clientWriter, err := channel.NewMessageWriter(clientConn)
	if err != nil {
		t.Fatalf("创建客户端 Writer 失败: %v", err)
	}
	clientReader, err := channel.NewMessageReader(clientConn)
	if err != nil {
		t.Fatalf("创建客户端 Reader 失败: %v", err)
	}

	if err := clientWriter.Write(channel.Message{Type: channel.MessagePing}); err != nil {
		t.Fatalf("发送 Ping 失败: %v", err)
	}

	pong, err := readMessageWithTimeout(clientReader, 2*time.Second)
	if err != nil {
		t.Fatalf("读取 Pong 失败: %v", err)
	}
	if pong.Type != channel.MessagePong {
		t.Fatalf("响应类型错误: %s", pong.Type)
	}

	if err := clientWriter.Write(channel.Message{Type: channel.MessageClose}); err != nil {
		t.Fatalf("发送 Close 失败: %v", err)
	}

	if err := <-runErr; err != nil {
		t.Fatalf("ChannelSession 运行失败: %v", err)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("关闭 ChannelSession 失败: %v", err)
	}
	_ = clientConn.Close()
}

func readMessageWithTimeout(
	reader *channel.MessageReader,
	timeout time.Duration,
) (channel.Message, error) {
	type result struct {
		message channel.Message
		err     error
	}

	resultCh := make(chan result, 1)
	go func() {
		message, err := reader.Read()
		resultCh <- result{message: message, err: err}
	}()

	select {
	case result := <-resultCh:
		return result.message, result.err
	case <-time.After(timeout):
		return channel.Message{}, context.DeadlineExceeded
	}
}
