// Package api exposes the orchestrator over HTTP. The Discord bot (and any
// other client) drives server lifecycle through this small JSON API.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/brandonli/cs2-server/internal/gamemode"
	"github.com/brandonli/cs2-server/internal/model"
	"github.com/brandonli/cs2-server/internal/orchestrator"
)

// Server is the HTTP front-end for a ServerManager.
type Server struct {
	mgr    orchestrator.ServerManager
	log    *slog.Logger
	mux    *http.ServeMux
	maxPer int    // per-owner instance cap (0 = unlimited)
	token  string // shared bearer token for /v1/* (empty disables auth)
}

// New builds an API server. maxPerOwner caps instances per owner (0 disables).
// token is the shared bearer required on /v1/* routes; when empty, auth is
// disabled (the orchestrator logs a startup security warning).
func New(mgr orchestrator.ServerManager, log *slog.Logger, maxPerOwner int, token string) *Server {
	s := &Server{mgr: mgr, log: log, mux: http.NewServeMux(), maxPer: maxPerOwner, token: token}
	s.routes()
	return s
}

// Handler returns the configured HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	// /healthz is intentionally unauthenticated for liveness probes.
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	// All lifecycle routes live under /v1/* and require the bearer token (when
	// configured) via the auth wrapper.
	s.mux.HandleFunc("POST /v1/servers", s.auth(s.handleCreate))
	s.mux.HandleFunc("GET /v1/servers", s.auth(s.handleList))
	s.mux.HandleFunc("DELETE /v1/servers", s.auth(s.handleStopAll))
	s.mux.HandleFunc("GET /v1/servers/{id}", s.auth(s.handleGet))
	s.mux.HandleFunc("GET /v1/servers/{id}/status", s.auth(s.handleStatus))
	s.mux.HandleFunc("POST /v1/servers/{id}/restart", s.auth(s.handleRestart))
	s.mux.HandleFunc("DELETE /v1/servers/{id}", s.auth(s.handleStop))
}

// auth wraps a handler so that, when a token is configured, the request must
// present "Authorization: Bearer <token>" (compared in constant time). When no
// token is configured, auth is disabled and the handler is called directly.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" {
			const prefix = "Bearer "
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, prefix) ||
				subtle.ConstantTimeCompare([]byte(h[len(prefix):]), []byte(s.token)) != 1 {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		next(w, r)
	}
}

// --- request/response payloads -------------------------------------------

type createRequest struct {
	OwnerID     string `json:"owner_id"`
	Name        string `json:"name"`
	Map         string `json:"map"`
	WorkshopMap string `json:"workshop_map"`
	Mode        string `json:"mode"`
	GameType    int    `json:"game_type"`
	GameMode    int    `json:"game_mode"`
	MaxPlayers  int    `json:"max_players"`
	Public      bool   `json:"public"`
	GSLT        string `json:"gslt"`
	Password    string `json:"password"`
	BotQuota    int    `json:"bot_quota"`
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
	// Bound the request body so an unauthenticated (or buggy) client cannot OOM
	// the orchestrator with a multi-GB payload.
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req createRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Mode != "" && !gamemode.IsValid(req.Mode) {
		writeError(w, http.StatusBadRequest, "unknown mode (valid: "+strings.Join(gamemode.Names(), ", ")+")")
		return
	}
	// Validate Valve's game_type/game_mode matrix and slot/bot ranges. (0,0) is
	// a legitimate competitive default; a validated Mode string, when present,
	// takes precedence over these raw numbers in the manager.
	if req.GameType < 0 || req.GameType > 1 {
		writeError(w, http.StatusBadRequest, "game_type out of range (0-1)")
		return
	}
	if req.GameMode < 0 || req.GameMode > 3 {
		writeError(w, http.StatusBadRequest, "game_mode out of range (0-3)")
		return
	}
	// max_players: 0 is the deployed sentinel for "use the mode/control-plane
	// default" (the bot omits it as 0); any explicit value must be 1-64.
	if req.MaxPlayers < 0 || req.MaxPlayers > 64 {
		writeError(w, http.StatusBadRequest, "max_players out of range (1-64)")
		return
	}
	if req.BotQuota < 0 || req.BotQuota > 64 {
		writeError(w, http.StatusBadRequest, "bot_quota out of range (0-64)")
		return
	}
	if req.Public && req.GSLT == "" {
		// The manager may still substitute a default GSLT; we only warn here by
		// allowing it through. Public without any GSLT will fail to be
		// internet-visible but should not error the request.
		s.log.Warn("public server requested without explicit GSLT", "owner", req.OwnerID)
	}

	if s.maxPer > 0 {
		// An empty owner_id would bypass the per-owner cap entirely (and the
		// manager keys the atomic cap on a non-empty owner), so reject it.
		if req.OwnerID == "" {
			writeError(w, http.StatusBadRequest, "owner_id is required")
			return
		}
		// Advisory pre-check: the authoritative cap is enforced atomically
		// inside the manager's Create (mapped to 409 via ErrLimitExceeded).
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
		OwnerID:     req.OwnerID,
		Name:        req.Name,
		Map:         req.Map,
		WorkshopMap: req.WorkshopMap,
		Mode:        req.Mode,
		GameType:    req.GameType,
		GameMode:    req.GameMode,
		MaxPlayers:  req.MaxPlayers,
		Public:      req.Public,
		GSLT:        req.GSLT,
		Password:    req.Password,
		BotQuota:    req.BotQuota,
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

// handleStopAll stops and removes every server, or just those owned by the
// optional owner_id query param. Each instance is stopped best-effort so a
// single failure doesn't abort the rest; the response reports how many were
// stopped and which ids failed.
func (s *Server) handleStopAll(w http.ResponseWriter, r *http.Request) {
	owner := r.URL.Query().Get("owner_id")
	list, err := s.mgr.List(r.Context(), owner)
	if err != nil {
		s.writeManagerError(w, err)
		return
	}

	stopped := 0
	failed := []string{}
	for _, in := range list {
		if err := s.mgr.Stop(r.Context(), in.ID); err != nil {
			s.log.Error("stop-all: stop failed", "id", in.ID, "err", err)
			failed = append(failed, in.ID)
			continue
		}
		stopped++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "stopped",
		"stopped": stopped,
		"failed":  failed,
	})
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
