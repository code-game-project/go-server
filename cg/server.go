package cg

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
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

	log *Logger

	killTicker *time.Ticker

	runGameFunc func(game *Game, config json.RawMessage)
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
		log:    NewLogger(true),
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
			log.Errorf("Web root '%s' does not exist.", server.config.WebRoot)
			server.config.WebRoot = ""
		} else if !stat.IsDir() {
			log.Errorf("Web root '%s' is not a directory.", server.config.WebRoot)
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

	if server.config.Version == "" {
		log.Warn("No game version specified.")
	} else {
		server.config.Version = strings.TrimPrefix(server.config.Version, "v")
		if _, _, _, err := parseVersion(server.config.Version); err != nil {
			log.Error("Invalid game version:", err)
			server.config.Version = ""
		}
	}

	return server
}

func parseVersion(version string) (int, int, int, error) {
	parts := strings.Split(version, ".")

	var major, minor, patch int
	var err error

	if len(parts) >= 1 {
		major, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("major version not a number: %s", version)
		}
	}

	if len(parts) >= 2 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("minor version not a number: %s", version)
		}
	}

	if len(parts) >= 3 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("patch version not a number: %s", version)
		}
	}

	return major, minor, patch, nil
}

// Run starts the webserver and listens for new connections.
func (s *Server) Run(runGameFunc func(game *Game, config json.RawMessage)) {
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

func (s *Server) createGame(public, protected bool, config json.RawMessage) (string, string, error) {
	s.gamesLock.Lock()
	defer s.gamesLock.Unlock()

	if s.config.MaxGames > 0 && len(s.games) >= s.config.MaxGames {
		return "", "", errors.New("max game count reached")
	}

	id := uuid.NewString()

	game := newGame(s, id, public)

	if protected {
		game.joinSecret = generateSecret()
	}

	s.games[id] = game

	go func() {
		s.runGameFunc(game, config)
		game.Close()
	}()

	if public {
		s.log.Info("Created public game %s.", id)
	} else {
		s.log.Info("Created private game %s-****-****-****-************.", id[:8])
	}

	return id, game.joinSecret, nil
}

func (s *Server) removeGame(game *Game) {
	s.gamesLock.Lock()
	delete(s.games, game.ID)
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
				if g.markedAsEmpty.Equal(time.Time{}) {
					g.markedAsEmpty = time.Now()
				} else if time.Now().After(g.markedAsEmpty.Add(s.config.DeleteInactiveGameDelay)) {
					g.Close()
				}
			}
		}
	}
}

func (s *Server) getGame(gameID string) (*Game, bool) {
	s.gamesLock.RLock()
	game, ok := s.games[gameID]
	s.gamesLock.RUnlock()
	return game, ok
}

func generateSecret() string {
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
