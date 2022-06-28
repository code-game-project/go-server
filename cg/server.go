package cg

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Bananenpro/log"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/cors"
)

type Server struct {
	gamesLock sync.RWMutex
	games     map[string]*Game

	upgrader websocket.Upgrader
	config   ServerConfig

	killTicker *time.Ticker

	runGameFunc func(game *Game)
}

type ServerConfig struct {
	// The port to listen on for new websocket connections. (default: 80)
	Port int
	// The path to the CGE file for the current game.
	CGEFilepath string
	// All files in this direcory will be served.
	WebRoot string
	// The maximum number of allowed sockets per player (0 => unlimited).
	MaxSocketsPerPlayer int
	// The maximum number of allowed players per game (0 => unlimited).
	MaxPlayersPerGame int
	// The maximum number of allowed spectators per game (0 => unlimited).
	MaxSpectatorsPerGame int
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
	// The time after which an inactive websocket connection will be closed. (default: 15 minutes)
	WebsocketTimeout time.Duration
}

type EventSender interface {
	Send(event EventName, data any) error
}

func NewServer(name string, config ServerConfig) *Server {
	config.Name = name

	server := &Server{
		games: make(map[string]*Game),

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

	if server.config.WebRoot != "" {
		stat, err := os.Stat(server.config.WebRoot)
		if err != nil {
			log.Warnf("Web root '%s' does not exist.", server.config.WebRoot)
			server.config.WebRoot = ""
		} else if !stat.IsDir() {
			log.Warnf("Web root '%s' is not a directory.", server.config.WebRoot)
			server.config.WebRoot = ""
		}
	}

	if server.config.WebsocketTimeout == 0 {
		server.config.WebsocketTimeout = 15 * time.Minute
	}

	if server.config.KickInactivePlayerDelay > 0 || server.config.DeleteInactiveGameDelay > 0 {
		duration := server.config.KickInactivePlayerDelay
		if server.config.DeleteInactiveGameDelay > 0 && (duration == 0 || duration > server.config.DeleteInactiveGameDelay) {
			duration = server.config.DeleteInactiveGameDelay
		}
		server.killTicker = time.NewTicker(duration)
		go func() {
			for range server.killTicker.C {
				server.removeInactiveGamesPlayers()
			}
		}()
	}

	return server
}

// Run starts the webserver and listens for new connections.
func (s *Server) Run(runGameFunc func(game *Game)) {
	s.runGameFunc = runGameFunc

	router := chi.NewMux()
	router.Use(middleware.Recoverer)
	router.Route("/api", s.apiRoutes)
	router.Route("/", s.frontendRoutes)

	handler := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedHeaders: []string{"*"},
		AllowedMethods: []string{"GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH"},
	}).Handler(router)

	log.Infof("Listening on port %d...", s.config.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", s.config.Port), handler))
}

func (s *Server) createGame(public bool) (string, error) {
	s.gamesLock.Lock()
	defer s.gamesLock.Unlock()

	if s.config.MaxGames > 0 && len(s.games) >= s.config.MaxGames {
		return "", errors.New("max game count reached")
	}

	id := uuid.NewString()

	game := newGame(s, id, public)

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

func (s *Server) removeInactiveGamesPlayers() {
	for _, g := range s.games {
		g.kickInactivePlayers()

		if s.config.DeleteInactiveGameDelay > 0 {
			g.playersLock.RLock()
			playerCount := len(g.players)
			g.playersLock.RUnlock()

			if playerCount == 0 {
				if g.markedAsEmpty == (time.Time{}) {
					g.markedAsEmpty = time.Now()
				} else if time.Now().After(g.markedAsEmpty.Add(s.config.DeleteInactiveGameDelay)) {
					g.Close()
				}
			}
		}
	}
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
