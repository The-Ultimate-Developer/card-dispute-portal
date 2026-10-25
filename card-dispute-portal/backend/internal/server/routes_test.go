package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/config"
)

func newTestServer() *Server {
	return New(config.Config{Port: 0}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestHealthEndpoints(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"liveness ok", http.MethodGet, "/livez", http.StatusOK},
		{"readiness ok", http.MethodGet, "/readyz", http.StatusOK},
		{"wrong method", http.MethodPost, "/livez", http.StatusMethodNotAllowed},
		{"unknown route", http.MethodGet, "/nope", http.StatusNotFound},
	}

	srv := newTestServer()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)

			srv.httpServer.Handler.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("%s %s: got %d, want %d", tc.method, tc.path, rec.Code, tc.want)
			}
		})
	}
}

func TestReadinessFailsWhenShuttingDown(t *testing.T) {
	srv := newTestServer()
	srv.ready.Store(false)

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", rec.Code)
	}
}
