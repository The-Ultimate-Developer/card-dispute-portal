package database

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/config"
)

func TestToPgx5URL(t *testing.T) {
	cases := map[string]string{
		"postgres://u:p@h:5432/db":   "pgx5://u:p@h:5432/db",
		"postgresql://u:p@h:5432/db": "pgx5://u:p@h:5432/db",
	}
	for in, want := range cases {
		if got := toPgx5URL(in); got != want {
			t.Errorf("toPgx5URL(%q) = %q, want %q", in, got, want)
		}
	}
}

// Integration test: runs only when TEST_DATABASE_URL is set (CI provides a Postgres service).
func TestConnectAndMigrate(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Running twice proves migrations are idempotent (second run is a no-op).
	for i := 0; i < 2; i++ {
		if err := Migrate(url, log); err != nil {
			t.Fatalf("migrate run %d: %v", i+1, err)
		}
	}

	pool, err := Connect(ctx, config.DBConfig{URL: url, MaxConns: 2, MinConns: 0, MaxConnLifetime: time.Minute})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	var ok bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'citext')`).Scan(&ok); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !ok {
		t.Fatal("expected citext extension to be installed by migration 000001")
	}
}
