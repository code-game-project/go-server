package cg

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"sync"

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

	newGameFunc NewGameFunc
}

type ServerConfig struct {
	// The port to listen on for new websocket connections.
	Port int
	// The path to the CGE file for the current game (0 -> unlimited).
	CGEFilepath string
	// The maximum number of allowed sockets per player (0 -> unlimited).
	MaxSocketsPerPlayer int
	// The maximum number of allowed players per game (0 -> unlimited).
	MaxPlayersPerGame int
	// The maximum number of games.
	MaxGames int
}

type NewGameFunc func(game *Game) GameInterface

func NewServer(config ServerConfig) *Server {
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
func (s *Server) Run(newGame NewGameFunc) {
	r := mux.NewRouter()
	r.HandleFunc("/ws", s.wsEndpoint)
	r.HandleFunc("/events", s.eventsEndpoint)
	http.Handle("/", r)

	s.newGameFunc = newGame

	log.Infof("Listening on port %d...", s.config.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", s.config.Port), nil))
}

func (s *Server) wsEndpoint(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade connection with %s: %s", r.RemoteAddr, err)
		return
	}

	socket := &Socket{
		Id:     uuid.NewString(),
		server: s,
		conn:   conn,
	}

	s.addSocket(socket)

	go socket.handleConnection()

	log.Tracef("Socket %s connected with id %s.", socket.conn.RemoteAddr(), socket.Id)
}

func (s *Server) eventsEndpoint(w http.ResponseWriter, r *http.Request) {
	if s.config.CGEFilepath == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	data, err := os.ReadFile(s.config.CGEFilepath)
	if err != nil {
		log.Errorf("Couldn't read '%s': %s", s.config.CGEFilepath, err)
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	_, err = w.Write(data)
	if err != nil {
		log.Errorf("Failed to send CGE file content: %s", err)
	}
}

func (s *Server) createGame(public bool) (string, error) {
	s.gamesLock.Lock()
	defer s.gamesLock.Unlock()

	if s.config.MaxGames > 0 && len(s.games) >= s.config.MaxGames {
		return "", errors.New("max game count reached")
	}

	id := uuid.NewString()

	game := &Game{
		Id:      id,
		public:  public,
		players: make(map[string]*Player),
		server:  s,
	}

	game.gameInterface = s.newGameFunc(game)

	s.games[id] = game

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
	game, ok := s.getGame(playerId)
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

	game.gameInterface.OnPlayerSocketConnected(player, socket)

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
