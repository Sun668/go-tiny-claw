package runtime

import (
	"context"
	"errors"
	"strings"

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
}

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

	r.session.Append(schema.Message{
		Role:    schema.RoleUser,
		Content: prompt,
	})

	runCtx, cancel := context.WithCancel(parent)

	task := newTask(cancel)

	go func() {
		err := r.runner.Run(runCtx, r.session, rep)
		task.finish(err)
	}()

	return task, nil
}

func (r *Runtime) Clear() {
	r.session.Clear()
}
