package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

type TaskStatus string

const (
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskCanceled  TaskStatus = "canceled"
	TaskTimedOut  TaskStatus = "timed_out"
	TaskFailed    TaskStatus = "failed"
)

// StatusFromError 将 Runner 返回的 error 映射为唯一终态语义。
// nil → completed；Canceled → canceled；DeadlineExceeded → timed_out；其他 → failed。
func StatusFromError(err error) TaskStatus {
	switch {
	case err == nil:
		return TaskCompleted
	case errors.Is(err, context.Canceled):
		return TaskCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return TaskTimedOut
	default:
		return TaskFailed
	}
}

type Task struct {
	id     string
	done   chan struct{}
	cancel context.CancelFunc

	mu     sync.RWMutex
	status TaskStatus
	err    error
}

var taskSequence uint64

func newTask(cancel context.CancelFunc) *Task {
	id := fmt.Sprintf(
		"task-%d",
		atomic.AddUint64(&taskSequence, 1),
	)
	return &Task{
		id:     id,
		done:   make(chan struct{}),
		cancel: cancel,
		status: TaskRunning,
	}
}

func (t *Task) ID() string {
	return t.id
}

func (t *Task) Done() <-chan struct{} {
	return t.done
}

func (t *Task) Cancel() {
	t.cancel()
}

func (t *Task) Wait() error {
	<-t.done

	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.err
}

func (t *Task) Status() TaskStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.status
}

func (t *Task) Err() error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.err
}

func (t *Task) finish(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.status != TaskRunning {
		return
	}

	t.err = err
	t.status = StatusFromError(err)

	close(t.done)
}
