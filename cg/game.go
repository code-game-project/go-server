package cg

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Game struct {
	Id string

	OnPlayerJoined          func(player *Player)
	OnPlayerLeft            func(player *Player)
	OnPlayerSocketConnected func(player *Player, socket *GameSocket)
	OnSpectatorConnected    func(socket *GameSocket)

	Log *Logger

	config any

	cmdChan chan CommandWrapper

	public     bool
	joinSecret string

	playersLock sync.RWMutex
	players     map[string]*Player

	spectatorsLock sync.RWMutex
	spectators     map[string]*GameSocket

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
		Log:        NewLogger(false),
		cmdChan:    make(chan CommandWrapper, 10),
		public:     public,
		players:    make(map[string]*Player),
		spectators: make(map[string]*GameSocket),
		server:     server,
		running:    true,
	}
}

// Set game config data. This should be a struct of type GameConfig.
// It is required to call this function in order for some API endpoints to work.
func (g *Game) SetConfig(config any) {
	g.config = config
}

// Send sends the event to all players currently in the game.
func (g *Game) Send(event EventName, data any) error {
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

	g.Log.TraceData(e, "Broadcasting '%s' event to all players...", e.Name)

	g.playersLock.RLock()
	defer g.playersLock.RUnlock()
	for _, p := range g.players {
		err := p.sendEncoded(jsonData)
		if err != nil {
			return err
		}
	}

	g.spectatorsLock.RLock()
	defer g.spectatorsLock.RUnlock()
	for _, s := range g.spectators {
		err := s.send(jsonData)
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
			g.Log.Error("Couldn't disconnect player '%s': %s", p.Id, err)
		}
	}

	close(g.cmdChan)

	g.server.log.Info("Removed game %s.", g.Id)

	g.Log.Close()

	return nil
}

func (g *Game) join(username, joinSecret string) (string, string, error) {
	if g.joinSecret != "" && g.joinSecret != joinSecret {
		return "", "", errors.New("wrong join secret")
	}

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
		Secret:       generateSecret(),
		Log:          NewLogger(false),
		server:       g.server,
		sockets:      make(map[string]*GameSocket),
		game:         g,
		missedEvents: make([][]byte, 0),
	}

	g.playersLock.Lock()
	g.players[playerId] = player
	g.playersLock.Unlock()

	g.Log.Info("Player '%s' (%s) joined the game.", player.Username, player.Id)

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

	g.Log.Info("Player '%s' (%s) left the game %s", player.Id, player.Username, player.game.Id)

	if playerCount == 0 {
		g.markedAsEmpty = time.Now()
	}

	return nil
}

func (g *Game) addSpectator(socket *GameSocket) error {
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
			if p.socketCount == 0 && time.Since(p.lastConnection) >= g.server.config.KickInactivePlayerDelay {
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
