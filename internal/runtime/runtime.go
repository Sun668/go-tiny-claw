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
	return <-task.Done()
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
	done := make(chan error, 1)

	go func() {
		defer close(done)
		done <- r.runner.Run(runCtx, r.session, rep)
	}()

	return &Task{
		done:   done,
		cancel: cancel,
	}, nil
}

func (r *Runtime) Clear() {
	r.session.Clear()
}
