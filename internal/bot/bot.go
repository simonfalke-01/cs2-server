// Package bot implements the Discord control surface: slash commands that drive
// the orchestrator API to create and manage on-demand CS2 servers.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/brandonli/cs2-server/internal/apiclient"
)

// Bot wires Discord slash commands to the orchestrator API client.
type Bot struct {
	session     *discordgo.Session
	api         *apiclient.Client
	log         *slog.Logger
	appID       string // configured application ID; fallback when session state lags
	guildID     string // empty => register commands globally
	ownerScoped bool
}

// Config configures the bot.
type Config struct {
	Token   string
	AppID   string // application ID; used as a fallback for command registration
	GuildID string // optional; guild-scoped commands appear instantly
	// OwnerScoped restricts list/stop/etc. to the invoking user's own servers.
	OwnerScoped bool
}

// New constructs a Bot.
func New(cfg Config, api *apiclient.Client, log *slog.Logger) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("bot: new session: %w", err)
	}
	// We only need guild slash commands; no privileged intents required.
	session.Identify.Intents = discordgo.IntentsGuilds

	b := &Bot{
		session:     session,
		api:         api,
		log:         log,
		appID:       cfg.AppID,
		guildID:     cfg.GuildID,
		ownerScoped: cfg.OwnerScoped,
	}
	session.AddHandler(b.onInteraction)
	return b, nil
}

// Run opens the gateway connection, registers commands, and blocks until ctx is
// cancelled, then cleans up registered commands.
func (b *Bot) Run(ctx context.Context) error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("bot: open: %w", err)
	}
	defer b.session.Close()

	// Resolve the application ID for command registration. session.State.User
	// is populated from the gateway READY event, which can briefly lag after
	// Open() returns — dereferencing it directly races and panics. Prefer the
	// session identity when available, otherwise fall back to the configured
	// application ID.
	appID := b.appID
	username := "unknown"
	if u := b.session.State.User; u != nil {
		appID = u.ID
		username = u.Username
	}
	if appID == "" {
		return fmt.Errorf("bot: application id unavailable (session state not ready and DISCORD_APP_ID unset)")
	}
	b.log.Info("bot connected", "user", username, "app_id", appID)

	registered, err := b.registerCommands(appID)
	if err != nil {
		return err
	}
	b.log.Info("commands registered", "count", len(registered), "guild", b.guildID)

	<-ctx.Done()

	// Intentionally do NOT delete commands on shutdown. Deleting + re-creating
	// on every restart leaves a window with zero commands and makes them flicker
	// out of the client; the bulk overwrite on startup already reconciles the
	// set idempotently, so persisting them across restarts is both correct and
	// gap-free.
	return nil
}

// registerCommands installs the slash-command set with a single atomic bulk
// overwrite. ApplicationCommandBulkOverwrite replaces the guild's command set
// in one call (idempotent, no per-command create/delete churn), so restarts
// never leave the guild with a partial or empty command list.
func (b *Bot) registerCommands(appID string) ([]*discordgo.ApplicationCommand, error) {
	defs := commandDefs()
	out, err := b.session.ApplicationCommandBulkOverwrite(appID, b.guildID, defs)
	if err != nil {
		return nil, fmt.Errorf("bot: bulk overwrite commands: %w", err)
	}
	return out, nil
}

// onInteraction dispatches slash commands.
func (b *Bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	data := i.ApplicationCommandData()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch data.Name {
	case "create":
		b.handleCreate(ctx, s, i)
	case "list":
		b.handleList(ctx, s, i)
	case "status":
		b.handleStatus(ctx, s, i)
	case "restart":
		b.handleRestart(ctx, s, i)
	case "stop":
		b.handleStop(ctx, s, i)
	case "killall":
		b.handleKillAll(ctx, s, i)
	default:
		b.respond(s, i, "Unknown command.")
	}
}
