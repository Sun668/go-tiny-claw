package channel

import (
	"context"
	"errors"
	"io"

	"github.com/Sun668/go-tiny-claw/internal/reporter"
)

type JSONEventSink struct {
	writer *MessageWriter
}

func NewJSONEventSink(writer io.Writer) (*JSONEventSink, error) {
	messageWriter, err := NewMessageWriter(writer)
	if err != nil {
		return nil, err
	}

	return NewJSONEventSinkWithWriter(messageWriter)
}

func NewJSONEventSinkWithWriter(writer *MessageWriter) (*JSONEventSink, error) {
	if writer == nil {
		return nil, errors.New("消息写入器不能为空")
	}

	return &JSONEventSink{writer: writer}, nil
}

func (s *JSONEventSink) Publish(
	ctx context.Context,
	event reporter.Event,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.writer.Write(event)
}
