// Command orchestrator runs the CS2 control-plane API: it manages on-demand
// CS2 dedicated server containers via the Docker engine and exposes a small
// JSON API consumed by the Discord bot.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brandonli/cs2-server/internal/api"
	"github.com/brandonli/cs2-server/internal/config"
	"github.com/brandonli/cs2-server/internal/orchestrator"
	"github.com/brandonli/cs2-server/internal/reaper"
	"github.com/brandonli/cs2-server/internal/store"
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(cfg.StatePath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := st.Close(); cerr != nil {
			log.Error("store close", "err", cerr)
		}
	}()

	mgr, err := orchestrator.NewDockerManager(ctx, orchestrator.DockerConfig{
		Image:             cfg.CS2Image,
		PublicIP:          cfg.PublicIP,
		GamePortMin:       cfg.GamePortMin,
		GamePortMax:       cfg.GamePortMax,
		DefaultGSLT:       cfg.DefaultGSLT,
		DefaultMap:        cfg.DefaultMap,
		DefaultMode:       cfg.DefaultMode,
		DefaultMaxPlayers: cfg.DefaultMaxPlayers,
		SharedGameFiles:   cfg.SharedGameFiles,
		SharedVolume:      cfg.SharedVolume,
		Network:           cfg.Network,
		MaxPerOwner:       cfg.MaxServersPerUser, // authoritative atomic per-owner cap (H4)
	}, st)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := mgr.Close(); cerr != nil {
			log.Error("manager close", "err", cerr)
		}
	}()

	// Background idle reaper.
	rp := reaper.New(mgr, log, time.Duration(cfg.IdleShutdownMinutes)*time.Minute)
	go rp.Run(ctx)

	if cfg.APIToken == "" {
		log.Warn("SECURITY: CS2C_API_TOKEN is not set — the orchestrator API is UNAUTHENTICATED; " +
			"anyone who can reach the listener can control every server. Set CS2C_API_TOKEN to enable bearer auth.")
	}

	apiSrv := api.New(mgr, log, cfg.MaxServersPerUser, cfg.APIToken)
	httpSrv := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           apiSrv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	log.Info("orchestrator listening", "addr", cfg.APIAddr, "image", cfg.CS2Image)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("orchestrator stopped")
	return nil
}
