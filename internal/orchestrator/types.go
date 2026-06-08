// Package orchestrator defines the control-plane abstraction for creating and
// managing on-demand CS2 dedicated server instances.
//
// The ServerManager interface is the seam that lets us start on Docker today
// and add a Kubernetes/Agones backend later without touching the API or bot.
package orchestrator

import (
	"context"

	"github.com/brandonli/cs2-server/internal/model"
)

// Re-export the shared domain types so callers can use orchestrator.Instance
// etc. while the canonical definitions live in the leaf model package (this
// avoids an import cycle with the store).
type (
	Status        = model.Status
	CreateOptions = model.CreateOptions
	Instance      = model.Instance
	LiveStatus    = model.LiveStatus
)

const (
	StatusStarting = model.StatusStarting
	StatusRunning  = model.StatusRunning
	StatusStopped  = model.StatusStopped
	StatusError    = model.StatusError
	StatusUnknown  = model.StatusUnknown
)

var (
	ErrNotFound      = model.ErrNotFound
	ErrNoPorts       = model.ErrNoPorts
	ErrLimitExceeded = model.ErrLimitExceeded
)

// ServerManager creates and controls game-server instances on some backend.
type ServerManager interface {
	// Create provisions and starts a new server instance.
	Create(ctx context.Context, opts CreateOptions) (*Instance, error)
	// Stop stops and removes the instance with the given id.
	Stop(ctx context.Context, id string) error
	// Restart restarts the instance with the given id.
	Restart(ctx context.Context, id string) error
	// Get returns the recorded instance by id.
	Get(ctx context.Context, id string) (*Instance, error)
	// List returns recorded instances; if ownerID is non-empty, filtered to it.
	List(ctx context.Context, ownerID string) ([]*Instance, error)
	// Status fetches live status from the running server via RCON.
	Status(ctx context.Context, id string) (*LiveStatus, error)
	// Close releases backend resources held by the manager.
	Close() error
}
