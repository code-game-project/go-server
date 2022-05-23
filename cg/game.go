package cg

import (
	"errors"
	"sync"
	"time"

	"github.com/Bananenpro/log"
	"github.com/google/uuid"
)

type Game struct {
	Id string

	OnPlayerJoined          func(player *Player)
	OnPlayerLeft            func(player *Player)
	OnPlayerSocketConnected func(player *Player, socket *Socket)

	eventsChan chan EventWrapper

	public bool

	playersLock sync.RWMutex
	players     map[string]*Player

	spectatorsLock sync.RWMutex
	spectators     map[string]*Socket

	server *Server

	running bool

	socketCountLock sync.RWMutex
	socketCount     int
	lastConnection  time.Time
}

type EventWrapper struct {
	Player *Player
	Event  Event
}

func (s *Server) newGame(id string, public bool) *Game {
	return &Game{
		Id:             id,
		eventsChan:     make(chan EventWrapper, 10),
		public:         public,
		players:        make(map[string]*Player),
		spectators:     make(map[string]*Socket),
		server:         s,
		running:        true,
		lastConnection: time.Now(),
	}
}

// Send sends the event to all players currently in the game.
func (g *Game) Send(origin string, eventName EventName, eventData any) error {
	g.playersLock.RLock()
	defer g.playersLock.RUnlock()
	for _, p := range g.players {
		err := p.Send(origin, eventName, eventData)
		if err != nil {
			return err
		}
	}

	g.spectatorsLock.RLock()
	defer g.spectatorsLock.RUnlock()
	for _, s := range g.spectators {
		err := s.Send(origin, eventName, eventData)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetPlayer returns a player in the game by id.
func (g *Game) GetPlayer(playerId string) (*Player, bool) {
	g.playersLock.RLock()
	defer g.playersLock.RUnlock()
	player, ok := g.players[playerId]
	return player, ok
}

// NextEvent returns the next event in the queue or ok = false if there is none.
func (g *Game) NextEvent() (EventWrapper, bool) {
	select {
	case wrapper, ok := <-g.eventsChan:
		if ok {
			return wrapper, true
		} else {
			return EventWrapper{}, false
		}
	default:
		return EventWrapper{}, false
	}
}

// WaitForNextEvent waits for and then returns the next event in the queue or ok = false if the game has been closed.
func (g *Game) WaitForNextEvent() (EventWrapper, bool) {
	wrapper, ok := <-g.eventsChan
	return wrapper, ok
}

// Returns true if the game has not already been closed.
func (g *Game) Running() bool {
	return g.running
}

// Stop the game, disconnect all players and remove it from the server.
func (g *Game) Close() error {
	if !g.running {
		return nil
	}

	g.running = false

	g.server.removeGame(g)

	for _, p := range g.players {
		err := g.leave(p)
		if err != nil {
			log.Errorf("Couldn't disconnect player '%s' from game '%s': %s", p.Id, g.Id, err)
		}
	}

	close(g.eventsChan)

	log.Tracef("Removed game %s.", g.Id)

	return nil
}

func (g *Game) join(username string, joiningSocket *Socket) error {
	if g.server.config.KickInactivePlayerDelay > 0 {
		g.playersLock.RLock()
		for _, p := range g.players {
			p.socketsLock.RLock()
			if p.socketCount == 0 && time.Now().Sub(p.lastConnection) >= g.server.config.KickInactivePlayerDelay {
				g.playersLock.RUnlock()
				p.socketsLock.RUnlock()
				g.leave(p)
				g.playersLock.RLock()
			} else {
				p.socketsLock.RUnlock()
			}
		}
		g.playersLock.RUnlock()
	}

	playerId := uuid.NewString()
	player := &Player{
		Id:       playerId,
		Username: username,
		Secret:   generatePlayerSecret(),
		server:   g.server,
		sockets:  make(map[string]*Socket),
		game:     g,
	}

	joiningSocket.player = player

	g.playersLock.Lock()
	g.players[playerId] = player
	g.players[playerId].addSocket(joiningSocket)
	g.playersLock.Unlock()

	log.Tracef("Player %s joined game %s with username '%s'.", player.Id, player.game.Id, player.Username)

	err := g.Send(player.Id, EventNewPlayer, EventNewPlayerData{
		Username: player.Username,
	})
	if err != nil {
		return err
	}

	err = joiningSocket.Send(player.Id, EventJoined, EventJoinedData{
		Secret: player.Secret,
	})
	if err != nil {
		return err
	}

	err = joiningSocket.sendGameInfo()
	if err != nil {
		return err
	}

	if g.OnPlayerJoined != nil {
		g.OnPlayerJoined(player)
	}

	return nil
}

func (g *Game) leave(player *Player) error {
	if g.running {
		g.Send(player.Id, EventLeft, EventLeftData{})

		if g.OnPlayerLeft != nil {
			g.OnPlayerLeft(player)
		}
	}

	g.playersLock.Lock()
	delete(g.players, player.Id)
	g.playersLock.Unlock()

	for _, socket := range player.sockets {
		player.disconnectSocket(socket.Id)
	}

	log.Tracef("Player %s (%s) left the game %s", player.Id, player.Username, player.game.Id)

	return nil
}

func (g *Game) playerUsernameMap() map[string]string {
	g.playersLock.RLock()
	usernameMap := make(map[string]string, len(g.players))
	for id, player := range g.players {
		usernameMap[id] = player.Username
	}
	g.playersLock.RUnlock()
	return usernameMap
}

func (g *Game) addSpectator(socket *Socket) error {
	g.spectatorsLock.Lock()
	defer g.spectatorsLock.Unlock()

	if g.server.config.MaxSpectatorsPerGame > 0 && len(g.spectators) >= g.server.config.MaxSpectatorsPerGame {
		return errors.New("max spectator count reached")
	}

	g.spectators[socket.Id] = socket
	return nil
}

func (g *Game) removeSpectator(id string) {
	g.spectatorsLock.Lock()
	delete(g.spectators, id)
	g.spectatorsLock.Unlock()
}
