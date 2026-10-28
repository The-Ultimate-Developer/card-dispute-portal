package config

import (
	"testing"
	"time"
)

const testDBURL = "postgres://user:pass@localhost:5432/disputes?sslmode=disable"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", testDBURL)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 8080 || cfg.Env != "development" || cfg.ShutdownTimeout != 15*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.DB.MaxConns != 10 || !cfg.DB.RunMigrations {
		t.Fatalf("unexpected DB defaults: %+v", cfg.DB)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name, key, value string
	}{
		{"port not a number", "PORT", "abc"},
		{"port out of range", "PORT", "70000"},
		{"unknown env", "APP_ENV", "staging"},
		{"bad duration", "SHUTDOWN_TIMEOUT", "soon"},
		{"negative duration", "SHUTDOWN_TIMEOUT", "-5s"},
		{"not a postgres url", "DATABASE_URL", "mysql://x"},
		{"too many conns", "DB_MAX_CONNS", "1000"},
		{"min above max", "DB_MIN_CONNS", "50"},
		{"bad bool", "DB_RUN_MIGRATIONS", "maybe"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", testDBURL)
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatalf("expected error for %s=%q", tc.key, tc.value)
			}
		})
	}
}
