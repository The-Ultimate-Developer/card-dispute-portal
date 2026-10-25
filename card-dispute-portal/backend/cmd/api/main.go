package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/config"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/logger"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.LogLevel).With("service", "dispute-api", "env", cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(cfg, log)
	return srv.Run(ctx, cfg.ShutdownTimeout)
}
