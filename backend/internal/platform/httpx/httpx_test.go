package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
)

func newReq(method, body string) *http.Request {
	r := httptest.NewRequestWithContext(context.Background(), method, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	tests := []struct {
		name     string
		body     string
		ctype    string
		wantKind apperr.Kind
		wantOK   bool
	}{
		{"valid", `{"name":"a","age":1}`, "application/json", 0, true},
		{"charset allowed", `{"name":"a"}`, "application/json; charset=utf-8", 0, true},
		{"form content type rejected", `{"name":"a"}`, "application/x-www-form-urlencoded", apperr.KindUnsupportedMedia, false},
		{"unknown field", `{"name":"a","admin":true}`, "application/json", apperr.KindValidation, false},
		{"wrong type", `{"age":"x"}`, "application/json", apperr.KindValidation, false},
		{"malformed", `{"name":`, "application/json", apperr.KindValidation, false},
		{"two objects", `{"name":"a"}{"name":"b"}`, "application/json", apperr.KindValidation, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newReq(http.MethodPost, tc.body)
			r.Header.Set("Content-Type", tc.ctype)
			var p payload
			err := DecodeJSON(r, &p)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if apperr.KindOf(err) != tc.wantKind {
				t.Fatalf("got kind %v (%v), want %v", apperr.KindOf(err), err, tc.wantKind)
			}
		})
	}
}

func TestDecodeJSONBodyTooLarge(t *testing.T) {
	r := newReq(http.MethodPost, `{"name":"`+strings.Repeat("a", 100)+`"}`)
	r.Body = http.MaxBytesReader(httptest.NewRecorder(), r.Body, 10)
	var v map[string]string
	if apperr.KindOf(DecodeJSON(r, &v)) != apperr.KindTooLarge {
		t.Fatal("expected KindTooLarge")
	}
}

func TestWriteErrorMapsKnownErrors(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rec := httptest.NewRecorder()
	WriteError(rec, newReq(http.MethodGet, ""), log, apperr.RateLimited(1500*time.Millisecond))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "2" {
		t.Fatalf("Retry-After %q, want rounded up to 2", rec.Header().Get("Retry-After"))
	}
	var p Problem
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if p.Code != "RATE_LIMITED" {
		t.Fatalf("code %q", p.Code)
	}
}

func TestWriteErrorHidesInternalErrors(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rec := httptest.NewRecorder()
	WriteError(rec, newReq(http.MethodGet, ""), log, errors.New("pq: password authentication failed for user admin"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatal("internal error details leaked to client")
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name string
		xff  string
		hops int
		want string
	}{
		{"no proxy uses socket address", "6.6.6.6", 0, "10.0.0.1"},
		{"one hop takes rightmost", "6.6.6.6, 41.1.1.1", 1, "41.1.1.1"},
		{"two hops skips the inner proxy", "41.1.1.1, 52.2.2.2", 2, "41.1.1.1"},
		{"spoofed left entries ignored", "1.2.3.4, 41.1.1.1, 52.2.2.2", 2, "41.1.1.1"},
		{"fewer entries than hops falls back", "41.1.1.1", 2, "10.0.0.1"},
		{"garbage falls back", "not-an-ip", 1, "10.0.0.1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newReq(http.MethodGet, "")
			r.RemoteAddr = "10.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", tc.xff)
			if got := ClientIP(r, tc.hops); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
