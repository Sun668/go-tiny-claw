package cli

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sync"

	"github.com/Sun668/go-tiny-claw/internal/reporter"
	runtimepkg "github.com/Sun668/go-tiny-claw/internal/runtime"
)

type TerminalSession struct {
	id       string
	manager  *runtimepkg.Manager
	runtime  *runtimepkg.Runtime
	reporter reporter.Reporter
	approval *TerminalApprovalHandler
	repl     *REPL

	closeOnce sync.Once
	closeErr  error
}

func NewTerminalSession(id string, manager *runtimepkg.Manager, reader *bufio.Reader, out io.Writer) (*TerminalSession, error) {
	if manager == nil {
		return nil, errors.New("RuntimeManager 不能为空")
	}
	if reader == nil {
		return nil, errors.New("终端读取器不能为空")
	}
	if out == nil {
		return nil, errors.New("终端输出不能为空")
	}

	approvalHandler, err := NewTerminalApprovalHandler(out)

	if err != nil {
		return nil, err
	}

	runtimeBundle, err := manager.Create(id, runtimepkg.RuntimeOptions{
		ApprovalHandler: approvalHandler,
		Reporter:        reporter.NewTerminalReporter(out),
	})
	if err != nil {
		return nil, err
	}

	repl := NewREPL(reader, out, runtimeBundle.Runtime, runtimeBundle.Reporter, approvalHandler)

	return &TerminalSession{
		id:       id,
		manager:  manager,
		runtime:  runtimeBundle.Runtime,
		reporter: runtimeBundle.Reporter,
		approval: approvalHandler,
		repl:     repl,
	}, nil
}

func (s *TerminalSession) Run(ctx context.Context) error {
	return s.repl.Run(ctx)
}

func (s *TerminalSession) Interrupt() {
	s.repl.Interrupt()
}

func (s *TerminalSession) Close() error {
	s.closeOnce.Do(func() {
		err := s.manager.Destroy(s.id)
		if err != nil && !errors.Is(err, runtimepkg.ErrRuntimeNotFound) {
			s.closeErr = err
			return
		}
		s.closeErr = nil
	})
	return s.closeErr
}
