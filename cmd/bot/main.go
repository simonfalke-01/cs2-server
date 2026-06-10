// Command bot runs the Discord control surface for the CS2 control plane. It
// registers slash commands and forwards them to the orchestrator API.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/brandonli/cs2-server/internal/apiclient"
	"github.com/brandonli/cs2-server/internal/bot"
	"github.com/brandonli/cs2-server/internal/config"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.RequireBot(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// CS2C_API_TOKEN authenticates the bot to the orchestrator API. It is
	// optional: when unset, no Authorization header is sent and the orchestrator
	// must likewise be running without a token.
	client := apiclient.New(cfg.OrchestratorURL, os.Getenv("CS2C_API_TOKEN"))

	b, err := bot.New(bot.Config{
		Token:       cfg.DiscordToken,
		AppID:       cfg.DiscordAppID,
		GuildID:     cfg.DiscordGuildID,
		OwnerScoped: true,
	}, client, log)
	if err != nil {
		return err
	}

	log.Info("starting bot", "orchestrator", cfg.OrchestratorURL)
	return b.Run(ctx)
}
