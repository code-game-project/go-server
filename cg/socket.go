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
func (s *Socket) Send(origin string, eventName EventName, eventData any) error {
	event := Event{
		Name: eventName,
	}
	err := event.marshalData(eventData)
	if err != nil {
		return err
	}

	wrapper := eventWrapper{
		Origin: origin,
		Event:  event,
	}

	jsonData, err := json.Marshal(wrapper)
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
		event, err := s.receiveEvent()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Tracef("Socket %s disconnected.", s.Id)
				break
			} else if err == ErrDecodeFailed || err == ErrInvalidMessageType {
				s.sendError(err.Error())
			} else {
				log.Tracef("Socket %s disconnected unexpectedly: %s", s.Id, err)
				break
			}
		}

		switch event.Name {
		case JoinEvent:
			err = s.joinGame(event)
		case ConnectEvent:
			err = s.connect(event)
		case SpectateEvent:
			err = s.spectate(event)
		default:
			if s.player != nil {
				err = s.player.handleEvent(event)
			} else {
				log.Tracef("Socket %s sent an unexpected event: %s", s.Id, event.Name)
				err = errors.New("unexpected event")
			}
		}

		if err != nil {
			s.sendError(err.Error())
		}
	}
	if s.player != nil {
		s.player.disconnectSocket(s.Id)
	} else {
		if s.spectateGame != nil {
			s.spectateGame.removeSpectator(s.Id)
		}
		s.server.removeSocket(s.Id)
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

func (s *Socket) joinGame(event Event) error {
	if s.player != nil {
		return errors.New("already joined")
	}
	if s.spectateGame != nil {
		return errors.New("already spectating a game")
	}

	var data JoinEventData
	err := event.UnmarshalData(&data)
	if err != nil {
		return err
	}

	return s.server.joinGame(data.GameId, data.Username, s)
}

func (s *Socket) connect(event Event) error {
	if s.player != nil {
		return errors.New("already connected")
	}
	if s.spectateGame != nil {
		return errors.New("already spectating a game")
	}

	var data ConnectEventData
	err := event.UnmarshalData(&data)
	if err != nil {
		return err
	}

	err = s.server.connect(data.GameId, data.PlayerId, data.Secret, s)
	if err != nil {
		return err
	}

	log.Tracef("Socket %s connected to player %s (%s).", s.Id, s.player.Id, s.player.Username)
	return nil
}

func (s *Socket) spectate(event Event) error {
	if s.player != nil {
		return errors.New("already connected")
	}
	if s.spectateGame != nil {
		return errors.New("already spectating a game")
	}

	var data SpectateEventData
	err := event.UnmarshalData(&data)
	if err != nil {
		return err
	}

	game, ok := s.server.getGame(data.GameId)
	if !ok {
		return errors.New("game does not exist")
	}

	err = game.addSpectator(s)
	if err != nil {
		return err
	}
	s.spectateGame = game

	log.Tracef("Socket %s is now spectating game %s.", s.Id, game.Id)
	return nil
}

func (s *Socket) disconnect() {
	close(s.done)
	s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "disconnect"), time.Now().Add(5*time.Second))
	s.conn.Close()
}

func (s *Socket) receiveEvent() (Event, error) {
	msgType, msg, err := s.conn.ReadMessage()
	if err != nil {
		return Event{}, err
	}
	if msgType != websocket.TextMessage {
		return Event{}, ErrInvalidMessageType
	}

	log.Tracef("Received '%s' from %s.", string(msg), s.Id)

	var event Event
	err = json.Unmarshal(msg, &event)

	if err != nil {
		return Event{}, ErrDecodeFailed
	}
	if event.Name == "" {
		return Event{}, ErrDecodeFailed
	}

	return event, nil
}

func (s *Socket) sendGameInfo() error {
	if s.player != nil && s.player.game != nil {
		return s.Send("server", InfoEvent, InfoEventData{
			Players: s.player.game.playerUsernameMap(),
		})
	}

	if s.spectateGame != nil {
		return s.Send("server", InfoEvent, InfoEventData{
			Players: s.spectateGame.playerUsernameMap(),
		})
	}

	return errors.New("not in game")
}

func (s *Socket) send(message []byte) error {
	log.Tracef("Sending '%s' to %s...", string(message), s.Id)
	s.conn.SetWriteDeadline(time.Now().Add(s.server.config.WebsocketTimeout))
	return s.conn.WriteMessage(websocket.TextMessage, message)
}

func (s *Socket) sendError(message string) error {
	log.Tracef("Error with socket %s: %s", s.Id, message)

	event := Event{
		Name: ErrorEvent,
	}
	err := event.marshalData(ErrorEventData{
		Message: message,
	})
	if err != nil {
		return err
	}

	jsonData, err := json.Marshal(eventWrapper{
		Origin: "server",
		Event:  event,
	})
	if err != nil {
		return err
	}

	return s.send(jsonData)
}
