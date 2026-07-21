package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"github.com/Sun668/go-tiny-claw/internal/channel"
	runtimepkg "github.com/Sun668/go-tiny-claw/internal/runtime"
)

var ErrChannelSessionExists = errors.New("ChannelSession 已经存在")

type TCPServer struct {
	listener net.Listener
	manager  *runtimepkg.Manager
	sequence uint64

	sessions map[string]*channel.ChannelSession
	mu       sync.RWMutex

	closeOnce sync.Once
	closeErr  error
}

func NewTCPServer(
	listener net.Listener,
	manager *runtimepkg.Manager,
) *TCPServer {
	return &TCPServer{
		listener: listener,
		manager:  manager,
		sessions: make(map[string]*channel.ChannelSession),
	}
}

func (s *TCPServer) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return err
		}

		sessionID := fmt.Sprintf(
			"tcp-channel-session-%d",
			atomic.AddUint64(&s.sequence, 1),
		)

		go s.handleConnection(
			ctx,
			sessionID,
			conn,
		)
	}
}

func (s *TCPServer) handleConnection(
	parent context.Context,
	sessionID string,
	conn net.Conn,
) {
	channelSession, err := channel.NewChannelSession(
		sessionID,
		conn,
		s.manager,
	)
	if err != nil {
		log.Printf("创建 ChannelSession 失败: %v", err)
		_ = conn.Close()
		return
	}

	if err := s.addSession(sessionID, channelSession); err != nil {
		log.Printf("注册 ChannelSession 失败: %v", err)
		_ = channelSession.Close()
		return
	}

	defer s.removeSession(sessionID)
	defer func() {
		if err := channelSession.Close(); err != nil {
			log.Printf("关闭 ChannelSession 失败: %v", err)
		}
	}()

	err = channelSession.Run(parent)
	if err != nil && err != io.EOF {
		log.Printf(
			"Channel [%s] 运行失败: %v",
			sessionID,
			err,
		)
	}
}

func (s *TCPServer) addSession(
	sessionID string,
	channelSession *channel.ChannelSession,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[sessionID]; exists {
		return ErrChannelSessionExists
	}

	s.sessions[sessionID] = channelSession
	return nil
}

func (s *TCPServer) removeSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionID)
}

func (s *TCPServer) Close() error {
	s.closeOnce.Do(func() {
		listenerErr := s.listener.Close()

		s.mu.RLock()
		sessions := make([]*channel.ChannelSession, 0, len(s.sessions))

		for _, channelSession := range s.sessions {
			sessions = append(sessions, channelSession)
		}
		s.mu.RUnlock()

		var sessionErr error

		for _, channelSession := range sessions {
			if err := channelSession.Close(); err != nil &&
				sessionErr == nil {
				sessionErr = err
			}
		}

		if listenerErr != nil &&
			!errors.Is(listenerErr, net.ErrClosed) {
			s.closeErr = listenerErr
			return
		}

		s.closeErr = sessionErr
	})

	return s.closeErr
}
