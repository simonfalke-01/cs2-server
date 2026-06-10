// Package gamemode defines the selectable game-mode presets exposed to users
// (Discord /create mode:, API "mode"). A preset maps a friendly name to Valve's
// game_type/game_mode matrix and a default slot count.
//
// Valve's dedicated-server matrix uses game_type/game_mode pairs; the zero pair
// (game_type=0, game_mode=0) is the engine default and resolves to Casual.
// "competitive" (the package Default) is game_type=0, game_mode=1.
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
	// NoBots forces the server to run without bots regardless of a requested
	// bot quota (e.g. the 1v1 arena is human-only; bots would also defeat idle
	// reaping by keeping the server perpetually "occupied").
	NoBots bool
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
		Description: "Standard 5v5 competitive",
	},
	"wingman": {
		Name:        "wingman",
		GameType:    0,
		GameMode:    2,
		MaxPlayers:  4,
		Description: "2v2 Wingman",
	},
	"deathmatch": {
		Name:        "deathmatch",
		GameType:    1,
		GameMode:    2,
		MaxPlayers:  16,
		Description: "Free-for-all deathmatch",
	},
	"1v1": {
		Name:        "1v1",
		GameType:    0,
		GameMode:    1,
		MaxPlayers:  8, // 2 active duelers + up to 6 spectators
		NoBots:      true,
		Description: "Two-player 1v1 duel",
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
