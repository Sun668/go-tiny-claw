package channel

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

type MessageType string

const (
	MessagePrompt           MessageType = "prompt"
	MessageInterrupt        MessageType = "interrupt"
	MessageClose            MessageType = "close"
	MessagePing             MessageType = "ping"
	MessagePong             MessageType = "pong"
	MessageApprovalResponse MessageType = "approval_response"
)

type Message struct {
	Type      MessageType `json:"type"`
	Content   string      `json:"content,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Decision  string      `json:"decision,omitempty"`
}

const MaxMessageSize = 64 * 1024

type MessageReader struct {
	reader *bufio.Reader
}

func NewMessageReader(input io.Reader) (*MessageReader, error) {
	if input == nil {
		return nil, errors.New("消息读取器输入不能为空")
	}

	return &MessageReader{reader: bufio.NewReader(input)}, nil
}

func (r *MessageReader) Read() (Message, error) {
	line, err := r.reader.ReadBytes('\n')

	if len(line) > MaxMessageSize {
		return Message{}, errors.New("消息长度超过限制")
	}

	if err != nil {
		return Message{}, err
	}

	line = bytes.TrimSpace(line)

	var message Message

	if err := json.Unmarshal(line, &message); err != nil {
		return Message{}, errors.New("消息格式错误")
	}

	switch message.Type {
	case MessagePrompt:
		if message.Content == "" {
			return Message{}, errors.New("提示不能为空")
		}
	case MessageInterrupt:
	case MessageClose:
	case MessagePing:
	case MessageApprovalResponse:
		if message.RequestID == "" {
			return Message{}, errors.New("审批响应缺少请求 ID")
		}
		if message.Decision == "" {
			return Message{}, errors.New("审批响应缺少决策")
		}
	case MessagePong:
	default:
		return Message{}, errors.New("不支持的消息类型")
	}

	return message, nil
}

type MessageWriter struct {
	encoder *json.Encoder
	mu      sync.Mutex
}

func NewMessageWriter(output io.Writer) (*MessageWriter, error) {
	if output == nil {
		return nil, errors.New("消息写入器输出不能为空")
	}

	return &MessageWriter{encoder: json.NewEncoder(output)}, nil
}

func (w *MessageWriter) Write(value any) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.encoder.Encode(value)
}
