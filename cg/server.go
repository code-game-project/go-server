package cg

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/Bananenpro/log"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type Server struct {
	socketsPlayersLock sync.RWMutex
	sockets            map[string]*Socket
	players            map[string]*Player

	gamesLock sync.RWMutex
	games     map[string]*Game

	upgrader websocket.Upgrader
	config   ServerConfig

	runGameFunc func(game *Game)
}

type ServerConfig struct {
	// The port to listen on for new websocket connections. (default: 80)
	Port int
	// The path to the CGE file for the current game.
	CGEFilepath string
	// The maximum number of allowed sockets per player (0 => unlimited).
	MaxSocketsPerPlayer int
	// The maximum number of allowed players per game (0 => unlimited).
	MaxPlayersPerGame int
	// The maximum number of games (0 => unlimited).
	MaxGames int
	// The time after which game with no connected sockets will be deleted. (0 => never)
	DeleteInactiveGameDelay time.Duration
	// The time after which a player without sockets will be kicked. (0 => never)
	KickInactivePlayerDelay time.Duration
	// The name of the game in snake_case.
	Name string
	// The name of the game that will be displayed to the user.
	DisplayName string
	// The version of the game.
	Version string
	// The description of the game.
	Description string
	// The URL to the code repository of the game.
	RepositoryURL string
}

type EventSender interface {
	Send(origin string, event EventName, eventData any) error
}

func NewServer(name string, config ServerConfig) *Server {
	config.Name = name

	server := &Server{
		sockets: make(map[string]*Socket),
		players: make(map[string]*Player),
		games:   make(map[string]*Game),

		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},

		config: config,
	}

	if server.config.Port == 0 {
		server.config.Port = 80
	}

	if server.config.CGEFilepath == "" {
		log.Warn("No CGE file location specified!")
	}

	return server
}

// Run starts the webserver and listens for new connections.
func (s *Server) Run(runGameFunc func(game *Game)) {
	r := mux.NewRouter()
	r.HandleFunc("/ws", s.wsEndpoint).Methods("GET")
	r.HandleFunc("/info", s.infoEndpoint).Methods("GET")
	r.HandleFunc("/events", s.eventsEndpoint).Methods("GET")
	r.HandleFunc("/games", s.gamesEndpoint).Methods("GET")
	r.HandleFunc("/games", s.createGameEndpoint).Methods("POST")
	http.Handle("/", r)

	s.runGameFunc = runGameFunc

	log.Infof("Listening on port %d...", s.config.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", s.config.Port), nil))
}

func (s *Server) createGame(public bool) (string, error) {
	s.gamesLock.Lock()
	defer s.gamesLock.Unlock()

	if s.config.DeleteInactiveGameDelay > 0 {
		for _, g := range s.games {
			g.socketCountLock.RLock()
			if g.socketCount == 0 && time.Now().Sub(g.lastConnection) >= s.config.DeleteInactiveGameDelay {
				s.gamesLock.Unlock()
				g.socketCountLock.RUnlock()
				g.Close()
				s.gamesLock.Lock()
			} else {
				g.socketCountLock.RUnlock()
			}
		}
	}

	if s.config.MaxGames > 0 && len(s.games) >= s.config.MaxGames {
		return "", errors.New("max game count reached")
	}

	id := uuid.NewString()

	game := s.newGame(id, public)

	s.games[id] = game

	go func() {
		s.runGameFunc(game)
		game.Close()
	}()

	log.Tracef("Created game %s.", id)

	return id, nil
}

func (s *Server) removeGame(game *Game) {
	s.gamesLock.Lock()
	delete(s.games, game.Id)
	s.gamesLock.Unlock()
}

func (s *Server) joinGame(gameId, username string, joiningSocket *Socket) error {
	game, ok := s.getGame(gameId)
	if !ok {
		return errors.New("game does not exist")
	}

	if s.config.MaxPlayersPerGame > 0 && len(game.players) >= s.config.MaxPlayersPerGame {
		return errors.New("max player count reached")
	}

	return game.join(username, joiningSocket)
}

func (s *Server) connect(gameId, playerId, playerSecret string, socket *Socket) error {
	game, ok := s.getGame(gameId)
	if !ok {
		return errors.New("game does not exist")
	}

	player, ok := game.GetPlayer(playerId)
	if !ok {
		return errors.New("player does not exist")
	}

	if subtle.ConstantTimeCompare([]byte(player.Secret), []byte(playerSecret)) == 0 {
		return errors.New("wrong player secret")
	}

	if s.config.MaxSocketsPerPlayer > 0 && len(player.sockets) >= s.config.MaxSocketsPerPlayer {
		return errors.New("max socket count reached")
	}

	socket.player = player
	player.addSocket(socket)

	if game.OnPlayerSocketConnected != nil {
		game.OnPlayerSocketConnected(player, socket)
	}

	return socket.Send(playerId, EventConnected, EventConnectedData{})
}

func (s *Server) addSocket(socket *Socket) {
	s.socketsPlayersLock.Lock()
	s.sockets[socket.Id] = socket
	s.socketsPlayersLock.Unlock()
}

func (s *Server) removeSocket(id string) {
	s.socketsPlayersLock.Lock()
	delete(s.sockets, id)
	s.socketsPlayersLock.Unlock()
}

func (s *Server) getGame(gameId string) (*Game, bool) {
	s.gamesLock.RLock()
	game, ok := s.games[gameId]
	s.gamesLock.RUnlock()
	return game, ok
}

func generatePlayerSecret() string {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	ret := make([]byte, 64)
	for i := 0; i < 64; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			panic(err)
		}
		ret[i] = letters[num.Int64()]
	}

	return string(ret)
}
