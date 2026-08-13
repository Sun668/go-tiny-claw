package channel_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/channel"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
)

type blockingWriter struct {
	started chan struct{}
	block   chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	select {
	case <-w.started:
	default:
		close(w.started)
	}
	<-w.block
	return len(p), nil
}

func newTestSink(t *testing.T, output io.Writer, size int) *channel.JSONEventSink {
	t.Helper()

	writer, err := channel.NewMessageWriter(output)
	if err != nil {
		t.Fatalf("创建 MessageWriter 失败: %v", err)
	}

	sink, err := channel.NewJSONEventSinkWithCapacity(writer, size)
	if err != nil {
		t.Fatalf("创建 EventSink 失败: %v", err)
	}

	t.Cleanup(func() {
		sink.Close()
		waitDone := make(chan struct{})
		go func() {
			sink.Wait()
			close(waitDone)
		}()
		select {
		case <-waitDone:
		case <-time.After(2 * time.Second):
			t.Error("超时：EventSink loop 未退出")
		}
	})

	return sink
}

func TestEventSinkPublishSucceedsWhenQueueHasSpace(t *testing.T) {
	sink := newTestSink(t, io.Discard, 4)

	err := sink.Publish(context.Background(), reporter.Event{
		Type: reporter.EventThinking,
	})
	if err != nil {
		t.Fatalf("队列有空位时 Publish 应成功: %v", err)
	}
}

func TestEventSinkPublishUnblocksOnContextCancel(t *testing.T) {
	writer := &blockingWriter{
		started: make(chan struct{}),
		block:   make(chan struct{}),
	}
	sink := newTestSink(t, writer, 1)
	t.Cleanup(func() {
		close(writer.block)
	})

	if err := sink.Publish(context.Background(), reporter.Event{
		Type: reporter.EventThinking,
	}); err != nil {
		t.Fatalf("第一条事件入队失败: %v", err)
	}

	select {
	case <-writer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("超时：writer 未开始阻塞")
	}

	if err := sink.Publish(context.Background(), reporter.Event{
		Type: reporter.EventTextDelta,
	}); err != nil {
		t.Fatalf("第二条事件填满队列失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- sink.Publish(ctx, reporter.Event{
			Type: reporter.EventTextCompleted,
		})
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("队列满时取消应返回 context.Canceled，实际: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("超时：Publish 未因 ctx 取消返回")
	}
}

func TestEventSinkPublishAfterClose(t *testing.T) {
	sink := newTestSink(t, io.Discard, 4)
	sink.Close()
	sink.Close()

	err := sink.Publish(context.Background(), reporter.Event{
		Type: reporter.EventThinking,
	})
	if !errors.Is(err, channel.ErrEventSinkClosed) {
		t.Fatalf("关闭后 Publish 应返回 ErrEventSinkClosed，实际: %v", err)
	}
}

func TestEventSinkWaitReturnsAfterWriteUnblocks(t *testing.T) {
	writer := &blockingWriter{
		started: make(chan struct{}),
		block:   make(chan struct{}),
	}
	sink := newTestSink(t, writer, 1)
	t.Cleanup(func() {
		select {
		case <-writer.block:
		default:
			close(writer.block)
		}
	})

	if err := sink.Publish(context.Background(), reporter.Event{
		Type: reporter.EventThinking,
	}); err != nil {
		t.Fatalf("入队失败: %v", err)
	}

	select {
	case <-writer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("超时：writer 未开始阻塞")
	}

	sink.Close()

	waitDone := make(chan struct{})
	go func() {
		sink.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		t.Fatal("Write 仍阻塞时 Wait 不应返回")
	case <-time.After(50 * time.Millisecond):
	}

	close(writer.block)

	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("超时：Write 返回后 Wait 应结束")
	}
}
