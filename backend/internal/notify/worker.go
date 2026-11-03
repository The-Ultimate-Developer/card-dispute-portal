// Package notify delivers outbox messages as customer emails.
//
// Delivery is at-least-once: a message is claimed with a lease, so if this
// process dies mid-send another instance retries it after the lease expires.
// Each email carries a stable ID ("outbox-<id>") that a real provider uses to
// drop duplicates.
package notify

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Email struct {
	ID      string
	To      string
	Subject string
	Body    string
}

type Sender interface {
	Send(ctx context.Context, e Email) error
}

type Observer interface {
	ObserveDelivery(topic, outcome string)
}

type Config struct {
	PollInterval time.Duration
	BatchSize    int
	MaxAttempts  int
	Lease        time.Duration
}

// errPermanent marks failures that retrying cannot fix, such as a payload
// pointing at a record that no longer exists.
var errPermanent = errors.New("permanent failure")

type message struct {
	ID       int64
	Topic    string
	Payload  json.RawMessage
	Attempts int
}

type Worker struct {
	db     *pgxpool.Pool
	sender Sender
	obs    Observer
	log    *slog.Logger
	cfg    Config
}

func NewWorker(db *pgxpool.Pool, sender Sender, obs Observer, log *slog.Logger, cfg Config) *Worker {
	return &Worker{db: db, sender: sender, obs: obs, log: log, cfg: cfg}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()
	for {
		for {
			n, err := w.RunOnce(ctx)
			if err != nil && ctx.Err() == nil {
				w.log.ErrorContext(ctx, "outbox poll failed", "error", err)
			}
			if n < w.cfg.BatchSize || ctx.Err() != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce claims and delivers one batch, returning how many were claimed.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	batch, err := w.claim(ctx)
	if err != nil {
		return 0, err
	}
	for _, m := range batch {
		w.deliver(ctx, m)
	}
	return len(batch), nil
}

// claim leases due messages. SKIP LOCKED lets several instances poll the
// same table without blocking each other or taking the same message.
func (w *Worker) claim(ctx context.Context) ([]message, error) {
	rows, err := w.db.Query(ctx, `
		UPDATE outbox SET attempts = attempts + 1, available_at = now() + make_interval(secs => $2)
		WHERE id IN (
			SELECT id FROM outbox
			WHERE status = 'PENDING' AND available_at <= now()
			ORDER BY id
			LIMIT $1
			FOR UPDATE SKIP LOCKED)
		RETURNING id, topic, payload, attempts`, w.cfg.BatchSize, w.cfg.Lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claim outbox: %w", err)
	}
	batch, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (message, error) {
		var m message
		err := row.Scan(&m.ID, &m.Topic, &m.Payload, &m.Attempts)
		return m, err
	})
	if err != nil {
		return nil, err
	}
	// RETURNING does not preserve order. Sort so a customer never gets
	// "resolved" before "received".
	slices.SortFunc(batch, func(a, b message) int { return cmp.Compare(a.ID, b.ID) })
	return batch, nil
}

func (w *Worker) deliver(ctx context.Context, m message) {
	email, err := render(ctx, w.db, m)
	if err == nil {
		email.ID = fmt.Sprintf("outbox-%d", m.ID)
		err = w.sender.Send(ctx, email)
	}

	switch {
	case err == nil:
		w.settle(ctx, m, `UPDATE outbox SET status = 'SENT', sent_at = now(), last_error = NULL WHERE id = $1`)
		w.obs.ObserveDelivery(m.Topic, "sent")
	case errors.Is(err, errPermanent) || m.Attempts >= w.cfg.MaxAttempts:
		w.settle(ctx, m, `UPDATE outbox SET status = 'DEAD', last_error = $2 WHERE id = $1`, truncate(err.Error()))
		w.obs.ObserveDelivery(m.Topic, "dead")
		w.log.ErrorContext(ctx, "notification moved to dead letter",
			"outbox_id", m.ID, "topic", m.Topic, "attempts", m.Attempts, "error", err)
	default:
		w.settle(ctx, m, `UPDATE outbox SET available_at = now() + make_interval(secs => $3), last_error = $2 WHERE id = $1`,
			truncate(err.Error()), backoff(m.Attempts).Seconds())
		w.obs.ObserveDelivery(m.Topic, "retry")
		w.log.WarnContext(ctx, "notification delivery failed, will retry",
			"outbox_id", m.ID, "topic", m.Topic, "attempts", m.Attempts, "error", err)
	}
}

func (w *Worker) settle(ctx context.Context, m message, sql string, args ...any) {
	if _, err := w.db.Exec(ctx, sql, append([]any{m.ID}, args...)...); err != nil {
		w.log.ErrorContext(ctx, "outbox update failed; message will be retried after its lease",
			"outbox_id", m.ID, "error", err)
	}
}

// backoff grows exponentially (2s, 4s, 8s ...) and caps at 10 minutes.
func backoff(attempt int) time.Duration {
	d := 2 * time.Second << min(attempt-1, 9)
	return min(d, 10*time.Minute)
}

func truncate(s string) string {
	if len(s) > 500 {
		return s[:500]
	}
	return s
}

// LogSender stands in for an email provider. It logs a masked recipient,
// never the full address, because logs are not a place for personal data.
type LogSender struct {
	Log *slog.Logger
}

func (s LogSender) Send(ctx context.Context, e Email) error {
	s.Log.InfoContext(ctx, "email sent (simulated)", "email_id", e.ID, "to", maskEmail(e.To), "subject", e.Subject)
	return nil
}

func maskEmail(addr string) string {
	local, domain, ok := strings.Cut(addr, "@")
	if !ok || local == "" {
		return "***"
	}
	return local[:1] + "***@" + domain
}
