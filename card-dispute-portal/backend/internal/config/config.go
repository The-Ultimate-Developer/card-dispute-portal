package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is the single, typed source of runtime settings.
// Everything comes from environment variables (12-factor).
type Config struct {
	Env             string
	Port            int
	LogLevel        string
	ShutdownTimeout time.Duration
}

// Load reads env vars, applies defaults, and validates.
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
		errs = append(errs, fmt.Errorf("PORT must be a number between 1 and 65535"))
	}
	cfg.Port = port

	shutdown, err := time.ParseDuration(getEnv("SHUTDOWN_TIMEOUT", "15s"))
	if err != nil {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err))
	}
	cfg.ShutdownTimeout = shutdown

	return cfg, errors.Join(errs...) // nil when there are no errors
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
