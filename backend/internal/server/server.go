package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/server/middleware"
)

// Pinger is the only thing the server needs from the database for health checks.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Worker is a background loop that runs until its context is cancelled.
type Worker struct {
	Name string
	Run  func(ctx context.Context)
}

type Options struct {
	Port     int
	Log      *slog.Logger
	DB       Pinger
	Routes   func(*http.ServeMux)
	Observer middleware.RequestObserver
	Workers  []Worker
}

type Server struct {
	httpServer *http.Server
	log        *slog.Logger
	db         Pinger
	workers    []Worker
	ready      atomic.Bool
}

// New builds the server. Feature routes are registered by the caller, so this
// package knows nothing about business modules.
func New(opts Options) *Server {
	s := &Server{log: opts.Log, db: opts.DB, workers: opts.Workers}

	mux := http.NewServeMux()
	s.registerHealthRoutes(mux)
	if opts.Routes != nil {
		opts.Routes(mux)
	}

	routeOf := func(r *http.Request) string {
		if _, pattern := mux.Handler(r); pattern != "" {
			return pattern
		}
		return "unmatched"
	}

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", opts.Port),
		Handler:           s.buildHandler(problemFallback(mux), routeOf, opts.Observer),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(opts.Log.Handler(), slog.LevelError),
	}
	s.ready.Store(true)
	return s
}

// buildHandler wraps the router in the middleware chain. Outermost first:
//  1. RequestID  - every later step, including a panic, has an ID to log
//  2. Recover    - catches panics from everything below it
//  3. AccessLog  - sees the final status (including Recover's 500) and records metrics
//  4. Security headers and a body limit
//  5. Timeout    - every request gets a deadline that flows into database calls
func (s *Server) buildHandler(h http.Handler, routeOf func(*http.Request) string, obs middleware.RequestObserver) http.Handler {
	return middleware.Chain(h,
		middleware.RequestID,
		middleware.Recover(s.log),
		middleware.AccessLog(s.log, routeOf, obs),
		middleware.SecurityHeaders,
		middleware.MaxBody(1<<20),
		func(next http.Handler) http.Handler {
			return http.TimeoutHandler(next, 10*time.Second,
				`{"type":"about:blank","title":"Service Unavailable","status":503,"code":"TIMEOUT","detail":"Request timed out."}`)
		},
	)
}

func (s *Server) Handler() http.Handler { return s.httpServer.Handler }

// Run serves until ctx is cancelled (SIGTERM / Ctrl+C), then shuts down in order:
// stop taking traffic, drain in-flight requests, then stop background workers.
func (s *Server) Run(ctx context.Context, shutdownTimeout time.Duration) error {
	workerCtx, stopWorkers := context.WithCancel(context.WithoutCancel(ctx))
	var wg sync.WaitGroup
	for _, w := range s.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.log.Info("worker started", "worker", w.Name)
			w.Run(workerCtx)
			s.log.Info("worker stopped", "worker", w.Name)
		}()
	}
	defer func() {
		stopWorkers()
		wg.Wait()
	}()

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
		return err
	case <-ctx.Done():
	}

	s.log.Info("shutdown signal received, draining connections")
	s.ready.Store(false)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	s.log.Info("server stopped cleanly")
	return nil
}
