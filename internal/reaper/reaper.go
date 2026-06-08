// Package reaper stops idle servers: instances that have had zero players for
// longer than a configured grace period. This keeps on-demand capacity from
// leaking when users forget to /stop.
package reaper

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/brandonli/cs2-server/internal/orchestrator"
)

// Reaper periodically polls live status and stops servers idle too long.
type Reaper struct {
	mgr      orchestrator.ServerManager
	log      *slog.Logger
	idleFor  time.Duration
	interval time.Duration

	mu         sync.Mutex
	emptySince map[string]time.Time
}

// New builds a reaper. idleFor is the grace period; if zero, Run is a no-op.
func New(mgr orchestrator.ServerManager, log *slog.Logger, idleFor time.Duration) *Reaper {
	return &Reaper{
		mgr:        mgr,
		log:        log,
		idleFor:    idleFor,
		interval:   time.Minute,
		emptySince: make(map[string]time.Time),
	}
}

// Run polls until ctx is cancelled.
func (r *Reaper) Run(ctx context.Context) {
	if r.idleFor <= 0 {
		r.log.Info("idle reaper disabled")
		return
	}
	r.log.Info("idle reaper started", "idle_for", r.idleFor, "interval", r.interval)

	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.tick(ctx)
		}
	}
}

func (r *Reaper) tick(ctx context.Context) {
	instances, err := r.mgr.List(ctx, "")
	if err != nil {
		r.log.Error("reaper list failed", "err", err)
		return
	}

	live := make(map[string]bool, len(instances))
	for _, in := range instances {
		live[in.ID] = true

		st, err := r.mgr.Status(ctx, in.ID)
		if err != nil || !st.Online {
			// Can't determine; don't accumulate idle time.
			r.clear(in.ID)
			continue
		}

		if st.PlayerCount > 0 {
			r.clear(in.ID)
			continue
		}

		first := r.mark(in.ID)
		if time.Since(first) >= r.idleFor {
			r.log.Info("stopping idle server", "id", in.ID, "idle_since", first)
			if err := r.mgr.Stop(ctx, in.ID); err != nil {
				r.log.Error("reaper stop failed", "id", in.ID, "err", err)
			} else {
				r.clear(in.ID)
			}
		}
	}

	// Drop tracking entries for instances that no longer exist.
	r.gc(live)
}

// mark records (once) when an instance was first seen empty and returns it.
func (r *Reaper) mark(id string) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.emptySince[id]; ok {
		return t
	}
	now := time.Now()
	r.emptySince[id] = now
	return now
}

func (r *Reaper) clear(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.emptySince, id)
}

func (r *Reaper) gc(live map[string]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id := range r.emptySince {
		if !live[id] {
			delete(r.emptySince, id)
		}
	}
}
