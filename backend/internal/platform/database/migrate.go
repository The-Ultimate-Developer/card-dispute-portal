package database

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/migrations"
)

// Migrate applies all pending up-migrations embedded in the binary.
// golang-migrate takes a Postgres advisory lock, so two instances starting
// at once cannot run the same migration twice.
func Migrate(databaseURL string, log *slog.Logger) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("load embedded migrations: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, toPgx5URL(databaseURL))
	if err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	log.Info("database migrations applied", "schema_version", version, "dirty", dirty)
	return nil
}

// golang-migrate picks its driver from the URL scheme; ours is pgx5.
func toPgx5URL(u string) string {
	for _, p := range []string{"postgresql://", "postgres://"} {
		if strings.HasPrefix(u, p) {
			return "pgx5://" + strings.TrimPrefix(u, p)
		}
	}
	return u
}
