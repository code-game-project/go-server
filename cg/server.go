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

type Game interface {
	OnPlayerJoined(player *Player)
	OnPlayerLeft(player *Player)
	OnPlayerSocketConnected(player *Player, socket *Socket)
	OnPlayerEvent(player *Player, event Event) error
}

type game struct {
	id     string
	game   Game
	public bool

	// protected by gamesLock in Server
	players map[string]*Player
}

type Server struct {
	socketsPlayersLock sync.RWMutex
	sockets            map[string]*Socket
	players            map[string]*Player

	gamesLock sync.RWMutex
	games     map[string]*game

	upgrader websocket.Upgrader
	config   ServerConfig

	newGame NewGame
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

type NewGame func(gameId string) Game

func NewServer(config ServerConfig) *Server {
	server := &Server{
		sockets: make(map[string]*Socket),
		players: make(map[string]*Player),
		games:   make(map[string]*game),

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

func (s *Server) Run(newGame NewGame) {
	r := mux.NewRouter()
	r.HandleFunc("/ws", s.connectSocket)
	r.HandleFunc("/events", s.events)
	http.Handle("/", r)

	s.newGame = newGame

	log.Infof("Listening on port %d...", s.config.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", s.config.Port), nil))
}

func (s *Server) Emit(gameId string, origin string, eventName EventName, eventData any) error {
	s.gamesLock.RLock()
	defer s.gamesLock.RUnlock()
	game, ok := s.games[gameId]
	if !ok {
		return errors.New("game not found!")
	}

	for _, p := range game.players {
		err := p.Send(origin, eventName, eventData)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) GetPlayer(gameId, playerId string) (*Player, bool) {
	s.gamesLock.RLock()
	defer s.gamesLock.RUnlock()

	game, ok := s.games[gameId]
	if !ok {
		return nil, false
	}

	player, ok := game.players[playerId]

	return player, ok
}

func (s *Server) connectSocket(w http.ResponseWriter, r *http.Request) {
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

	s.socketsPlayersLock.Lock()
	s.sockets[socket.Id] = socket
	s.socketsPlayersLock.Unlock()

	go socket.handleConnection()

	log.Tracef("Socket %s connected with id %s.", socket.conn.RemoteAddr(), socket.Id)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
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

	s.games[id] = &game{
		id:      id,
		game:    s.newGame(id),
		public:  public,
		players: make(map[string]*Player),
	}

	return id, nil
}

func (s *Server) joinGame(gameId, username string, joiningSocket *Socket) error {
	s.gamesLock.Lock()
	game, ok := s.games[gameId]
	if !ok {
		s.gamesLock.Unlock()
		return errors.New("game does not exist")
	}

	if s.config.MaxPlayersPerGame > 0 && len(game.players) >= s.config.MaxPlayersPerGame {
		return errors.New("max player count reached")
	}

	playerId := uuid.NewString()
	player := &Player{
		Id:       playerId,
		Username: username,
		Secret:   generatePlayerSecret(),
		server:   s,
		sockets:  make(map[string]*Socket),
		gameId:   gameId,
	}

	game.players[playerId] = player
	joiningSocket.player = player
	game.players[playerId].sockets[joiningSocket.Id] = joiningSocket

	s.gamesLock.Unlock()

	log.Tracef("Player %s joined game %s with username '%s'.", player.Id, player.gameId, player.Username)

	err := s.Emit(player.gameId, player.Id, EventJoinedGame, EventJoinedGameData{
		Username: player.Username,
	})
	if err != nil {
		return err
	}

	game.game.OnPlayerJoined(player)

	return nil
}

func (s *Server) leaveGame(player *Player) error {
	s.gamesLock.RLock()
	game := s.games[player.gameId]
	s.gamesLock.RUnlock()

	s.Emit(game.id, player.Id, EventLeftGame, EventLeftGameData{})
	game.game.OnPlayerLeft(player)

	s.gamesLock.Lock()
	delete(game.players, player.Id)
	s.gamesLock.Unlock()

	for _, socket := range player.sockets {
		player.disconnectSocket(socket.Id)
	}

	log.Tracef("Player %s (%s) left the game %s", player.Id, player.Username, player.gameId)

	return nil
}

func (s *Server) RemoveGame(gameId string) error {
	s.gamesLock.RLock()
	game, ok := s.games[gameId]
	s.gamesLock.RUnlock()
	if !ok {
		return errors.New("game does not exist")
	}

	s.gamesLock.RLock()
	for _, p := range game.players {
		err := s.leaveGame(p)
		if err != nil {
			s.gamesLock.RUnlock()
			return err
		}
	}
	s.gamesLock.RUnlock()

	s.gamesLock.Lock()
	delete(s.games, gameId)
	s.gamesLock.Unlock()

	log.Tracef("Removed game %s.", gameId)

	return nil
}

func (s *Server) connect(gameId, playerId, playerSecret string, socket *Socket) error {
	s.gamesLock.RLock()
	game, ok := s.games[gameId]
	s.gamesLock.RUnlock()
	if !ok {
		return errors.New("game does not exist")
	}

	s.gamesLock.RLock()
	player, ok := game.players[playerId]
	s.gamesLock.RUnlock()
	if !ok {
		return errors.New("player does not exist")
	}

	if subtle.ConstantTimeCompare([]byte(player.Secret), []byte(playerSecret)) == 0 {
		return errors.New("wrong player secret")
	}

	if s.config.MaxSocketsPerPlayer > 0 && len(player.sockets) >= s.config.MaxSocketsPerPlayer {
		return errors.New("max socket count reached")
	}

	s.gamesLock.Lock()
	player.sockets[socket.Id] = socket
	socket.player = player
	s.gamesLock.Unlock()

	game.game.OnPlayerSocketConnected(player, socket)

	return s.Emit(gameId, playerId, EventConnected, EventConnectedData{})
}

func (s *Server) removeSocket(id string) {
	s.socketsPlayersLock.Lock()
	delete(s.sockets, id)
	s.socketsPlayersLock.Unlock()
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
