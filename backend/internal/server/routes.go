package server

import (
	"context"
	"net/http"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/buildinfo"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/httpx"
)

// Every route lives here, in one readable place.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /livez", s.handleLive)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("GET /version", s.handleVersion)
}

// Liveness: is the process alive? Failing means "restart me".
// Never checks dependencies: a DB blip must not restart every instance at once.
func (s *Server) handleLive(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readiness: can I serve traffic right now? Failing means "don't route to me".
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "shutting_down"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		s.log.Warn("readiness: database ping failed", "error", err)
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "database_unavailable"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// Version tells you exactly which commit is live. CI's smoke test waits for this to match.
func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"version":   buildinfo.Version,
		"buildTime": buildinfo.BuildTime,
	})
}
