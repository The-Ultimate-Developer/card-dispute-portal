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
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/server/middleware"
)

// Pinger is the only thing the server needs from the database for health checks.
// An interface (not *pgxpool.Pool) so tests can pass a fake.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Server struct {
	httpServer *http.Server
	log        *slog.Logger
	db         Pinger
	ready      atomic.Bool
}

func New(cfg config.Config, log *slog.Logger, db Pinger) *Server {
	s := &Server{log: log, db: db}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           s.buildHandler(problemFallback(mux)),
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

// buildHandler wraps the router in the middleware chain. Outermost first:
//  1. RequestID  - so every later step (including panics) has an ID to log
//  2. Recover    - catches panics from everything below it
//  3. AccessLog  - sees the final status, including the 500 from Recover
//  4. Security headers and body limit
//  5. Timeout    - every request gets a deadline that flows into DB calls
func (s *Server) buildHandler(mux http.Handler) http.Handler {
	return middleware.Chain(mux,
		middleware.RequestID,
		middleware.Recover(s.log),
		middleware.AccessLog(s.log),
		middleware.SecurityHeaders,
		middleware.MaxBody(1<<20), // 1 MB
		func(next http.Handler) http.Handler {
			return http.TimeoutHandler(next, 10*time.Second, `{"title":"Service Unavailable","status":503,"detail":"Request timed out."}`)
		},
	)
}

// Handler exposes the full handler (router + middleware) for tests.
func (s *Server) Handler() http.Handler { return s.httpServer.Handler }

// Run serves until ctx is cancelled (SIGTERM / Ctrl+C), then drains gracefully.
func (s *Server) Run(ctx context.Context, shutdownTimeout time.Duration) error {
	errCh := make(chan error, 1) // buffered: the goroutine never blocks forever

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
	s.ready.Store(false) // readiness fails, so the platform stops routing traffic here

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	s.log.Info("server stopped cleanly")
	return nil
}
