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
	guildID     string // empty => register commands globally
	ownerScoped bool
}

// Config configures the bot.
type Config struct {
	Token   string
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

	appID := b.session.State.User.ID
	b.log.Info("bot connected", "user", b.session.State.User.Username, "app_id", appID)

	registered, err := b.registerCommands(appID)
	if err != nil {
		return err
	}
	b.log.Info("commands registered", "count", len(registered), "guild", b.guildID)

	<-ctx.Done()

	// Best-effort cleanup of guild commands so stale schemas don't linger.
	b.cleanupCommands(appID, registered)
	return nil
}

func (b *Bot) registerCommands(appID string) ([]*discordgo.ApplicationCommand, error) {
	defs := commandDefs()
	out := make([]*discordgo.ApplicationCommand, 0, len(defs))
	for _, d := range defs {
		c, err := b.session.ApplicationCommandCreate(appID, b.guildID, d)
		if err != nil {
			return nil, fmt.Errorf("bot: register %q: %w", d.Name, err)
		}
		out = append(out, c)
	}
	return out, nil
}

func (b *Bot) cleanupCommands(appID string, cmds []*discordgo.ApplicationCommand) {
	// Only clean up guild-scoped commands; global ones are expensive to churn.
	if b.guildID == "" {
		return
	}
	for _, c := range cmds {
		if err := b.session.ApplicationCommandDelete(appID, b.guildID, c.ID); err != nil {
			b.log.Warn("failed to delete command", "name", c.Name, "err", err)
		}
	}
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
	default:
		b.respond(s, i, "Unknown command.")
	}
}
