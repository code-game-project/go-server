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
	OnSpectatorConnected    func(socket *Socket)

	cmdChan chan CommandWrapper

	public bool

	playersLock sync.RWMutex
	players     map[string]*Player

	spectatorsLock sync.RWMutex
	spectators     map[string]*Socket

	server *Server

	running bool

	markedAsEmpty time.Time
}

type EventWrapper struct {
	Player *Player
	Event  Event
}

func newGame(server *Server, id string, public bool) *Game {
	return &Game{
		Id:         id,
		cmdChan:    make(chan CommandWrapper, 10),
		public:     public,
		players:    make(map[string]*Player),
		spectators: make(map[string]*Socket),
		server:     server,
		running:    true,
	}
}

// Send sends the event to all players currently in the game.
func (g *Game) Send(event EventName, data any) error {
	g.playersLock.RLock()
	defer g.playersLock.RUnlock()
	for _, p := range g.players {
		err := p.Send(event, data)
		if err != nil {
			return err
		}
	}

	g.spectatorsLock.RLock()
	defer g.spectatorsLock.RUnlock()
	for _, s := range g.spectators {
		err := s.Send(event, data)
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

// NextCommand returns the next command in the queue or ok = false if there is none.
func (g *Game) NextCommand() (CommandWrapper, bool) {
	select {
	case wrapper, ok := <-g.cmdChan:
		if ok {
			return wrapper, true
		} else {
			return CommandWrapper{}, false
		}
	default:
		return CommandWrapper{}, false
	}
}

// WaitForNextCommand waits for and then returns the next command in the queue or ok = false if the game has been closed.
func (g *Game) WaitForNextCommand() (CommandWrapper, bool) {
	wrapper, ok := <-g.cmdChan
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

	close(g.cmdChan)

	log.Tracef("Removed game %s.", g.Id)

	return nil
}

func (g *Game) join(username string) (string, string, error) {
	if g.server.config.MaxPlayersPerGame > 0 {
		g.playersLock.RLock()
		playerCount := len(g.players)
		g.playersLock.RUnlock()
		if playerCount >= g.server.config.MaxPlayersPerGame {
			return "", "", errors.New("max player count reached")
		}
	}

	g.markedAsEmpty = time.Time{}

	playerId := uuid.NewString()
	player := &Player{
		Id:           playerId,
		Username:     username,
		Secret:       generatePlayerSecret(),
		server:       g.server,
		sockets:      make(map[string]*Socket),
		game:         g,
		missedEvents: make([][]byte, 0),
	}

	g.playersLock.Lock()
	g.players[playerId] = player
	g.playersLock.Unlock()

	log.Tracef("Player %s joined game %s with username '%s'.", player.Id, player.game.Id, player.Username)

	if g.OnPlayerJoined != nil {
		g.OnPlayerJoined(player)
	}

	return player.Id, player.Secret, nil
}

func (g *Game) leave(player *Player) error {
	if g.running {
		if g.OnPlayerLeft != nil {
			g.OnPlayerLeft(player)
		}
	}

	g.playersLock.Lock()
	delete(g.players, player.Id)
	playerCount := len(g.players)
	g.playersLock.Unlock()

	for _, socket := range player.sockets {
		player.disconnectSocket(socket.Id)
	}

	log.Tracef("Player %s (%s) left the game %s", player.Id, player.Username, player.game.Id)

	if playerCount == 0 {
		g.markedAsEmpty = time.Now()
	}

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
	if g.server.config.MaxSpectatorsPerGame > 0 && len(g.spectators) >= g.server.config.MaxSpectatorsPerGame {
		g.spectatorsLock.Unlock()
		return errors.New("max spectator count reached")
	}

	socket.spectateGame = g
	g.spectators[socket.Id] = socket
	g.spectatorsLock.Unlock()

	if g.OnSpectatorConnected != nil {
		g.OnSpectatorConnected(socket)
	}

	return nil
}

func (g *Game) removeSpectator(id string) {
	g.spectatorsLock.Lock()
	delete(g.spectators, id)
	g.spectatorsLock.Unlock()
}

func (g *Game) kickInactivePlayers() {
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
}
