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

	log.Tracef("Sending '%s' to %s...", string(jsonData), s.Id)

	return s.conn.WriteMessage(websocket.TextMessage, jsonData)
}

func (s *Socket) handleConnection() {
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
		case EventJoin:
			err = s.joinGame(event)
		case EventConnect:
			err = s.connect(event)
		case EventSpectate:
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

func (s *Socket) joinGame(event Event) error {
	if s.player != nil {
		return errors.New("already joined")
	}
	if s.spectateGame != nil {
		return errors.New("already spectating a game")
	}

	var data EventJoinData
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

	var data EventConnectData
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

	var data EventSpectateData
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

	return s.sendGameInfo()
}

func (s *Socket) disconnect() {
	err := s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "disconnect"), time.Now().Add(5*time.Second))
	if err != nil {
		s.conn.Close()
	}
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
		return s.Send("server", EventInfo, EventInfoData{
			Players: s.player.game.playerUsernameMap(),
		})
	}

	if s.spectateGame != nil {
		return s.Send("server", EventInfo, EventInfoData{
			Players: s.spectateGame.playerUsernameMap(),
		})
	}

	return errors.New("not in game")
}

func (s *Socket) send(message []byte) error {
	log.Tracef("Sending '%s' to %s...", string(message), s.Id)
	return s.conn.WriteMessage(websocket.TextMessage, message)
}

func (s *Socket) sendError(message string) error {
	log.Tracef("Error with socket %s: %s", s.Id, message)

	event := Event{
		Name: EventError,
	}
	err := event.marshalData(EventErrorData{
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

	return s.conn.WriteMessage(websocket.TextMessage, jsonData)
}
