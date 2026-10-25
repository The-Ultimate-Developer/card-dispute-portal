package server

import (
	"net/http"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/httpx"
)

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /livez", s.handleLive)
	mux.HandleFunc("GET /readyz", s.handleReady)
}

// Liveness: is the process alive? Failing this means "restart me".
func (s *Server) handleLive(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readiness: can I serve traffic right now? Failing this means "don't route to me".
// Later this also checks the database.
func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	if !s.ready.Load() {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "shutting_down"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
