package cg

import (
	"time"

	"github.com/gorilla/websocket"
)

type debugSocket struct {
	id     string
	server *Server
	logger *Logger
	conn   *websocket.Conn
	done   chan struct{}

	severities map[DebugSeverity]bool
}

type DebugSeverity string

const (
	DebugError   = "error"
	DebugWarning = "warning"
	DebugInfo    = "info"
	DebugTrace   = "trace"
)

func (s *debugSocket) send(message []byte) error {
	s.conn.SetWriteDeadline(time.Now().Add(s.server.config.WebsocketTimeout))
	return s.conn.WriteMessage(websocket.TextMessage, message)
}

func (s *debugSocket) handleConnection() {
	s.done = make(chan struct{})

	s.conn.SetReadDeadline(time.Now().Add(s.server.config.WebsocketTimeout))
	s.conn.SetPongHandler(func(string) error {
		s.conn.SetReadDeadline(time.Now().Add(s.server.config.WebsocketTimeout))
		return nil
	})

	go s.ping()

	for {
		_, _, err := s.conn.ReadMessage()
		if err != nil {
			break
		}
	}
	if s.logger != nil {
		s.logger.disconnectDebugSocket(s.id)
	}
}

func (s *debugSocket) ping() {
	ticker := time.NewTicker((s.server.config.WebsocketTimeout * 9) / 10)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(30*time.Second))
		case <-s.done:
			return
		}
	}
}

func (s *debugSocket) disconnect() {
	close(s.done)
	s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "disconnect"), time.Now().Add(5*time.Second))
	s.conn.Close()
}
