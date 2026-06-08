// Package model holds the shared domain types for the control plane. It is a
// leaf package (no internal imports) so both the store and the orchestrator can
// depend on it without an import cycle.
package model

import (
	"errors"
	"strconv"
	"time"
)

// Status is the lifecycle state of a managed server instance.
type Status string

const (
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusStopped  Status = "stopped"
	StatusError    Status = "error"
	StatusUnknown  Status = "unknown"
)

// Common errors returned across the control plane.
var (
	ErrNotFound      = errors.New("server instance not found")
	ErrNoPorts       = errors.New("no free game ports available")
	ErrLimitExceeded = errors.New("per-user server limit exceeded")
)

// CreateOptions describes a requested server instance. Zero-valued fields are
// filled with control-plane defaults by the manager.
type CreateOptions struct {
	// OwnerID identifies who requested the server (e.g. a Discord user ID).
	OwnerID string
	// Name is a human-friendly server name shown in the browser.
	Name string
	// Map is the start map, e.g. "de_inferno".
	Map string
	// Mode is a game-mode preset name (e.g. "competitive", "1v1"). When set it
	// seeds GameType/GameMode/MaxPlayers and the in-game cfg bundle. Explicitly
	// provided GameType/GameMode/MaxPlayers still take precedence.
	Mode string
	// GameType / GameMode follow Valve's dedicated server matrix.
	GameType int
	GameMode int
	// MaxPlayers caps the slot count.
	MaxPlayers int
	// Public requests an Internet-joinable server. When true a GSLT is required
	// (either GSLT below or the control-plane default).
	Public bool
	// GSLT is the Steam Game Server Login Token for public servers. Optional;
	// the manager may substitute a default.
	GSLT string
	// Password optionally protects the server with a join password.
	Password string
	// BotQuota seeds the server with bots (useful for testing/plugins).
	BotQuota int
}

// Instance is the observable state of a managed server.
type Instance struct {
	ID         string    `json:"id"`         // stable short id assigned by the control plane
	BackendID  string    `json:"backend_id"` // backend handle (e.g. docker container id)
	OwnerID    string    `json:"owner_id"`
	Name       string    `json:"name"`
	Map        string    `json:"map"`
	Mode       string    `json:"mode"`
	Status     Status    `json:"status"`
	Public     bool      `json:"public"`
	Host       string    `json:"host"`      // advertised connect host/IP
	GamePort   int       `json:"game_port"` // UDP game port
	RCONPort   int       `json:"rcon_port"` // TCP RCON port
	RCONPass   string    `json:"-"`         // never serialized to clients
	MaxPlayers int       `json:"max_players"`
	CreatedAt  time.Time `json:"created_at"`
}

// ConnectString returns the in-game connect address, e.g. "1.2.3.4:27015".
func (i Instance) ConnectString() string {
	return i.Host + ":" + strconv.Itoa(i.GamePort)
}

// LiveStatus is real-time data pulled from a running server via RCON.
type LiveStatus struct {
	Online      bool   `json:"online"`
	Map         string `json:"map"`
	PlayerCount int    `json:"player_count"`
	MaxPlayers  int    `json:"max_players"`
	Raw         string `json:"raw,omitempty"`
}
