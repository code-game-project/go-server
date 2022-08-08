package cg

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gorilla/websocket"
)

type GameSocket struct {
	Id           string
	server       *Server
	player       *Player
	spectateGame *Game
	conn         *websocket.Conn
	done         chan struct{}
}

var (
	ErrInvalidMessageType = errors.New("invalid message type")
	ErrEncodeFailed       = errors.New("failed to encode json object")
	ErrDecodeFailed       = errors.New("failed to decode event")
)

// Send sends the event the socket.
func (s *GameSocket) Send(event EventName, data any) error {
	e := Event{
		Name: event,
	}
	err := e.marshalData(data)
	if err != nil {
		return err
	}

	jsonData, err := json.Marshal(e)
	if err != nil {
		return err
	}

	if s.player != nil {
		s.player.Log.TraceData(e, "Sending '%s' event to socket %s...", e.Name, s.Id)
	}

	s.send(jsonData)
	return nil
}

func (s *GameSocket) handleConnection() {
	s.done = make(chan struct{})

	s.conn.SetReadDeadline(time.Now().Add(s.server.config.WebsocketTimeout))
	s.conn.SetPongHandler(func(string) error {
		s.conn.SetReadDeadline(time.Now().Add(s.server.config.WebsocketTimeout))
		return nil
	})

	go s.ping()

	for {
		cmd, err := s.receiveCommand()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.server.log.Trace("Socket %s disconnected.", s.Id)
				break
			} else if err == ErrDecodeFailed || err == ErrInvalidMessageType {
				s.logger().Error("Socket %s failed to decode command: %s", s.Id, err)
			} else {
				s.logger().Warning("Socket %s disconnected unexpectedly: %s", s.Id, err)
				break
			}
		}

		if s.player != nil {
			s.player.handleCommand(cmd)
		} else {
			s.logger().Warning("Socket %s sent an unexpected command: %s", s.Id, cmd.Name)
		}
	}

	if s.player != nil {
		s.player.disconnectSocket(s.Id)
	} else {
		if s.spectateGame != nil {
			s.spectateGame.removeSpectator(s.Id)
		}
	}
}

func (s *GameSocket) ping() {
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

func (s *GameSocket) disconnect() {
	close(s.done)
	s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "disconnect"), time.Now().Add(5*time.Second))
	s.conn.Close()
}

func (s *GameSocket) receiveCommand() (Command, error) {
	msgType, msg, err := s.conn.ReadMessage()
	if err != nil {
		return Command{}, err
	}
	if msgType != websocket.TextMessage {
		return Command{}, ErrInvalidMessageType
	}

	var cmd Command
	err = json.Unmarshal(msg, &cmd)

	if err != nil || cmd.Name == "" {
		return Command{}, ErrDecodeFailed
	}

	s.logger().TraceData(cmd, "Received '%s' command from socket %s.", cmd.Name, s.Id)

	return cmd, nil
}

func (s *GameSocket) send(message []byte) error {
	s.conn.SetWriteDeadline(time.Now().Add(s.server.config.WebsocketTimeout))
	return s.conn.WriteMessage(websocket.TextMessage, message)
}

func (s *GameSocket) logger() *Logger {
	if s.player != nil {
		return s.player.Log
	} else if s.spectateGame != nil {
		return s.spectateGame.Log
	} else {
		return s.server.log
	}
}
