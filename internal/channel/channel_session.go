package channel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	runtimepkg "github.com/Sun668/go-tiny-claw/internal/runtime"
)

type ChannelSession struct {
	id       string
	conn     io.ReadWriteCloser
	manager  *runtimepkg.Manager
	runtime  *runtimepkg.Runtime
	reader   *MessageReader
	writer   *MessageWriter
	reporter reporter.Reporter
	events   *JSONEventSink
	approval *ChannelApprovalHandler

	closeOnce sync.Once
	closeErr  error
}

func NewChannelSession(id string, conn io.ReadWriteCloser, manager *runtimepkg.Manager) (*ChannelSession, error) {
	if conn == nil {
		return nil, errors.New("通道连接不能为空")
	}

	if manager == nil {
		return nil, errors.New("RuntimeManager 不能为空")
	}

	reader, err := NewMessageReader(conn)
	if err != nil {
		return nil, err
	}

	writer, err := NewMessageWriter(conn)
	if err != nil {
		return nil, err
	}

	eventSink, err := NewJSONEventSinkWithWriter(writer)
	if err != nil {
		return nil, err
	}

	channelApproval, err := NewChannelApprovalHandler(eventSink)
	if err != nil {
		return nil, err
	}

	bundle, err := manager.Create(id, runtimepkg.RuntimeOptions{
		ApprovalHandler: channelApproval,
		Reporter:        reporter.NewJSONReporter(eventSink),
	})

	if err != nil {
		return nil, err
	}

	return &ChannelSession{
		id:       id,
		conn:     conn,
		manager:  manager,
		runtime:  bundle.Runtime,
		reader:   reader,
		writer:   writer,
		reporter: bundle.Reporter,
		events:   eventSink,
		approval: channelApproval,
	}, nil
}

func (s *ChannelSession) ID() string {
	return s.id
}

func (s *ChannelSession) Runtime() *runtimepkg.Runtime {
	return s.runtime
}

func (s *ChannelSession) Run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		message, err := s.reader.Read()
		if err != nil {
			return err
		}

		switch message.Type {
		case MessagePrompt:
			if err := s.startTask(ctx, message.Content); err != nil {
				s.publishError(ctx, err)
			}
		case MessageInterrupt:
			s.runtime.Cancel()
		case MessageApprovalResponse:
			decision := approval.Decision(message.Decision)
			if err := s.approval.Respond(message.RequestID, decision); err != nil {
				s.publishError(ctx, err)
			}
		case MessagePing:
			if err := s.writer.Write(Message{Type: MessagePong}); err != nil {
				return err
			}
		case MessageClose:
			return nil
		default:
			return fmt.Errorf("不支持的消息类型: %s", message.Type)
		}
	}
}

func (s *ChannelSession) startTask(ctx context.Context, prompt string) error {
	if s.runtime.IsRunning() {
		return runtimepkg.ErrTaskRunning
	}

	task, err := s.runtime.Start(ctx, prompt, s.reporter)
	if err != nil {
		return err
	}

	go func() {
		err := task.Wait()
		event := reporter.Event{}

		switch task.Status() {
		case runtimepkg.TaskCompleted:
			event.Type = reporter.EventTaskCompleted
		case runtimepkg.TaskCanceled:
			event.Type = reporter.EventTaskCanceled
		case runtimepkg.TaskTimedOut:
			event.Type = reporter.EventTaskTimedOut
			if err != nil {
				event.Error = err.Error()
				event.IsError = true
			}
		case runtimepkg.TaskFailed:
			event.Type = reporter.EventTaskFailed
			if err != nil {
				event.Error = err.Error()
				event.IsError = true
			}
		default:
			event.Type = reporter.EventTaskFailed
			if err != nil {
				event.Error = err.Error()
			} else {
				event.Error = "任务结束状态未知"
			}
			event.IsError = true
		}

		_ = s.events.Publish(context.Background(), event)
	}()

	return err
}

func (s *ChannelSession) publishError(ctx context.Context, err error) {
	if err == nil {
		return
	}

	_ = s.events.Publish(ctx, reporter.Event{
		Type:    reporter.EventError,
		Error:   err.Error(),
		IsError: true,
	})
}

func (s *ChannelSession) Interrupt() {
	s.runtime.Cancel()
}

func (s *ChannelSession) Close() error {
	s.closeOnce.Do(func() {
		destroyErr := s.manager.Destroy(s.id)
		s.events.Close()
		closeErr := s.conn.Close()

		if destroyErr != nil &&
			!errors.Is(destroyErr, runtimepkg.ErrRuntimeNotFound) {
			s.closeErr = destroyErr
			return
		}

		s.closeErr = closeErr
	})

	return s.closeErr
}
