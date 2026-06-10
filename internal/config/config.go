// Package config loads runtime configuration for the control plane (the
// orchestrator API and the Discord bot) from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/brandonli/cs2-server/internal/gamemode"
)

// Config holds all settings for the control plane. Secrets are sourced from the
// environment and never logged.
type Config struct {
	// Orchestrator
	APIAddr   string // listen address for the orchestrator HTTP API, e.g. ":8080"
	StatePath string // path to the SQLite state file

	// APIToken is the shared bearer token guarding the orchestrator API. When
	// empty, auth is disabled (and the orchestrator logs a startup security
	// warning); when set, every /v1/* request must present it. The bot's
	// apiclient sends the same value via CS2C_API_TOKEN.
	APIToken string // CS2C_API_TOKEN

	// CS2 game image + container settings
	CS2Image string // docker image used for game servers

	// Docker network that game containers join so the orchestrator can reach
	// their RCON port by container name (set to the compose network).
	Network string

	// Named Docker volumes (no host paths; portable across machines).
	SharedVolume string // shared seeded game copy (shared-files mode)

	// Shared game files (OverlayFS). When enabled, all instances share one
	// read-only ~40GB game copy and each server only stores a thin writable
	// layer. Requires OverlayFS or fuse-overlayfs; grants game containers
	// CAP_SYS_ADMIN + /dev/fuse to mount.
	SharedGameFiles bool // CS2C_SHARED_GAME_FILES

	// Networking / port allocation for on-demand servers
	GamePortMin int    // inclusive lower bound for UDP game ports
	GamePortMax int    // inclusive upper bound for UDP game ports
	PublicIP    string // advertised IP players connect to (for the connect string)

	// Defaults applied to new servers
	DefaultMap        string
	DefaultMode       string // game-mode preset applied when none is requested
	DefaultMaxPlayers int
	DefaultGSLT       string // fallback Steam GSLT for public servers
	SteamAPIKey       string // optional, for future stats/workshop features

	// Discord bot
	DiscordToken      string
	DiscordGuildID    string // optional: register guild-scoped commands for fast iteration
	DiscordAppID      string
	OrchestratorURL   string // base URL the bot uses to reach the orchestrator API
	MaxServersPerUser int

	// Idle reaper
	IdleShutdownMinutes int // stop a server after this many minutes with 0 players (0 disables)
}

// Load reads configuration from the environment, applying sensible defaults.
func Load() (*Config, error) {
	c := &Config{
		APIAddr:         getEnv("CS2C_API_ADDR", ":8080"),
		StatePath:       getEnv("CS2C_STATE_PATH", "data/cs2-server.db"),
		APIToken:        getEnv("CS2C_API_TOKEN", ""),
		CS2Image:        getEnv("CS2C_IMAGE", "cs2-server/cs2:latest"),
		Network:         getEnv("CS2C_NETWORK", ""),
		SharedVolume:    getEnv("CS2C_SHARED_VOLUME", "cs2-shared"),
		PublicIP:        getEnv("CS2C_PUBLIC_IP", "127.0.0.1"),
		DefaultMap:      getEnv("CS2C_DEFAULT_MAP", "de_inferno"),
		DefaultMode:     getEnv("CS2C_DEFAULT_MODE", gamemode.Default),
		DefaultGSLT:     getEnv("CS2C_DEFAULT_GSLT", ""),
		SteamAPIKey:     getEnv("CS2C_STEAM_API_KEY", ""),
		DiscordToken:    getEnv("DISCORD_TOKEN", ""),
		DiscordGuildID:  getEnv("DISCORD_GUILD_ID", ""),
		DiscordAppID:    getEnv("DISCORD_APP_ID", ""),
		OrchestratorURL: getEnv("CS2C_ORCHESTRATOR_URL", "http://127.0.0.1:8080"),
	}

	var err error
	if c.GamePortMin, err = getEnvInt("CS2C_GAME_PORT_MIN", 27015); err != nil {
		return nil, err
	}
	if c.GamePortMax, err = getEnvInt("CS2C_GAME_PORT_MAX", 27115); err != nil {
		return nil, err
	}
	if c.DefaultMaxPlayers, err = getEnvInt("CS2C_DEFAULT_MAXPLAYERS", 10); err != nil {
		return nil, err
	}
	if c.MaxServersPerUser, err = getEnvInt("CS2C_MAX_SERVERS_PER_USER", 2); err != nil {
		return nil, err
	}
	if c.IdleShutdownMinutes, err = getEnvInt("CS2C_IDLE_SHUTDOWN_MINUTES", 30); err != nil {
		return nil, err
	}

	c.SharedGameFiles = getEnvBool("CS2C_SHARED_GAME_FILES", false)

	if !gamemode.IsValid(c.DefaultMode) {
		return nil, fmt.Errorf("config: CS2C_DEFAULT_MODE %q is not a known game mode (valid: %s)",
			c.DefaultMode, strings.Join(gamemode.Names(), ", "))
	}

	if c.GamePortMin > c.GamePortMax {
		return nil, fmt.Errorf("config: CS2C_GAME_PORT_MIN (%d) must be <= CS2C_GAME_PORT_MAX (%d)", c.GamePortMin, c.GamePortMax)
	}

	return c, nil
}

// RequireBot validates that fields needed by the Discord bot are present.
func (c *Config) RequireBot() error {
	var missing []string
	if c.DiscordToken == "" {
		missing = append(missing, "DISCORD_TOKEN")
	}
	if c.DiscordAppID == "" {
		missing = append(missing, "DISCORD_APP_ID")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required bot settings: %s", strings.Join(missing, ", "))
	}
	return nil
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func getEnvInt(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer, got %q", key, v)
	}
	return n, nil
}

func getEnvBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
