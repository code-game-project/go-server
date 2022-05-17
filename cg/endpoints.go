package cg

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/Bananenpro/log"
	"github.com/google/uuid"
)

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

func (s *Server) infoEndpoint(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Name          string `json:"name"`
		CGVersion     string `json:"cg_version"`
		DisplayName   string `json:"display_name,omitempty"`
		Description   string `json:"description,omitempty"`
		Version       string `json:"version,omitempty"`
		RepositoryURL string `json:"repository_url,omitempty"`
	}
	data, err := json.Marshal(response{
		Name:          s.config.Name,
		CGVersion:     CGVersion,
		DisplayName:   s.config.DisplayName,
		Description:   s.config.Description,
		Version:       s.config.Version,
		RepositoryURL: s.config.RepositoryURL,
	})
	if err != nil {
		log.Errorf("Failed to decode data for the /info endpoint.")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.Write(data)
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

func (s *Server) gamesEndpoint(w http.ResponseWriter, r *http.Request) {
	type game struct {
		Id      string `json:"id"`
		Players int    `json:"players"`
	}
	type response struct {
		Private int    `json:"private"`
		Public  []game `json:"public"`
	}

	s.gamesLock.RLock()
	publicGames := make([]game, 0, len(s.games)/2)
	private := 0
	for _, g := range s.games {
		if g.public {
			publicGames = append(publicGames, game{
				Id:      g.Id,
				Players: len(g.players),
			})
		} else {
			private++
		}
	}
	s.gamesLock.RUnlock()

	data, err := json.Marshal(response{
		Private: private,
		Public:  publicGames,
	})
	if err != nil {
		log.Errorf("GET /games: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.Write(data)
}

func (s *Server) createGameEndpoint(w http.ResponseWriter, r *http.Request) {
	body := r.Body
	if body == nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Empty request body."))
		return
	}
	defer body.Close()

	type request struct {
		Public *bool `json:"public"`
	}
	var req request
	err := json.NewDecoder(body).Decode(&req)
	if err != nil || req.Public == nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request body."))
		return
	}

	gameId, err := s.createGame(*req.Public)
	if err != nil {
		type errorResponse struct {
			Message string `json:"message"`
		}
		data, err := json.Marshal(errorResponse{
			Message: err.Error(),
		})
		if err != nil {
			log.Errorf("POST /games: unexpected error: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Header().Set("content-type", "application/json")
		w.Write(data)
	}

	type response struct {
		GameId string `json:"game_id"`
	}

	data, err := json.Marshal(response{
		GameId: gameId,
	})
	if err != nil {
		log.Errorf("POST /games: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.Write(data)
}
