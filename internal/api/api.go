// Package api exposes the orchestrator over HTTP. The Discord bot (and any
// other client) drives server lifecycle through this small JSON API.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/brandonli/cs2-server/internal/model"
	"github.com/brandonli/cs2-server/internal/orchestrator"
)

// Server is the HTTP front-end for a ServerManager.
type Server struct {
	mgr    orchestrator.ServerManager
	log    *slog.Logger
	mux    *http.ServeMux
	maxPer int // per-owner instance cap (0 = unlimited)
}

// New builds an API server. maxPerOwner caps instances per owner (0 disables).
func New(mgr orchestrator.ServerManager, log *slog.Logger, maxPerOwner int) *Server {
	s := &Server{mgr: mgr, log: log, mux: http.NewServeMux(), maxPer: maxPerOwner}
	s.routes()
	return s
}

// Handler returns the configured HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /v1/servers", s.handleCreate)
	s.mux.HandleFunc("GET /v1/servers", s.handleList)
	s.mux.HandleFunc("GET /v1/servers/{id}", s.handleGet)
	s.mux.HandleFunc("GET /v1/servers/{id}/status", s.handleStatus)
	s.mux.HandleFunc("POST /v1/servers/{id}/restart", s.handleRestart)
	s.mux.HandleFunc("DELETE /v1/servers/{id}", s.handleStop)
}

// --- request/response payloads -------------------------------------------

type createRequest struct {
	OwnerID    string `json:"owner_id"`
	Name       string `json:"name"`
	Map        string `json:"map"`
	GameType   int    `json:"game_type"`
	GameMode   int    `json:"game_mode"`
	MaxPlayers int    `json:"max_players"`
	Public     bool   `json:"public"`
	GSLT       string `json:"gslt"`
	Password   string `json:"password"`
	BotQuota   int    `json:"bot_quota"`
}

type instanceView struct {
	*model.Instance
	Connect string `json:"connect"`
}

func view(in *model.Instance) instanceView {
	return instanceView{Instance: in, Connect: in.ConnectString()}
}

// --- handlers ------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Public && req.GSLT == "" {
		// The manager may still substitute a default GSLT; we only warn here by
		// allowing it through. Public without any GSLT will fail to be
		// internet-visible but should not error the request.
		s.log.Warn("public server requested without explicit GSLT", "owner", req.OwnerID)
	}

	if s.maxPer > 0 && req.OwnerID != "" {
		existing, err := s.mgr.List(r.Context(), req.OwnerID)
		if err != nil {
			s.writeManagerError(w, err)
			return
		}
		if len(existing) >= s.maxPer {
			writeError(w, http.StatusConflict, "per-user server limit reached")
			return
		}
	}

	inst, err := s.mgr.Create(r.Context(), orchestrator.CreateOptions{
		OwnerID:    req.OwnerID,
		Name:       req.Name,
		Map:        req.Map,
		GameType:   req.GameType,
		GameMode:   req.GameMode,
		MaxPlayers: req.MaxPlayers,
		Public:     req.Public,
		GSLT:       req.GSLT,
		Password:   req.Password,
		BotQuota:   req.BotQuota,
	})
	if err != nil {
		s.writeManagerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view(inst))
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	owner := r.URL.Query().Get("owner_id")
	list, err := s.mgr.List(r.Context(), owner)
	if err != nil {
		s.writeManagerError(w, err)
		return
	}
	out := make([]instanceView, 0, len(list))
	for _, in := range list {
		out = append(out, view(in))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	in, err := s.mgr.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeManagerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view(in))
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.mgr.Status(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeManagerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if err := s.mgr.Restart(r.Context(), r.PathValue("id")); err != nil {
		s.writeManagerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarting"})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if err := s.mgr.Stop(r.Context(), r.PathValue("id")); err != nil {
		s.writeManagerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// --- helpers -------------------------------------------------------------

func (s *Server) writeManagerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, model.ErrNotFound):
		writeError(w, http.StatusNotFound, "server not found")
	case errors.Is(err, model.ErrNoPorts):
		writeError(w, http.StatusServiceUnavailable, "no free ports available")
	case errors.Is(err, model.ErrLimitExceeded):
		writeError(w, http.StatusConflict, "server limit exceeded")
	default:
		s.log.Error("manager error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
