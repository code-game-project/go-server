package cg

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/Bananenpro/log"
	"github.com/gorilla/websocket"
)

type Socket struct {
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

// Send sends the event to the socket.
func (s *Socket) Send(event EventName, data any) error {
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

	return s.send(jsonData)
}

func (s *Socket) handleConnection() {
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
				log.Tracef("Socket %s disconnected.", s.Id)
				break
			} else if err == ErrDecodeFailed || err == ErrInvalidMessageType {
				log.Tracef("Socket %s failed to decode command: %s", s.Id, err)
			} else {
				log.Tracef("Socket %s disconnected unexpectedly: %s", s.Id, err)
				break
			}
		}

		if s.player != nil {
			err = s.player.handleCommand(cmd)
			if err != nil {
				log.Tracef("Error while executing `%s` command: %s", cmd.Name, err)
			}
		} else {
			log.Tracef("Socket %s sent an unexpected command: %s", s.Id, cmd.Name)
		}

		if err != nil {
			log.Tracef("Unexpected error in socket %s: %s", s.Id, err)
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

func (s *Socket) ping() {
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

func (s *Socket) disconnect() {
	close(s.done)
	s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "disconnect"), time.Now().Add(5*time.Second))
	s.conn.Close()
}

func (s *Socket) receiveCommand() (Command, error) {
	msgType, msg, err := s.conn.ReadMessage()
	if err != nil {
		return Command{}, err
	}
	if msgType != websocket.TextMessage {
		return Command{}, ErrInvalidMessageType
	}

	log.Tracef("Received '%s' from %s.", string(msg), s.Id)

	var cmd Command
	err = json.Unmarshal(msg, &cmd)

	if err != nil || cmd.Name == "" {
		return Command{}, ErrDecodeFailed
	}

	return cmd, nil
}

func (s *Socket) send(message []byte) error {
	log.Tracef("Sending '%s' to %s...", string(message), s.Id)
	s.conn.SetWriteDeadline(time.Now().Add(s.server.config.WebsocketTimeout))
	return s.conn.WriteMessage(websocket.TextMessage, message)
}
