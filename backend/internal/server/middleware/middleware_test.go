package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRequestIDGeneratedAndPropagated(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))

	got := rec.Header().Get("X-Request-ID")
	if got == "" || got != seen {
		t.Fatalf("header %q and context %q should match and be non-empty", got, seen)
	}
}

func TestRequestIDRejectsUnsafeInput(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "bad\nid")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if strings.Contains(rec.Header().Get("X-Request-ID"), "\n") {
		t.Fatal("unsafe request id was passed through")
	}
}

func TestRequestIDKeepsValidInput(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "abc-12345678")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") != "abc-12345678" {
		t.Fatal("valid incoming request id should be kept")
	}
}

func TestRecoverReturnsProblemJSON(t *testing.T) {
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }),
		RequestID, Recover(discardLogger()))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content type %q", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["requestId"] == "" || body["requestId"] == nil {
		t.Fatal("problem body should include the request id")
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Fatal("panic details must not leak to the client")
	}
}

func TestAccessLogRecordsStatusAndNoQueryString(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	h := AccessLog(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x?email=secret@example.com", nil))

	out := buf.String()
	if !strings.Contains(out, `"status":418`) {
		t.Fatalf("status not logged: %s", out)
	}
	if strings.Contains(out, "secret@example.com") {
		t.Fatal("query string leaked into logs")
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := SecurityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))

	for _, k := range []string{"X-Content-Type-Options", "X-Frame-Options", "Cache-Control", "Content-Security-Policy"} {
		if rec.Header().Get(k) == "" {
			t.Errorf("missing header %s", k)
		}
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("API responses must be no-store")
	}
}

func TestChainOrder(t *testing.T) {
	var order []string
	mk := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), mk("a"), mk("b"), mk("c")).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))

	if strings.Join(order, ",") != "a,b,c" {
		t.Fatalf("wrong order: %v", order)
	}
}
