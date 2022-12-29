package cg

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"github.com/Bananenpro/log"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) apiRoutes(r chi.Router) {
	r.Get("/info", s.infoEndpoint)
	r.Get("/events", s.eventsEndpoint)
	r.Get("/games", s.gamesEndpoint)
	r.Post("/games", s.createGameEndpoint)
	r.Get("/games/{gameId}", s.gameEndpoint)
	r.Get("/games/{gameId}/players", s.playersEndpoint)
	r.Post("/games/{gameId}/players", s.createPlayerEndpoint)
	r.Get("/games/{gameId}/players/{playerId}", s.playerEndpoint)
	r.Get("/games/{gameId}/players/{playerId}/connect", s.connectEndpoint)
	r.Get("/games/{gameId}/spectate", s.spectateEndpoint)

	r.Get("/debug", s.debugServer)
	r.Get("/games/{gameId}/debug", s.debugGame)
	r.Get("/games/{gameId}/players/{playerId}/debug", s.debugPlayer)
}

func (s *Server) infoEndpoint(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Name          string `json:"name"`
		CGVersion     string `json:"cg_version"`
		DisplayName   string `json:"display_name,omitempty"`
		Description   string `json:"description,omitempty"`
		Version       string `json:"version,omitempty"`
		RepositoryURL string `json:"repository_url,omitempty"`
	}
	sendJSON(w, http.StatusOK, response{
		Name:          s.config.Name,
		CGVersion:     CGVersion,
		DisplayName:   s.config.DisplayName,
		Description:   s.config.Description,
		Version:       s.config.Version,
		RepositoryURL: s.config.RepositoryURL,
	})
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
	w.Write(data)
}

func (s *Server) gamesEndpoint(w http.ResponseWriter, r *http.Request) {
	type game struct {
		Id        string `json:"id"`
		Players   int    `json:"players"`
		Protected bool   `json:"protected"`
	}

	protectedParam := r.URL.Query().Get("protected")
	protected, _ := strconv.ParseBool(protectedParam)

	s.gamesLock.RLock()
	publicGames := make([]game, 0, len(s.games)/2)
	private := 0
	for _, g := range s.games {
		if protectedParam == "" || protected == (g.joinSecret != "") {
			if g.public {
				publicGames = append(publicGames, game{
					Id:        g.Id,
					Players:   len(g.players),
					Protected: g.joinSecret != "",
				})
			} else {
				private++
			}
		}
	}
	s.gamesLock.RUnlock()

	type response struct {
		Private int    `json:"private"`
		Public  []game `json:"public"`
	}
	sendJSON(w, http.StatusOK, response{
		Private: private,
		Public:  publicGames,
	})
}

func (s *Server) createGameEndpoint(w http.ResponseWriter, r *http.Request) {
	body := r.Body
	if body == nil {
		send(w, http.StatusBadRequest, "empty request body")
		return
	}
	defer body.Close()

	type request struct {
		Public    bool            `json:"public"`
		Protected bool            `json:"protected"`
		Config    json.RawMessage `json:"config"`
	}
	var req request
	err := json.NewDecoder(body).Decode(&req)
	if err != nil {
		send(w, http.StatusBadRequest, "invalid request body")
		return
	}

	gameId, joinSecret, err := s.createGame(req.Public, req.Protected, req.Config)
	if err != nil {
		send(w, http.StatusForbidden, "max game count reached")
		return
	}

	type response struct {
		GameId     string `json:"game_id"`
		JoinSecret string `json:"join_secret,omitempty"`
	}
	sendJSON(w, http.StatusCreated, response{
		GameId:     gameId,
		JoinSecret: joinSecret,
	})
}

func (s *Server) gameEndpoint(w http.ResponseWriter, r *http.Request) {
	gameId := chi.URLParam(r, "gameId")

	game, ok := s.getGame(gameId)
	if !ok {
		send(w, http.StatusNotFound, "game not found")
		return
	}

	type response struct {
		Id        string `json:"id"`
		Players   int    `json:"players"`
		Protected bool   `json:"protected"`
		Config    any    `json:"config,omitempty"`
	}

	sendJSON(w, http.StatusOK, response{
		Id:        game.Id,
		Players:   len(game.players),
		Protected: game.joinSecret != "",
		Config:    game.config,
	})
}

func (s *Server) playersEndpoint(w http.ResponseWriter, r *http.Request) {
	gameId := chi.URLParam(r, "gameId")

	game, ok := s.getGame(gameId)
	if !ok {
		send(w, http.StatusNotFound, "game not found")
		return
	}

	players := game.playerUsernameMap()

	sendJSON(w, http.StatusOK, players)
}

func (s *Server) createPlayerEndpoint(w http.ResponseWriter, r *http.Request) {
	gameId := chi.URLParam(r, "gameId")

	body := r.Body
	if body == nil {
		send(w, http.StatusBadRequest, "empty request body")
		return
	}
	defer body.Close()
	type request struct {
		Username   string `json:"username"`
		JoinSecret string `json:"join_secret"`
	}
	var req request
	err := json.NewDecoder(body).Decode(&req)
	if err != nil || req.Username == "" {
		send(w, http.StatusBadRequest, "invalid request body")
		return
	}

	game, ok := s.getGame(gameId)
	if !ok {
		send(w, http.StatusNotFound, "game not found")
		return
	}

	playerId, playerSecret, err := game.join(req.Username, req.JoinSecret)
	if err != nil {
		send(w, http.StatusForbidden, err.Error())
		return
	}

	type response struct {
		PlayerId     string `json:"player_id"`
		PlayerSecret string `json:"player_secret"`
	}
	sendJSON(w, http.StatusCreated, response{
		PlayerId:     playerId,
		PlayerSecret: playerSecret,
	})
}

func (s *Server) playerEndpoint(w http.ResponseWriter, r *http.Request) {
	gameId := chi.URLParam(r, "gameId")
	playerId := chi.URLParam(r, "playerId")

	game, ok := s.getGame(gameId)
	if !ok {
		send(w, http.StatusNotFound, "game not found")
		return
	}

	player, ok := game.GetPlayer(playerId)
	if !ok {
		send(w, http.StatusNotFound, "player not found")
		return
	}

	type response struct {
		Username string `json:"username"`
	}
	sendJSON(w, http.StatusOK, response{
		Username: player.Username,
	})
}

func (s *Server) connectEndpoint(w http.ResponseWriter, r *http.Request) {
	gameId := chi.URLParam(r, "gameId")
	playerId := chi.URLParam(r, "playerId")
	playerSecret := r.URL.Query().Get("player_secret")
	if playerSecret == "" {
		send(w, http.StatusBadRequest, "missing `player_secret` query parameter")
		return
	}

	game, ok := s.getGame(gameId)
	if !ok {
		send(w, http.StatusNotFound, "game not found")
		return
	}

	player, ok := game.GetPlayer(playerId)
	if !ok {
		send(w, http.StatusNotFound, "player not found")
		return
	}

	if player.Secret != playerSecret {
		send(w, http.StatusForbidden, "wrong player secret")
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error("Failed to upgrade connection with %s: %s", r.RemoteAddr, err)
		return
	}

	socket := &GameSocket{
		Id:     uuid.NewString(),
		server: s,
		conn:   conn,
	}

	err = player.addSocket(socket)
	if err != nil {
		send(w, http.StatusForbidden, err.Error())
		return
	}

	s.log.Trace("Socket %s connected with id %s.", socket.conn.RemoteAddr(), socket.Id)

	go socket.handleConnection()

	if game.OnPlayerSocketConnected != nil {
		game.OnPlayerSocketConnected(player, socket)
	}
}

func (s *Server) spectateEndpoint(w http.ResponseWriter, r *http.Request) {
	gameId := chi.URLParam(r, "gameId")

	game, ok := s.getGame(gameId)
	if !ok {
		send(w, http.StatusNotFound, "game not found")
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error("Failed to upgrade connection with %s: %s", r.RemoteAddr, err)
		return
	}

	socket := &GameSocket{
		Id:     uuid.NewString(),
		server: s,
		conn:   conn,
	}

	err = game.addSpectator(socket)
	if err != nil {
		send(w, http.StatusForbidden, err.Error())
	}

	s.log.Trace("Socket %s connected with id %s.", socket.conn.RemoteAddr(), socket.Id)

	go socket.handleConnection()
}

func (s *Server) debugServer(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error("Failed to upgrade connection with %s: %s", r.RemoteAddr, err)
		return
	}

	socket := &debugSocket{
		id:         uuid.NewString(),
		server:     s,
		logger:     s.log,
		conn:       conn,
		severities: getDebugSeverities(r),
	}

	socket.logger.addDebugSocket(socket)

	go socket.handleConnection()
}

func (s *Server) debugGame(w http.ResponseWriter, r *http.Request) {
	gameId := chi.URLParam(r, "gameId")
	game, ok := s.getGame(gameId)
	if !ok {
		send(w, http.StatusNotFound, "game not found")
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error("Failed to upgrade connection with %s: %s", r.RemoteAddr, err)
		return
	}

	socket := &debugSocket{
		id:         uuid.NewString(),
		server:     s,
		logger:     game.Log,
		conn:       conn,
		severities: getDebugSeverities(r),
	}

	socket.logger.addDebugSocket(socket)

	go socket.handleConnection()
}

func (s *Server) debugPlayer(w http.ResponseWriter, r *http.Request) {
	gameId := chi.URLParam(r, "gameId")
	playerId := chi.URLParam(r, "playerId")
	playerSecret := r.URL.Query().Get("player_secret")
	if playerSecret == "" {
		send(w, http.StatusBadRequest, "missing `player_secret` query parameter")
		return
	}

	game, ok := s.getGame(gameId)
	if !ok {
		send(w, http.StatusNotFound, "game not found")
		return
	}

	player, ok := game.GetPlayer(playerId)
	if !ok {
		send(w, http.StatusNotFound, "player not found")
		return
	}

	if player.Secret != playerSecret {
		send(w, http.StatusForbidden, "wrong player secret")
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error("Failed to upgrade connection with %s: %s", r.RemoteAddr, err)
		return
	}

	socket := &debugSocket{
		id:         uuid.NewString(),
		server:     s,
		logger:     player.Log,
		conn:       conn,
		severities: getDebugSeverities(r),
	}

	socket.logger.addDebugSocket(socket)

	go socket.handleConnection()
}

func getDebugSeverities(r *http.Request) map[DebugSeverity]bool {
	var err error
	severities := make(map[DebugSeverity]bool)

	severities[DebugTrace], err = strconv.ParseBool(r.URL.Query().Get("trace"))
	if err != nil {
		severities[DebugTrace] = true
	}

	severities[DebugInfo], err = strconv.ParseBool(r.URL.Query().Get("info"))
	if err != nil {
		severities[DebugInfo] = true
	}

	severities[DebugWarning], err = strconv.ParseBool(r.URL.Query().Get("warning"))
	if err != nil {
		severities[DebugWarning] = true
	}

	severities[DebugError], err = strconv.ParseBool(r.URL.Query().Get("error"))
	if err != nil {
		severities[DebugError] = true
	}

	return severities
}

func sendJSON(w http.ResponseWriter, status int, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	w.Write(jsonData)
}

func send(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	w.Write([]byte(msg))
}
