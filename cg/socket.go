package cg

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/Bananenpro/log"
	"github.com/gorilla/websocket"
)

type socket struct {
	id     string
	server *Server
	player *Player
	conn   *websocket.Conn
}

var (
	ErrInvalidMessageType = errors.New("invalid message type")
	ErrEncodeFailed       = errors.New("failed to encode json object")
	ErrDecodeFailed       = errors.New("failed to decode event")
)

func (s *socket) handleConnection() {
	for {
		event, err := s.receiveEvent()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Tracef("Socket %s disconnected.", s.id)
				break
			} else if err == ErrDecodeFailed || err == ErrInvalidMessageType {
				s.sendError(err.Error())
			} else {
				log.Warnf("Socket %s disconnected unexpectedly: %s", s.id, err)
				break
			}
		}

		switch event.Name {
		case EventCreateGame:
			err = s.createGame(event)
		case EventJoinGame:
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
		s.player.disconnectSocket(s.id)
	} else {
		s.server.removeSocket(s.id)
	}
}

func (s *socket) createGame(event Event) error {
	var data EventCreateGameData
	err := event.UnmarshalData(&data)
	if err != nil {
		return err
	}

	gameId, err := s.server.createGame(data.Public)
	if err != nil {
		return err
	}

	s.sendEvent("server", EventCreatedGame, EventCreatedGameData{
		GameId: gameId,
	})

	if data.Public {
		log.Tracef("Socket %s created a new public game: %s", s.id, gameId)
	} else {
		log.Tracef("Socket %s created a new private game: %s", s.id, gameId)
	}

	return nil
}

func (s *socket) joinGame(event Event) error {
	if s.player != nil {
		return errors.New("already joined")
	}

	var data EventJoinGameData
	err := event.UnmarshalData(&data)
	if err != nil {
		return err
	}

	err = s.server.joinGame(data.GameId, data.Username, s)
	if err != nil {
		return err
	}

	err = s.sendEvent("server", EventPlayerSecret, EventPlayerSecretData{
		Secret: s.player.Secret,
	})
	if err != nil {
		return err
	}

	return s.sendGameInfo()
}

func (s *socket) connect(event Event) error {
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

	log.Tracef("Socket %s connected to player %s (%s).", s.id, s.player.Id, s.player.Username)

	return s.sendGameInfo()
}

func (s *socket) disconnect() {
	if s.player != nil && s.player.gameId != "" {
		s.sendEvent(s.player.Id, EventDisconnected, EventDisconnectedData{})
	}

	err := s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "disconnect"), time.Now().Add(5*time.Second))
	if err != nil {
		s.conn.Close()
	}
}

func (s *socket) receiveEvent() (Event, error) {
	msgType, msg, err := s.conn.ReadMessage()
	if err != nil {
		return Event{}, err
	}
	if msgType != websocket.TextMessage {
		return Event{}, ErrInvalidMessageType
	}

	log.Tracef("Received '%s' from %s.", string(msg), s.id)

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

func (s *socket) sendGameInfo() error {
	if s.player == nil || s.player.gameId == "" {
		return errors.New("not in game")
	}
	s.server.gamesLock.RLock()
	players := s.server.games[s.player.gameId].players

	playersMap := make(map[string]string, len(players))

	for id, player := range players {
		playersMap[id] = player.Username
	}
	s.server.gamesLock.RUnlock()

	return s.sendEvent("server", EventGameInfo, EventGameInfoData{
		Players: playersMap,
	})
}

func (s *socket) send(message []byte) error {
	log.Tracef("Sending '%s' to %s...", string(message), s.id)
	return s.conn.WriteMessage(websocket.TextMessage, message)
}

func (s *socket) sendEvent(origin string, eventName EventName, eventData interface{}) error {
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

	log.Tracef("Sending '%s' to %s...", string(jsonData), s.id)

	return s.conn.WriteMessage(websocket.TextMessage, jsonData)
}

func (s *socket) sendError(reason string) error {
	log.Errorf("Error with socket %s: %s", s.id, reason)

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
