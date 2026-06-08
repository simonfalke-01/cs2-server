// Package ports provides a small, concurrency-safe allocator for the UDP game
// ports (and derived RCON ports) handed out to on-demand server instances.
package ports

import (
	"errors"
	"sync"
)

// ErrExhausted is returned when no ports remain in the configured range.
var ErrExhausted = errors.New("ports: no free ports in range")

// Allocator hands out ports from [min, max]. It is safe for concurrent use.
//
// The set of in-use ports is seeded from existing instances at startup so
// allocations survive a control-plane restart.
type Allocator struct {
	mu   sync.Mutex
	min  int
	max  int
	used map[int]bool
	next int
}

// New returns an allocator over the inclusive range [min, max]. The provided
// reserved ports are marked as already in use.
func New(min, max int, reserved ...int) *Allocator {
	a := &Allocator{
		min:  min,
		max:  max,
		used: make(map[int]bool),
		next: min,
	}
	for _, p := range reserved {
		if p >= min && p <= max {
			a.used[p] = true
		}
	}
	return a
}

// Acquire returns the next free port, marking it in use.
func (a *Allocator) Acquire() (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	n := a.max - a.min + 1
	for i := 0; i < n; i++ {
		p := a.next
		a.next++
		if a.next > a.max {
			a.next = a.min
		}
		if !a.used[p] {
			a.used[p] = true
			return p, nil
		}
	}
	return 0, ErrExhausted
}

// Release returns a port to the pool.
func (a *Allocator) Release(p int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.used, p)
}

// Reserve marks a specific port as used (e.g. when reconciling existing state).
func (a *Allocator) Reserve(p int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if p >= a.min && p <= a.max {
		a.used[p] = true
	}
}
