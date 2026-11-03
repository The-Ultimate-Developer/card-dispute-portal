package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/admin"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/auth"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/buildinfo"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/config"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/corebanking"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/disputes"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/httpapi"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/notify"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/database"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/httpx"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/logger"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/metrics"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/ratelimit"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/seed"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/server"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/transactions"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// run returns an error instead of exiting, so every defer always runs.
// This is the only place that knows how the pieces fit together.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.LogLevel).With("service", "dispute-api", "env", cfg.Env, "version", buildinfo.Version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.DB.RunMigrations {
		if err := database.Migrate(cfg.DB.URL, log); err != nil {
			return err
		}
	}

	pool, err := database.Connect(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.Info("database connected", "max_conns", cfg.DB.MaxConns)

	if cfg.SeedDemoData {
		if err := seed.Run(ctx, pool, log); err != nil {
			return fmt.Errorf("seed demo data: %w", err)
		}
	}

	m := metrics.New()
	registerPoolMetrics(m, pool)

	clientIP := func(r *http.Request) string { return httpx.ClientIP(r, cfg.TrustedProxyHops) }

	authSvc := auth.NewService(auth.NewRepository(pool), auth.Config{
		MaxFailedAttempts: cfg.Auth.MaxFailedAttempts,
		IdleTimeout:       cfg.Auth.IdleTimeout,
		AbsoluteTimeout:   cfg.Auth.AbsoluteTimeout,
	}, log, m)

	bank := corebanking.WithCircuitBreaker(
		corebanking.WithTimeout(
			corebanking.NewMock(pool, corebanking.Faults{
				Latency:   cfg.CoreBanking.MockLatency,
				ErrorRate: cfg.CoreBanking.MockErrorRate,
			}),
			cfg.CoreBanking.Timeout,
		),
		corebanking.BreakerConfig{FailureThreshold: cfg.CoreBanking.BreakerThreshold, OpenFor: cfg.CoreBanking.BreakerOpenFor},
		m,
	)

	disputeSvc := disputes.NewService(disputes.NewRepository(pool), bank, m)
	transactionSvc := transactions.NewService(bank, disputeSvc)

	notifier := notify.NewWorker(pool, notify.LogSender{Log: log}, m, log, notify.Config{
		PollInterval: cfg.Notify.PollInterval,
		BatchSize:    20,
		MaxAttempts:  cfg.Notify.MaxAttempts,
		Lease:        2 * time.Minute,
	})

	routes := httpapi.Routes(httpapi.Deps{
		Log:          log,
		Auth:         auth.NewHandler(authSvc, auth.HandlerConfig{CookieSecure: cfg.Auth.CookieSecure, ClientIP: clientIP}, log),
		Transactions: transactions.NewHandler(transactionSvc, log),
		Disputes:     disputes.NewHandler(disputeSvc, log),
		Admin:        admin.NewHandler(admin.NewService(pool), log),
		LoginLimiter: ratelimit.New(cfg.RateLimits.LoginPerMinute, time.Minute),
		WriteLimiter: ratelimit.New(cfg.RateLimits.WritesPerHour, time.Hour),
		ClientIP:     clientIP,
		Metrics:      m.Handler(cfg.MetricsToken),
		EnableDocs:   cfg.EnableDocs,
	})

	srv := server.New(server.Options{
		Port:     cfg.Port,
		Log:      log,
		DB:       pool,
		Routes:   routes,
		Observer: m,
		Workers: []server.Worker{
			{Name: "notifications", Run: notifier.Run},
			{Name: "session-janitor", Run: func(ctx context.Context) { authSvc.RunJanitor(ctx, 10*time.Minute) }},
		},
	})
	return srv.Run(ctx, cfg.ShutdownTimeout)
}

func registerPoolMetrics(m *metrics.Metrics, pool *pgxpool.Pool) {
	gauge := func(name, help string, f func(*pgxpool.Stat) float64) {
		m.Register(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: name, Help: help},
			func() float64 { return f(pool.Stat()) }))
	}
	gauge("db_pool_acquired_connections", "Connections currently in use.", func(s *pgxpool.Stat) float64 { return float64(s.AcquiredConns()) })
	gauge("db_pool_idle_connections", "Idle connections.", func(s *pgxpool.Stat) float64 { return float64(s.IdleConns()) })
	gauge("db_pool_max_connections", "Pool size limit.", func(s *pgxpool.Stat) float64 { return float64(s.MaxConns()) })
	m.Register(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: "db_pool_empty_acquire_total", Help: "Times a request had to wait because every connection was busy.",
	}, func() float64 { return float64(pool.Stat().EmptyAcquireCount()) }))
}
