// Package testdb gives each integration test its own freshly migrated database,
// so tests can run in parallel across packages without sharing state.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/config"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/database"
)

func New(t *testing.T) *pgxpool.Pool {
	t.Helper()

	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "test_" + hex.EncodeToString(b)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil { //nolint:gosec // name is generated here, never user input
		t.Fatalf("create database: %v", err)
	}

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	u.Path = "/" + name
	dbURL := u.String()

	if err := database.Migrate(dbURL, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := database.Connect(ctx, config.DBConfig{URL: dbURL, MaxConns: 10, MaxConnLifetime: time.Minute})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close(context.Background()) }()
		_, _ = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)") //nolint:gosec // generated name
	})
	return pool
}

func CreateUser(t *testing.T, db *pgxpool.Pool, email, role, passwordHash string) string {
	t.Helper()
	var id string
	err := db.QueryRow(context.Background(),
		`INSERT INTO users (email, full_name, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING id`,
		email, "Test "+role, passwordHash, role).Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

func CreateCard(t *testing.T, db *pgxpool.Pool, customerID, last4 string) string {
	t.Helper()
	var id string
	err := db.QueryRow(context.Background(),
		`INSERT INTO cards (customer_id, brand, last4) VALUES ($1, 'VISA', $2) RETURNING id`,
		customerID, last4).Scan(&id)
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	return id
}

func CreateTransaction(t *testing.T, db *pgxpool.Pool, cardID, status string, amountCents int64, occurredAt time.Time) string {
	t.Helper()
	var id string
	err := db.QueryRow(context.Background(),
		`INSERT INTO transactions (card_id, merchant, category, amount_cents, status, occurred_at)
		 VALUES ($1, 'Test Merchant', 'Shopping', $2, $3, $4) RETURNING id`,
		cardID, amountCents, status, occurredAt).Scan(&id)
	if err != nil {
		t.Fatalf("create transaction: %v", err)
	}
	return id
}
