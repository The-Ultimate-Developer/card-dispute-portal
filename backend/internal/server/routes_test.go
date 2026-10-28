package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/config"
)

// fakeDB lets us simulate a healthy or broken database without running one.
type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func newTestServer(db Pinger) *Server {
	return New(config.Config{Port: 0}, slog.New(slog.NewTextHandler(io.Discard, nil)), db)
}

func do(s *Server, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
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
		{"version ok", http.MethodGet, "/version", http.StatusOK},
		{"wrong method", http.MethodPost, "/livez", http.StatusMethodNotAllowed},
		{"unknown route", http.MethodGet, "/nope", http.StatusNotFound},
	}

	srv := newTestServer(fakeDB{})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(srv, tc.method, tc.path)
			if rec.Code != tc.want {
				t.Fatalf("%s %s: got %d, want %d", tc.method, tc.path, rec.Code, tc.want)
			}
		})
	}
}

func TestRouterErrorsUseProblemJSON(t *testing.T) {
	srv := newTestServer(fakeDB{})

	notFound := do(srv, http.MethodGet, "/nope")
	if ct := notFound.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("404 content type %q", ct)
	}

	wrongMethod := do(srv, http.MethodPost, "/livez")
	if ct := wrongMethod.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("405 content type %q", ct)
	}
	if wrongMethod.Header().Get("Allow") == "" {
		t.Fatal("405 must keep the Allow header")
	}
	if !strings.Contains(wrongMethod.Body.String(), `"status":405`) {
		t.Fatalf("unexpected body: %s", wrongMethod.Body.String())
	}
}

func TestReadinessFailsWhenShuttingDown(t *testing.T) {
	srv := newTestServer(fakeDB{})
	srv.ready.Store(false)

	if rec := do(srv, http.MethodGet, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", rec.Code)
	}
}

func TestReadinessFailsWhenDatabaseDown(t *testing.T) {
	srv := newTestServer(fakeDB{err: errors.New("connection refused")})

	if rec := do(srv, http.MethodGet, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", rec.Code)
	}
	// Liveness must stay green: the process itself is fine.
	if rec := do(srv, http.MethodGet, "/livez"); rec.Code != http.StatusOK {
		t.Fatalf("liveness should not depend on the DB, got %d", rec.Code)
	}
}

func TestEveryResponseHasSecurityHeadersAndRequestID(t *testing.T) {
	rec := do(newTestServer(fakeDB{}), http.MethodGet, "/livez")
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("missing X-Request-ID")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
}
