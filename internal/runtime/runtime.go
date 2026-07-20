package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"

	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type AgentRunner interface {
	Run(ctx context.Context, session *ctxpkg.Session, rep reporter.Reporter) error
}

type Runtime struct {
	runner  AgentRunner
	session *ctxpkg.Session

	mu     sync.Mutex
	active *Task
}

var ErrTaskRunning = errors.New("当前会话已有任务运行")

var ErrClearWhileRunning = errors.New("当前任务正在运行，不能清空会话")

func NewRuntime(runner AgentRunner, session *ctxpkg.Session) *Runtime {
	return &Runtime{
		runner:  runner,
		session: session,
	}
}

func (r *Runtime) Run(ctx context.Context, prompt string, rep reporter.Reporter) error {
	task, err := r.Start(ctx, prompt, rep)
	if err != nil {
		return err
	}
	return task.Wait()
}

func (r *Runtime) Start(parent context.Context, prompt string, rep reporter.Reporter) (*Task, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, errors.New("用户提示不能为空")
	}

	if parent == nil {
		return nil, errors.New("父上下文不能为空")
	}

	if err := parent.Err(); err != nil {
		return nil, err
	}

	r.mu.Lock()

	if r.active != nil {
		r.mu.Unlock()
		return nil, ErrTaskRunning
	}

	r.session.Append(schema.Message{
		Role:    schema.RoleUser,
		Content: prompt,
	})

	runCtx, cancel := context.WithCancel(parent)

	task := newTask(cancel)

	r.active = task
	r.mu.Unlock()

	go r.execute(runCtx, task, rep)

	return task, nil
}

func (r *Runtime) execute(ctx context.Context, task *Task, rep reporter.Reporter) {
	err := r.runner.Run(ctx, r.session, rep)
	task.finish(err)
	r.mu.Lock()
	if r.active == task {
		r.active = nil
	}
	r.mu.Unlock()
}

func (r *Runtime) Clear() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.active != nil {
		return ErrClearWhileRunning
	}

	r.session.Clear()
	return nil
}

func (r *Runtime) Cancel() bool {
	r.mu.Lock()
	task := r.active
	r.mu.Unlock()

	if task == nil {
		return false
	}

	task.Cancel()
	return true
}

func (r *Runtime) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.active != nil
}

func (r *Runtime) SessionID() string {
	if r == nil || r.session == nil {
		return ""
	}

	return r.session.ID
}
