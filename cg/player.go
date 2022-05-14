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

	game        *Game
	socketsLock sync.RWMutex
	sockets     map[string]*Socket
	server      *Server
}

// Send sends the event to all sockets currently connected to the player.
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
		return p.game.leave(p)
	default:
		if p.game == nil {
			return errors.New(fmt.Sprintf("unexpected event: %s", event.Name))
		}
		p.game.Events <- EventWrapper{
			Player: p,
			Event:  event,
		}
	}
	return nil
}

func (p *Player) addSocket(socket *Socket) {
	p.socketsLock.Lock()
	p.sockets[socket.Id] = socket
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
