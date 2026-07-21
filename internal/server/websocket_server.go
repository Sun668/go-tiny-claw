package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/Sun668/go-tiny-claw/internal/channel"
	runtimepkg "github.com/Sun668/go-tiny-claw/internal/runtime"
	"github.com/gorilla/websocket"
)

type WebSocketServer struct {
	manager  *runtimepkg.Manager
	upgrader websocket.Upgrader

	sequence uint64
	ctx      context.Context

	httpServer *http.Server
	sessions   map[string]*channel.ChannelSession
	mu         sync.RWMutex
	closeOnce  sync.Once
	closeErr   error
}

func NewWebSocketServer(manager *runtimepkg.Manager) *WebSocketServer {
	return &WebSocketServer{
		manager: manager,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
		sessions: make(map[string]*channel.ChannelSession),
	}
}

func (s *WebSocketServer) Serve(ctx context.Context, address string) error {
	s.ctx = ctx
	s.httpServer = &http.Server{
		Addr:    address,
		Handler: http.HandlerFunc(s.handleHTTP),
	}

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func (s *WebSocketServer) handleHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/ws" {
		http.NotFound(writer, request)
		return
	}

	conn, err := s.upgrader.Upgrade(writer, request, nil)
	if err != nil {
		log.Printf("WebSocket 升级失败: %v", err)
		return
	}

	sessionID := "ws-channel-session-" + formatSequence(atomic.AddUint64(&s.sequence, 1))
	stream := newWebsocketConn(conn)
	channelSession, err := channel.NewChannelSession(sessionID, stream, s.manager)
	if err != nil {
		log.Printf("创建 WebSocket ChannelSession 失败: %v", err)
		_ = stream.Close()
		return
	}

	if err := s.addSession(sessionID, channelSession); err != nil {
		log.Printf("注册 WebSocket ChannelSession 失败: %v", err)
		_ = channelSession.Close()
		return
	}

	defer s.removeSession(sessionID)
	defer func() {
		if err := channelSession.Close(); err != nil {
			log.Printf("关闭 WebSocket ChannelSession 失败: %v", err)
		}
	}()

	if err := channelSession.Run(s.ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("WebSocket [%s] 运行失败: %v", sessionID, err)
	}
}

func (s *WebSocketServer) addSession(sessionID string, session *channel.ChannelSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[sessionID]; exists {
		return ErrChannelSessionExists
	}

	s.sessions[sessionID] = session
	return nil
}

func (s *WebSocketServer) removeSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func (s *WebSocketServer) Close() error {
	s.closeOnce.Do(func() {
		if s.httpServer != nil {
			s.closeErr = s.httpServer.Close()
		}

		s.mu.RLock()
		sessions := make([]*channel.ChannelSession, 0, len(s.sessions))
		for _, session := range s.sessions {
			sessions = append(sessions, session)
		}
		s.mu.RUnlock()

		for _, session := range sessions {
			if err := session.Close(); err != nil && s.closeErr == nil {
				s.closeErr = err
			}
		}
	})

	return s.closeErr
}

func formatSequence(sequence uint64) string {
	if sequence == 0 {
		return "0"
	}

	var digits [20]byte
	index := len(digits)
	for sequence > 0 {
		index--
		digits[index] = byte('0' + sequence%10)
		sequence /= 10
	}

	return string(digits[index:])
}
