package cg

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Player struct {
	Id       string
	Username string
	Secret   string

	game   *Game
	server *Server

	socketsLock    sync.RWMutex
	sockets        map[string]*Socket
	socketCount    int
	lastConnection time.Time

	missedEventsLock sync.RWMutex
	missedEvents     [][]byte
}

// Send sends the event to all sockets currently connected to the player.
// Events are added to a queue in case there are no sockets.
// The next socket to connect to the player will then receive the missed events.
func (p *Player) Send(event EventName, data any) error {
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

	p.socketsLock.RLock()
	defer p.socketsLock.RUnlock()
	for _, socket := range p.sockets {
		err := socket.send(jsonData)
		if err != nil {
			return err
		}
	}

	if len(p.sockets) == 0 {
		p.missedEventsLock.Lock()
		p.missedEvents = append(p.missedEvents, jsonData)
		p.missedEventsLock.Unlock()
	}

	return nil
}

// Leave leaves the game.
func (p *Player) Leave() error {
	return p.game.leave(p)
}

// SocketCount returns the amount of sockets currently connected to the player.
func (p *Player) SocketCount() int {
	p.socketsLock.RLock()
	defer p.socketsLock.RUnlock()
	return p.socketCount
}

func (p *Player) handleCommand(cmd Command) error {
	if p.game == nil {
		return errors.New(fmt.Sprintf("unexpected command: %s", cmd.Name))
	}
	p.game.cmdChan <- CommandWrapper{
		Origin: p,
		Cmd:    cmd,
	}
	return nil
}

func (p *Player) addSocket(socket *Socket) error {
	if p.server.config.MaxSocketsPerPlayer > 0 && p.SocketCount() >= p.server.config.MaxSocketsPerPlayer {
		return errors.New("max socket count reached for this player")
	}

	socket.player = p

	p.socketsLock.Lock()
	p.sockets[socket.Id] = socket
	p.socketCount++
	p.socketsLock.Unlock()

	p.missedEventsLock.Lock()
	if len(p.missedEvents) > 0 {
		for _, e := range p.missedEvents {
			socket.send(e)
		}
		p.missedEvents = make([][]byte, 0)
	}
	p.missedEventsLock.Unlock()
	return nil
}

func (p *Player) disconnectSocket(id string) {
	p.socketsLock.Lock()

	if socket, ok := p.sockets[id]; ok {
		socket.disconnect()
		delete(p.sockets, id)
		p.socketCount--
		p.lastConnection = time.Now()
	}

	p.socketsLock.Unlock()
}
