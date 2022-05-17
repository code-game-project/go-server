package cg

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/Bananenpro/log"
	"github.com/gorilla/websocket"
)

type Socket struct {
	Id     string
	server *Server
	player *Player
	conn   *websocket.Conn
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
				log.Warnf("Socket %s disconnected unexpectedly: %s", s.Id, err)
				break
			}
		}

		switch event.Name {
		case EventJoin:
			err = s.joinGame(event)
		case EventConnect:
			err = s.connect(event)
		default:
			if s.player != nil {
				err = s.player.handleEvent(event)
			} else {

			}
		}

		if err != nil {
			s.sendError(err.Error())
		}
	}
	if s.player != nil {
		s.player.disconnectSocket(s.Id)
	} else {
		s.server.removeSocket(s.Id)
	}
}

func (s *Socket) joinGame(event Event) error {
	if s.player != nil {
		return errors.New("already joined")
	}

	var data EventJoinData
	err := event.UnmarshalData(&data)
	if err != nil {
		return err
	}

	err = s.server.joinGame(data.GameId, data.Username, s)
	if err != nil {
		return err
	}

	err = s.Send(s.player.Id, EventJoined, EventJoinedData{
		Secret: s.player.Secret,
	})
	if err != nil {
		return err
	}

	return s.sendGameInfo()
}

func (s *Socket) connect(event Event) error {
	if s.player != nil {
		return errors.New("already connected")
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
	if s.player == nil || s.player.game == nil {
		return errors.New("not in game")
	}

	return s.Send("server", EventInfo, EventInfoData{
		Players: s.player.game.playerUsernameMap(),
	})
}

func (s *Socket) send(message []byte) error {
	log.Tracef("Sending '%s' to %s...", string(message), s.Id)
	return s.conn.WriteMessage(websocket.TextMessage, message)
}

func (s *Socket) sendError(reason string) error {
	log.Errorf("Error with socket %s: %s", s.Id, reason)

	event := Event{
		Name: EventError,
	}
	err := event.marshalData(EventErrorData{
		Reason: reason,
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
