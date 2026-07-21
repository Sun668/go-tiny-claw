package channel_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/Sun668/go-tiny-claw/internal/channel"
)

func TestMessageReaderAcceptsIOReader(t *testing.T) {
	reader, err := channel.NewMessageReader(strings.NewReader(
		`{"type":"prompt","content":"读取文件"}` + "\n",
	))
	if err != nil {
		t.Fatalf("创建消息读取器失败: %v", err)
	}

	message, err := reader.Read()
	if err != nil {
		t.Fatalf("读取消息失败: %v", err)
	}

	if message.Type != channel.MessagePrompt || message.Content != "读取文件" {
		t.Fatalf("消息内容错误: %+v", message)
	}
}

func TestMessageWriterAcceptsIOWriter(t *testing.T) {
	var output bytes.Buffer
	writer, err := channel.NewMessageWriter(&output)
	if err != nil {
		t.Fatalf("创建消息写入器失败: %v", err)
	}

	if err := writer.Write(channel.Message{
		Type:    channel.MessagePong,
		Content: "ok",
	}); err != nil {
		t.Fatalf("写入消息失败: %v", err)
	}

	var message channel.Message
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &message); err != nil {
		t.Fatalf("解析输出消息失败: %v", err)
	}

	if message.Type != channel.MessagePong || message.Content != "ok" {
		t.Fatalf("输出消息错误: %+v", message)
	}
}

func TestMessageWriterSerializesConcurrentWrites(t *testing.T) {
	var output bytes.Buffer
	writer, err := channel.NewMessageWriter(&output)
	if err != nil {
		t.Fatalf("创建消息写入器失败: %v", err)
	}

	const messageCount = 32
	var waitGroup sync.WaitGroup
	waitGroup.Add(messageCount)

	for index := 0; index < messageCount; index++ {
		index := index
		go func() {
			defer waitGroup.Done()
			if err := writer.Write(channel.Message{
				Type:    channel.MessagePong,
				Content: string(rune('a' + index)),
			}); err != nil {
				t.Errorf("并发写入失败: %v", err)
			}
		}()
	}

	waitGroup.Wait()

	reader, err := channel.NewMessageReader(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatalf("创建输出读取器失败: %v", err)
	}

	for index := 0; index < messageCount; index++ {
		if _, err := reader.Read(); err != nil {
			t.Fatalf("第 %d 条并发输出不是完整 JSON: %v", index, err)
		}
	}
}

func TestMessageReaderRejectsInvalidApprovalResponse(t *testing.T) {
	reader, err := channel.NewMessageReader(strings.NewReader(
		`{"type":"approval_response","request_id":"request-1"}` + "\n",
	))
	if err != nil {
		t.Fatalf("创建消息读取器失败: %v", err)
	}

	if _, err := reader.Read(); err == nil {
		t.Fatal("缺少审批决策时应该读取失败")
	}
}
