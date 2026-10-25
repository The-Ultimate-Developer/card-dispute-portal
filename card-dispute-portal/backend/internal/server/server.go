package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/config"
)

type Server struct {
	httpServer *http.Server
	log        *slog.Logger
	ready      atomic.Bool
}

func New(cfg config.Config, log *slog.Logger) *Server {
	s := &Server{log: log}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,  // slowloris protection
		ReadTimeout:       10 * time.Second, // whole request incl. body
		WriteTimeout:      15 * time.Second, // writing the response
		IdleTimeout:       60 * time.Second, // keep-alive connections
		MaxHeaderBytes:    1 << 20,          // 1 MB
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}
	s.ready.Store(true)
	return s
}

// Run serves until ctx is cancelled (SIGTERM / Ctrl+C), then drains gracefully.
func (s *Server) Run(ctx context.Context, shutdownTimeout time.Duration) error {
	errCh := make(chan error, 1)

	go func() {
		s.log.Info("http server starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err // failed to start, e.g. port in use
	case <-ctx.Done():
	}

	s.log.Info("shutdown signal received, draining connections")
	s.ready.Store(false) // readiness now fails, so the platform stops sending traffic

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	s.log.Info("server stopped cleanly")
	return nil
}
