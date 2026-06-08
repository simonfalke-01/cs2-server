// Package gamemode defines the selectable game-mode presets exposed to users
// (Discord /create mode:, API "mode"). A preset maps a friendly name to Valve's
// game_type/game_mode matrix, a default slot count, and an in-game cfg bundle
// the server execs on boot.
//
// It is a leaf package (no internal imports) so the model, orchestrator, api,
// and bot can all depend on it without import cycles.
package gamemode

import (
	"sort"
	"strings"
)

// Preset describes one selectable game mode.
type Preset struct {
	// Name is the canonical, lowercase identifier (e.g. "1v1").
	Name string
	// GameType / GameMode follow Valve's dedicated-server matrix.
	GameType int
	GameMode int
	// MaxPlayers is the default slot count for this mode (used when the request
	// does not specify one).
	MaxPlayers int
	// Cfg is the cfg filename the game container execs for this mode (e.g.
	// "1v1.cfg"). Empty means no mode-specific cfg.
	Cfg string
	// Description is a short human-friendly summary (for Discord choices/docs).
	Description string
}

// registry holds the built-in presets keyed by canonical name.
var registry = map[string]Preset{
	"competitive": {
		Name:        "competitive",
		GameType:    0,
		GameMode:    1,
		MaxPlayers:  10,
		Cfg:         "competitive.cfg",
		Description: "Standard 5v5 competitive",
	},
	"wingman": {
		Name:        "wingman",
		GameType:    0,
		GameMode:    2,
		MaxPlayers:  4,
		Cfg:         "wingman.cfg",
		Description: "2v2 Wingman",
	},
	"deathmatch": {
		Name:        "deathmatch",
		GameType:    1,
		GameMode:    2,
		MaxPlayers:  16,
		Cfg:         "deathmatch.cfg",
		Description: "Free-for-all deathmatch",
	},
	"1v1": {
		Name:        "1v1",
		GameType:    0,
		GameMode:    1,
		MaxPlayers:  12,
		Cfg:         "1v1.cfg",
		Description: "Winner-stays 1v1 arena",
	},
}

// Default is the preset name applied when none is requested.
const Default = "competitive"

// Lookup returns the preset for a name (case-insensitive, surrounding spaces
// ignored) and whether it exists.
func Lookup(name string) (Preset, bool) {
	p, ok := registry[normalize(name)]
	return p, ok
}

// IsValid reports whether name maps to a known preset.
func IsValid(name string) bool {
	_, ok := Lookup(name)
	return ok
}

// Names returns the canonical preset names in a stable, sorted order.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// All returns the presets in canonical-name order.
func All() []Preset {
	names := Names()
	out := make([]Preset, 0, len(names))
	for _, n := range names {
		out = append(out, registry[n])
	}
	return out
}

func normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
