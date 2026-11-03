package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/migrations"
)

// TestMigrationsRoundTrip proves every down migration actually reverses its up
// migration, so a bad release can be rolled back with confidence.
func TestMigrationsRoundTrip(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	dbURL := freshDatabase(t, adminURL)

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, toPgx5URL(dbURL))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil {
		t.Fatalf("up: %v", err)
	}
	if err := m.Down(); err != nil {
		t.Fatalf("down: %v", err)
	}
	if _, _, err := m.Version(); !errors.Is(err, migrate.ErrNilVersion) {
		t.Fatalf("expected an empty schema after down, got %v", err)
	}
	if err := Migrate(dbURL, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("up again: %v", err)
	}
}

func freshDatabase(t *testing.T, adminURL string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "roundtrip_" + hex.EncodeToString(b)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if c, err := pgx.Connect(ctx, adminURL); err == nil {
			_, _ = c.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			_ = c.Close(ctx)
		}
	})

	u, _ := url.Parse(adminURL)
	u.Path = "/" + name
	return u.String()
}
