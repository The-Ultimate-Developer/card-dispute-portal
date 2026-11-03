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
	if cfg.Auth.MaxFailedAttempts != 3 || cfg.Auth.IdleTimeout != 15*time.Minute || cfg.Auth.CookieSecure {
		t.Fatalf("unexpected auth defaults: %+v", cfg.Auth)
	}
	if cfg.SeedDemoData || !cfg.EnableDocs || cfg.TrustedProxyHops != 0 {
		t.Fatalf("unexpected feature defaults: %+v", cfg)
	}
}

func TestSecureCookiesByDefaultOutsideDevelopment(t *testing.T) {
	t.Setenv("DATABASE_URL", testDBURL)
	t.Setenv("APP_ENV", "production")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Auth.CookieSecure {
		t.Fatal("production must default to secure cookies")
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
		name string
		env  map[string]string
	}{
		{"port not a number", map[string]string{"PORT": "abc"}},
		{"port out of range", map[string]string{"PORT": "70000"}},
		{"unknown env", map[string]string{"APP_ENV": "staging"}},
		{"bad duration", map[string]string{"SHUTDOWN_TIMEOUT": "soon"}},
		{"negative duration", map[string]string{"SHUTDOWN_TIMEOUT": "-5s"}},
		{"not a postgres url", map[string]string{"DATABASE_URL": "mysql://x"}},
		{"too many conns", map[string]string{"DB_MAX_CONNS": "1000"}},
		{"min above max", map[string]string{"DB_MIN_CONNS": "50"}},
		{"bad bool", map[string]string{"DB_RUN_MIGRATIONS": "maybe"}},
		{"lockout of zero", map[string]string{"LOGIN_MAX_FAILED_ATTEMPTS": "0"}},
		{"idle beyond absolute", map[string]string{"SESSION_IDLE_TIMEOUT": "9h"}},
		{"insecure cookies in production", map[string]string{"APP_ENV": "production", "COOKIE_SECURE": "false"}},
		{"error rate above one", map[string]string{"MOCK_CORE_BANKING_ERROR_RATE": "1.5"}},
		{"too many proxy hops", map[string]string{"TRUSTED_PROXY_HOPS": "9"}},
		{"weak metrics token", map[string]string{"METRICS_TOKEN": "short"}},
		{"breaker threshold zero", map[string]string{"CORE_BANKING_BREAKER_THRESHOLD": "0"}},
		{"notify attempts zero", map[string]string{"NOTIFY_MAX_ATTEMPTS": "0"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", testDBURL)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatalf("expected error for %v", tc.env)
			}
		})
	}
}
