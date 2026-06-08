package orchestrator

import "testing"

// newDefaultsManager builds a DockerManager with only the config fields that
// applyDefaults reads. applyDefaults touches no Docker client, so this is safe
// without a real engine.
func newDefaultsManager() *DockerManager {
	return &DockerManager{cfg: DockerConfig{
		DefaultMap:        "de_inferno",
		DefaultMode:       "competitive",
		DefaultMaxPlayers: 10,
	}}
}

func TestApplyDefaults_PresetSeedsTypeModeAndSlots(t *testing.T) {
	m := newDefaultsManager()
	opts := CreateOptions{Mode: "1v1"}
	m.applyDefaults(&opts)

	if opts.Mode != "1v1" {
		t.Fatalf("mode = %q, want 1v1", opts.Mode)
	}
	if opts.GameType != 0 || opts.GameMode != 1 {
		t.Fatalf("type/mode = %d/%d, want 0/1", opts.GameType, opts.GameMode)
	}
	if opts.MaxPlayers != 12 {
		t.Fatalf("maxplayers = %d, want 12 (1v1 preset)", opts.MaxPlayers)
	}
}

func TestApplyDefaults_EmptyModeUsesConfigDefault(t *testing.T) {
	m := newDefaultsManager()
	opts := CreateOptions{}
	m.applyDefaults(&opts)

	if opts.Mode != "competitive" {
		t.Fatalf("mode = %q, want competitive (config default)", opts.Mode)
	}
	if opts.MaxPlayers != 10 {
		t.Fatalf("maxplayers = %d, want 10 (competitive preset)", opts.MaxPlayers)
	}
}

func TestApplyDefaults_ExplicitValuesOverridePreset(t *testing.T) {
	m := newDefaultsManager()
	opts := CreateOptions{Mode: "1v1", MaxPlayers: 4, GameType: 0, GameMode: 2}
	m.applyDefaults(&opts)

	if opts.MaxPlayers != 4 {
		t.Fatalf("maxplayers = %d, want 4 (explicit wins)", opts.MaxPlayers)
	}
	if opts.GameMode != 2 {
		t.Fatalf("game_mode = %d, want 2 (explicit wins)", opts.GameMode)
	}
}

func TestApplyDefaults_1v1ForcesNoBots(t *testing.T) {
	m := newDefaultsManager()
	opts := CreateOptions{Mode: "1v1", BotQuota: 8}
	m.applyDefaults(&opts)

	if opts.BotQuota != 0 {
		t.Fatalf("bot_quota = %d, want 0 (1v1 is human-only)", opts.BotQuota)
	}
}

func TestApplyDefaults_BotsAllowedInOtherModes(t *testing.T) {
	m := newDefaultsManager()
	opts := CreateOptions{Mode: "deathmatch", BotQuota: 8}
	m.applyDefaults(&opts)

	if opts.BotQuota != 8 {
		t.Fatalf("bot_quota = %d, want 8 (non-1v1 keeps requested bots)", opts.BotQuota)
	}
}

func TestApplyDefaults_UnknownModeFallsBackToDefaults(t *testing.T) {
	m := newDefaultsManager()
	opts := CreateOptions{Mode: "surf"}
	m.applyDefaults(&opts)

	// Unknown mode is left as-is (validated at the API edge) but slot/game
	// defaults still apply so the container is launchable.
	if opts.MaxPlayers != 10 {
		t.Fatalf("maxplayers = %d, want 10 (fallback default)", opts.MaxPlayers)
	}
	if opts.GameMode != 1 || opts.GameType != 0 {
		t.Fatalf("type/mode = %d/%d, want 0/1 (fallback default)", opts.GameType, opts.GameMode)
	}
}
