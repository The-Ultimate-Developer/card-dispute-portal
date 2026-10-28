package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/buildinfo"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/config"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/database"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/logger"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// run returns an error instead of exiting, so every defer (closing the pool) always runs.
// main.go is wiring only: config -> logger -> database -> server.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.LogLevel).With("service", "dispute-api", "env", cfg.Env, "version", buildinfo.Version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.DB.RunMigrations {
		if err := database.Migrate(cfg.DB.URL, log); err != nil {
			return err
		}
	}

	pool, err := database.Connect(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.Info("database connected", "max_conns", cfg.DB.MaxConns)

	srv := server.New(cfg, log, pool)
	return srv.Run(ctx, cfg.ShutdownTimeout)
}
