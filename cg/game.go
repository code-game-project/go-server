package cg

import (
	"sync"

	"github.com/Bananenpro/log"
	"github.com/google/uuid"
)

type Game struct {
	Id string

	OnPlayerJoined          func(player *Player)
	OnPlayerLeft            func(player *Player)
	OnPlayerSocketConnected func(player *Player, socket *Socket)

	Events chan EventWrapper

	public bool

	playersLock sync.RWMutex
	players     map[string]*Player

	server *Server

	running bool
}

type EventWrapper struct {
	Player *Player
	Event  Event
}

func (s *Server) newGame(id string, public bool) *Game {
	return &Game{
		Id:      id,
		Events:  make(chan EventWrapper, 100),
		public:  public,
		players: make(map[string]*Player),
		server:  s,
		running: true,
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

	return nil
}

// GetPlayer returns a player in the game by id.
func (g *Game) GetPlayer(playerId string) (*Player, bool) {
	g.playersLock.RLock()
	defer g.playersLock.RUnlock()
	player, ok := g.players[playerId]
	return player, ok
}

// Returns true if the game has not already been closed.
func (g *Game) Running() bool {
	return g.running
}

func (g *Game) close() error {
	if !g.running {
		return nil
	}

	g.running = false

	g.server.removeGame(g)

	for _, p := range g.players {
		err := g.leave(p)
		if err != nil {
			return err
		}
	}

	close(g.Events)

	log.Tracef("Removed game %s.", g.Id)

	return nil
}

func (g *Game) join(username string, joiningSocket *Socket) error {
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
	g.players[playerId].sockets[joiningSocket.Id] = joiningSocket
	g.playersLock.Unlock()

	log.Tracef("Player %s joined game %s with username '%s'.", player.Id, player.game.Id, player.Username)

	if g.OnPlayerJoined != nil {
		g.OnPlayerJoined(player)
	}

	return g.Send(player.Id, EventNewPlayer, EventNewPlayerData{
		Username: player.Username,
	})
}

func (g *Game) leave(player *Player) error {
	if g.running {
		if g.OnPlayerJoined != nil {
			g.OnPlayerLeft(player)
		}

		g.Send(player.Id, EventLeft, EventLeftData{})
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
