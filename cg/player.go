package cg

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

type Player struct {
	Id       string
	Username string
	Secret   string

	gameId      string
	socketsLock sync.RWMutex
	sockets     map[string]*socket
	server      *Server
}

func (p *Player) Send(origin string, eventName EventName, eventData any) error {
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

	p.socketsLock.RLock()
	defer p.socketsLock.RUnlock()
	for _, socket := range p.sockets {
		err := socket.send(jsonData)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Player) handleEvent(event Event) error {
	switch event.Name {
	case EventLeaveGame:
		return p.server.leaveGame(p)
	default:
		if p.gameId != "" {
			p.server.gamesLock.RLock()
			game, ok := p.server.games[p.gameId]
			p.server.gamesLock.RUnlock()
			if ok {
				return game.game.OnPlayerEvent(p, event)
			}
		}
		return errors.New(fmt.Sprintf("unexpected event: %s", event.Name))
	}
}

func (p *Player) addSocket(socket *socket) {
	p.socketsLock.Lock()
	p.sockets[socket.id] = socket
	p.socketsLock.Unlock()
}

func (p *Player) disconnectSocket(id string) {
	p.socketsLock.Lock()

	socket, ok := p.sockets[id]
	if ok {
		socket.disconnect()
		delete(p.sockets, id)
	}

	p.socketsLock.Unlock()

	p.server.removeSocket(id)
}
