package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/outbox"
)

var errNoRows = errors.New("not found")

type credentials struct {
	User
	PasswordHash string
	Locked       bool
}

type newSession struct {
	TokenHash []byte
	UserID    string
	CSRFToken string
	ExpiresAt time.Time
}

type auditEvent struct {
	Event   Event
	Email   string
	UserID  string
	ActorID string
	IP      string
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) findCredentials(ctx context.Context, email string) (credentials, error) {
	var c credentials
	err := r.db.QueryRow(ctx, `
		SELECT id, email, full_name, role, password_hash, locked_at IS NOT NULL
		FROM users WHERE email = $1`, email).
		Scan(&c.ID, &c.Email, &c.FullName, &c.Role, &c.PasswordHash, &c.Locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return credentials{}, errNoRows
	}
	if err != nil {
		return credentials{}, fmt.Errorf("find credentials: %w", err)
	}
	return c, nil
}

// claimAttempt atomically counts this attempt before the password is checked.
// Concurrent requests are serialised on the row, so no more than the allowed
// number of guesses can ever be evaluated, however many arrive at once.
func (r *Repository) claimAttempt(ctx context.Context, email string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		INSERT INTO login_failures (email, failed_count, last_failed_at) VALUES ($1, 1, now())
		ON CONFLICT (email) DO UPDATE
			SET failed_count = login_failures.failed_count + 1, last_failed_at = now()
		RETURNING failed_count`, email).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("claim attempt: %w", err)
	}
	return n, nil
}

func (r *Repository) clearAttempts(ctx context.Context, email string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM login_failures WHERE email = $1`, email)
	if err != nil {
		return fmt.Errorf("clear attempts: %w", err)
	}
	return nil
}

// lockUser locks the account and revokes every session it holds, in one transaction.
func (r *Repository) lockUser(ctx context.Context, userID, reason string) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE users SET locked_at = now(), lock_reason = $2
			WHERE id = $1 AND locked_at IS NULL`, userID, reason)
		if err != nil {
			return fmt.Errorf("lock user: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		return outbox.Enqueue(ctx, tx, outbox.TopicAccountLocked, map[string]string{"userId": userID})
	})
}

func (r *Repository) unlockUser(ctx context.Context, userID string) (string, error) {
	var email string
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			UPDATE users SET locked_at = NULL, lock_reason = NULL
			WHERE id = $1 AND locked_at IS NOT NULL
			RETURNING email`, userID).Scan(&email)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoRows
		}
		if err != nil {
			return fmt.Errorf("unlock user: %w", err)
		}
		_, err = tx.Exec(ctx, `DELETE FROM login_failures WHERE email = $1`, email)
		return err
	})
	return email, err
}

func (r *Repository) lockedAccounts(ctx context.Context) ([]LockedAccount, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, email, full_name, locked_at, coalesce(lock_reason, '')
		FROM users WHERE locked_at IS NOT NULL ORDER BY locked_at DESC LIMIT 200`)
	if err != nil {
		return nil, fmt.Errorf("list locked accounts: %w", err)
	}
	accounts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (LockedAccount, error) {
		var a LockedAccount
		err := row.Scan(&a.ID, &a.Email, &a.FullName, &a.LockedAt, &a.LockReason)
		return a, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan locked accounts: %w", err)
	}
	return accounts, nil
}

func (r *Repository) createSession(ctx context.Context, s newSession) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, csrf_token, expires_at) VALUES ($1, $2, $3, $4)`,
		s.TokenHash, s.UserID, s.CSRFToken, s.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// touchSession validates and slides the idle window in one round trip.
// Locked users are rejected here too, so a lock takes effect immediately.
func (r *Repository) touchSession(ctx context.Context, tokenHash []byte, idle time.Duration) (Principal, error) {
	p := Principal{SessionHash: tokenHash}
	err := r.db.QueryRow(ctx, `
		UPDATE sessions s SET last_seen_at = now()
		FROM users u
		WHERE s.token_hash = $1
		  AND u.id = s.user_id
		  AND u.locked_at IS NULL
		  AND s.expires_at > now()
		  AND s.last_seen_at > now() - make_interval(secs => $2)
		RETURNING s.user_id, u.role, s.csrf_token`, tokenHash, idle.Seconds()).
		Scan(&p.UserID, &p.Role, &p.CSRFToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, errNoRows
	}
	if err != nil {
		return Principal{}, fmt.Errorf("touch session: %w", err)
	}
	return p, nil
}

func (r *Repository) deleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *Repository) deleteExpiredSessions(ctx context.Context, idle time.Duration) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM sessions
		WHERE expires_at <= now() OR last_seen_at <= now() - make_interval(secs => $1)`, idle.Seconds())
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *Repository) getUser(ctx context.Context, id string) (User, error) {
	var u User
	err := r.db.QueryRow(ctx, `SELECT id, email, full_name, role FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Email, &u.FullName, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, errNoRows
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

func (r *Repository) recordEvent(ctx context.Context, e auditEvent) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO auth_events (event, email, user_id, actor_id, ip)
		VALUES ($1, nullif($2, ''), nullif($3, '')::uuid, nullif($4, '')::uuid, nullif($5, ''))`,
		e.Event, e.Email, e.UserID, e.ActorID, e.IP)
	if err != nil {
		return fmt.Errorf("record auth event: %w", err)
	}
	return nil
}
