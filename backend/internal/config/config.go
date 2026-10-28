package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Config is the single, typed source of runtime settings.
// Everything comes from environment variables (12-factor), validated at startup.
type Config struct {
	Env             string
	Port            int
	LogLevel        string
	ShutdownTimeout time.Duration
	DB              DBConfig
}

type DBConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	RunMigrations   bool
}

// Load reads env vars, applies defaults and validates.
// It reports ALL problems at once, so a bad deploy fails with one clear message.
func Load() (Config, error) {
	var errs []error

	cfg := Config{
		Env:      getEnv("APP_ENV", "development"),
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	switch cfg.Env {
	case "development", "test", "production":
	default:
		errs = append(errs, fmt.Errorf("APP_ENV must be development, test or production, got %q", cfg.Env))
	}

	port, err := strconv.Atoi(getEnv("PORT", "8080"))
	if err != nil || port < 1 || port > 65535 {
		errs = append(errs, errors.New("PORT must be a number between 1 and 65535"))
	}
	cfg.Port = port

	cfg.ShutdownTimeout = parseDuration("SHUTDOWN_TIMEOUT", "15s", &errs)

	// --- Database ---
	cfg.DB.URL = os.Getenv("DATABASE_URL")
	if cfg.DB.URL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	} else if u, err := url.Parse(cfg.DB.URL); err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		errs = append(errs, errors.New("DATABASE_URL must be a postgres:// URL"))
	}

	cfg.DB.MaxConns = parseInt32("DB_MAX_CONNS", "10", 1, 100, &errs)
	cfg.DB.MinConns = parseInt32("DB_MIN_CONNS", "1", 0, 100, &errs)
	if cfg.DB.MinConns > cfg.DB.MaxConns {
		errs = append(errs, errors.New("DB_MIN_CONNS cannot exceed DB_MAX_CONNS"))
	}
	cfg.DB.MaxConnLifetime = parseDuration("DB_MAX_CONN_LIFETIME", "30m", &errs)

	run, err := strconv.ParseBool(getEnv("DB_RUN_MIGRATIONS", "true"))
	if err != nil {
		errs = append(errs, errors.New("DB_RUN_MIGRATIONS must be true or false"))
	}
	cfg.DB.RunMigrations = run

	return cfg, errors.Join(errs...) // nil when there are no errors
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func parseDuration(key, fallback string, errs *[]error) time.Duration {
	d, err := time.ParseDuration(getEnv(key, fallback))
	if err != nil || d <= 0 {
		*errs = append(*errs, fmt.Errorf("%s must be a positive duration like 15s", key))
	}
	return d
}

func parseInt32(key, fallback string, lo, hi int32, errs *[]error) int32 {
	n, err := strconv.ParseInt(getEnv(key, fallback), 10, 32) // bitSize 32: can't overflow int32
	if err != nil || n < int64(lo) || n > int64(hi) {
		*errs = append(*errs, fmt.Errorf("%s must be a number between %d and %d", key, lo, hi))
		return 0
	}
	return int32(n)
}
