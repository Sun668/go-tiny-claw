package channel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/Sun668/go-tiny-claw/internal/reporter"
)

var ErrEventSinkClosed = errors.New("事件输出已关闭")

const DefaultEventQueueSize = 64

type queuedEvent struct {
	ctx   context.Context
	event reporter.Event
}

type JSONEventSink struct {
	writer    *MessageWriter
	queue     chan queuedEvent
	done      chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool
}

func NewJSONEventSinkWithWriter(writer *MessageWriter) (*JSONEventSink, error) {
	return NewJSONEventSinkWithCapacity(writer, DefaultEventQueueSize)
}

func NewJSONEventSinkWithCapacity(writer *MessageWriter, size int) (*JSONEventSink, error) {
	if writer == nil {
		return nil, errors.New("消息写入器不能为空")
	}
	if size <= 0 {
		size = DefaultEventQueueSize
	}

	sink := &JSONEventSink{
		writer: writer,
		queue:  make(chan queuedEvent, size),
		done:   make(chan struct{}),
	}
	go sink.loop()
	return sink, nil
}

func (s *JSONEventSink) loop() {
	for {
		select {
		case <-s.done:
			return
		case item := <-s.queue:
			if item.ctx.Err() != nil {
				continue
			}
			_ = s.writer.Write(item.event)
		}
	}
}

func (s *JSONEventSink) Publish(ctx context.Context, event reporter.Event) error {
	if s.closed.Load() {
		return ErrEventSinkClosed
	}

	item := queuedEvent{ctx: ctx, event: event}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return ErrEventSinkClosed
	case s.queue <- item:
		return nil
	}
}

func (s *JSONEventSink) Close() {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.done)
	})
}
