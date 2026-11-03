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
	Auth            AuthConfig
	CoreBanking     CoreBankingConfig
	RateLimits      RateLimitConfig
	Notify          NotifyConfig
	MetricsToken    string
	SeedDemoData    bool
	EnableDocs      bool
	// TrustedProxyHops is how many proxies append to X-Forwarded-For in front of
	// the API: 0 locally, 2 on Render (Cloudflare, then Render's proxy).
	TrustedProxyHops int
}

type DBConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	RunMigrations   bool
}

type AuthConfig struct {
	MaxFailedAttempts int
	IdleTimeout       time.Duration
	AbsoluteTimeout   time.Duration
	CookieSecure      bool
}

type CoreBankingConfig struct {
	Timeout          time.Duration
	BreakerThreshold int
	BreakerOpenFor   time.Duration
	MockLatency      time.Duration
	MockErrorRate    float64
}

type NotifyConfig struct {
	PollInterval time.Duration
	MaxAttempts  int
}

type RateLimitConfig struct {
	LoginPerMinute int
	WritesPerHour  int
}

// Load reads env vars, applies defaults and validates.
// It reports ALL problems at once, so a bad deploy fails with one clear message.
func Load() (Config, error) {
	var errs []error
	p := parser{errs: &errs}

	cfg := Config{
		Env:      getEnv("APP_ENV", "development"),
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	switch cfg.Env {
	case "development", "test", "production":
	default:
		errs = append(errs, fmt.Errorf("APP_ENV must be development, test or production, got %q", cfg.Env))
	}

	cfg.Port = p.intRange("PORT", "8080", 1, 65535)
	cfg.ShutdownTimeout = p.duration("SHUTDOWN_TIMEOUT", "15s", false)

	cfg.DB.URL = os.Getenv("DATABASE_URL")
	if cfg.DB.URL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	} else if u, err := url.Parse(cfg.DB.URL); err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		errs = append(errs, errors.New("DATABASE_URL must be a postgres:// URL"))
	}
	cfg.DB.MaxConns = p.int32Range("DB_MAX_CONNS", "10", 1, 100)
	cfg.DB.MinConns = p.int32Range("DB_MIN_CONNS", "1", 0, 100)
	if cfg.DB.MinConns > cfg.DB.MaxConns {
		errs = append(errs, errors.New("DB_MIN_CONNS cannot exceed DB_MAX_CONNS"))
	}
	cfg.DB.MaxConnLifetime = p.duration("DB_MAX_CONN_LIFETIME", "30m", false)
	cfg.DB.RunMigrations = p.boolean("DB_RUN_MIGRATIONS", "true")

	cfg.Auth.MaxFailedAttempts = p.intRange("LOGIN_MAX_FAILED_ATTEMPTS", "3", 1, 10)
	cfg.Auth.IdleTimeout = p.duration("SESSION_IDLE_TIMEOUT", "15m", false)
	cfg.Auth.AbsoluteTimeout = p.duration("SESSION_ABSOLUTE_TIMEOUT", "8h", false)
	if cfg.Auth.IdleTimeout > cfg.Auth.AbsoluteTimeout {
		errs = append(errs, errors.New("SESSION_IDLE_TIMEOUT cannot exceed SESSION_ABSOLUTE_TIMEOUT"))
	}
	cfg.Auth.CookieSecure = p.boolean("COOKIE_SECURE", strconv.FormatBool(cfg.Env != "development"))
	if cfg.Env == "production" && !cfg.Auth.CookieSecure {
		errs = append(errs, errors.New("COOKIE_SECURE cannot be false in production"))
	}

	cfg.CoreBanking.Timeout = p.duration("CORE_BANKING_TIMEOUT", "3s", false)
	cfg.CoreBanking.BreakerThreshold = p.intRange("CORE_BANKING_BREAKER_THRESHOLD", "5", 1, 100)
	cfg.CoreBanking.BreakerOpenFor = p.duration("CORE_BANKING_BREAKER_OPEN_FOR", "30s", false)
	cfg.CoreBanking.MockLatency = p.duration("MOCK_CORE_BANKING_LATENCY", "0s", true)
	cfg.CoreBanking.MockErrorRate = p.ratio("MOCK_CORE_BANKING_ERROR_RATE", "0")

	cfg.RateLimits.LoginPerMinute = p.intRange("RATE_LIMIT_LOGIN_PER_MINUTE", "10", 1, 1000)
	cfg.RateLimits.WritesPerHour = p.intRange("RATE_LIMIT_WRITES_PER_HOUR", "30", 1, 10000)

	cfg.Notify.PollInterval = p.duration("NOTIFY_POLL_INTERVAL", "2s", false)
	cfg.Notify.MaxAttempts = p.intRange("NOTIFY_MAX_ATTEMPTS", "8", 1, 50)

	cfg.MetricsToken = os.Getenv("METRICS_TOKEN")
	if cfg.MetricsToken != "" && len(cfg.MetricsToken) < 32 {
		errs = append(errs, errors.New("METRICS_TOKEN must be at least 32 characters (or empty to disable /metrics)"))
	}

	cfg.SeedDemoData = p.boolean("SEED_DEMO_DATA", "false")
	cfg.EnableDocs = p.boolean("ENABLE_API_DOCS", "true")
	cfg.TrustedProxyHops = p.intRange("TRUSTED_PROXY_HOPS", "0", 0, 5)

	return cfg, errors.Join(errs...)
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

type parser struct{ errs *[]error }

func (p parser) fail(format string, args ...any) {
	*p.errs = append(*p.errs, fmt.Errorf(format, args...))
}

func (p parser) duration(key, fallback string, allowZero bool) time.Duration {
	d, err := time.ParseDuration(getEnv(key, fallback))
	if err != nil || d < 0 || (d == 0 && !allowZero) {
		p.fail("%s must be a positive duration like 15s", key)
	}
	return d
}

func (p parser) intRange(key, fallback string, lo, hi int) int {
	n, err := strconv.Atoi(getEnv(key, fallback))
	if err != nil || n < lo || n > hi {
		p.fail("%s must be a number between %d and %d", key, lo, hi)
	}
	return n
}

func (p parser) int32Range(key, fallback string, lo, hi int32) int32 {
	n, err := strconv.ParseInt(getEnv(key, fallback), 10, 32)
	if err != nil || n < int64(lo) || n > int64(hi) {
		p.fail("%s must be a number between %d and %d", key, lo, hi)
		return 0
	}
	return int32(n)
}

func (p parser) boolean(key, fallback string) bool {
	b, err := strconv.ParseBool(getEnv(key, fallback))
	if err != nil {
		p.fail("%s must be true or false", key)
	}
	return b
}

func (p parser) ratio(key, fallback string) float64 {
	f, err := strconv.ParseFloat(getEnv(key, fallback), 64)
	if err != nil || f < 0 || f > 1 {
		p.fail("%s must be a number between 0 and 1", key)
	}
	return f
}
