package server

import (
	"bytes"
	"errors"
	"io"
	"sync"

	"github.com/gorilla/websocket"
)

type websocketConn struct {
	conn *websocket.Conn

	readBuffer []byte
	readOffset int
	writeMu    sync.Mutex
}

func newWebsocketConn(conn *websocket.Conn) *websocketConn {
	return &websocketConn{conn: conn}
}

func (c *websocketConn) Read(p []byte) (int, error) {
	for c.readOffset >= len(c.readBuffer) {
		messageType, reader, err := c.conn.NextReader()
		if err != nil {
			return 0, err
		}

		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			return 0, errors.New("只支持文本或二进制消息")
		}

		message, err := io.ReadAll(reader)
		if err != nil {
			return 0, err
		}

		c.readBuffer = append(message[:0:0], message...)
		c.readBuffer = append(c.readBuffer, '\n')
		c.readOffset = 0
	}

	count := copy(p, c.readBuffer[c.readOffset:])
	c.readOffset += count
	return count, nil
}

func (c *websocketConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	payload := bytes.TrimSpace(p)
	if len(payload) == 0 {
		return len(p), nil
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return 0, err
	}

	return len(p), nil
}

func (c *websocketConn) Close() error {
	return c.conn.Close()
}
