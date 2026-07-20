package channel

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sync"

	"github.com/Sun668/go-tiny-claw/internal/cli"
	runtimepkg "github.com/Sun668/go-tiny-claw/internal/runtime"
)

type ChannelSession struct {
	id      string
	conn    io.ReadWriteCloser
	manager *runtimepkg.Manager
	runtime *runtimepkg.Runtime
	repl    *cli.REPL

	closeOnce sync.Once
	closeErr  error
}

func NewChannelSession(id string, conn io.ReadWriteCloser, manager *runtimepkg.Manager) (*ChannelSession, error) {
	if conn == nil {
		return nil, errors.New("Terminal 连接不能为空")
	}

	if manager == nil {
		return nil, errors.New("RuntimeManager 不能为空")
	}

	reader := bufio.NewReader(conn)

	bundle, err := manager.Create(id, reader, conn)

	if err != nil {
		return nil, err
	}

	repl := cli.NewREPL(reader, conn, bundle.Runtime, bundle.Reporter)

	return &ChannelSession{
		id:      id,
		conn:    conn,
		manager: manager,
		runtime: bundle.Runtime,
		repl:    repl,
	}, nil
}

func (s *ChannelSession) ID() string {
	return s.id
}

func (s *ChannelSession) Runtime() *runtimepkg.Runtime {
	return s.runtime
}

func (t *ChannelSession) Run(ctx context.Context) error {
	return t.repl.Run(ctx)
}

func (t *ChannelSession) Interrupt() {
	t.repl.Interrupt()
}

func (s *ChannelSession) Close() error {
	s.closeOnce.Do(func() {
		destroyErr := s.manager.Destroy(s.id)
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
