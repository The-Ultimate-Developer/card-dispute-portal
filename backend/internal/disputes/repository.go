package disputes

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/corebanking"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/cursor"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/outbox"
)

const (
	columns = `id, customer_id, transaction_id, merchant, amount_cents, currency, card_last4,
		transaction_occurred_at, reason, description, status, version, created_at, updated_at`
	activeDisputeIndex = "disputes_one_active_per_transaction"
	uniqueViolation    = "23505"
)

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type createParams struct {
	CustomerID     string
	IdempotencyKey string
	RequestHash    []byte
	Transaction    corebanking.Transaction
	Reason         Reason
	Description    string
}

// create is idempotent per (customer, key). A retry with the same key and body
// returns the original dispute; the same key with a different body is refused.
// A concurrent duplicate blocks on the key's primary key until the first
// transaction finishes, then sees its result.
func (r *Repository) create(ctx context.Context, p createParams) (Dispute, bool, error) {
	var d Dispute
	created := false

	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO idempotency_keys (user_id, key, request_hash) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, p.CustomerID, p.IdempotencyKey, p.RequestHash)
		if err != nil {
			return fmt.Errorf("claim idempotency key: %w", err)
		}

		if tag.RowsAffected() == 0 {
			var hash []byte
			var existingID *string
			if err := tx.QueryRow(ctx, `
				SELECT request_hash, dispute_id FROM idempotency_keys WHERE user_id = $1 AND key = $2`,
				p.CustomerID, p.IdempotencyKey).Scan(&hash, &existingID); err != nil {
				return fmt.Errorf("read idempotency key: %w", err)
			}
			if !bytes.Equal(hash, p.RequestHash) || existingID == nil {
				return ErrIdempotencyKeyReused
			}
			d, err = getDispute(ctx, tx, *existingID, &p.CustomerID)
			return err
		}

		t := p.Transaction
		row := tx.QueryRow(ctx, `
			INSERT INTO disputes (customer_id, transaction_id, merchant, amount_cents, currency,
				card_last4, transaction_occurred_at, reason, description, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING `+columns,
			p.CustomerID, t.ID, t.Merchant, t.AmountCents, t.Currency, t.CardLast4, t.OccurredAt,
			p.Reason, p.Description, StatusSubmitted)
		if d, err = scanDispute(row); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == activeDisputeIndex {
				return ErrActiveDisputeExists
			}
			return fmt.Errorf("insert dispute: %w", err)
		}

		if err := insertEvent(ctx, tx, d.ID, nil, StatusSubmitted, p.CustomerID, "CUSTOMER", "Dispute submitted"); err != nil {
			return err
		}
		if err := outbox.Enqueue(ctx, tx, outbox.TopicDisputeCreated, map[string]string{"disputeId": d.ID}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE idempotency_keys SET dispute_id = $3 WHERE user_id = $1 AND key = $2`,
			p.CustomerID, p.IdempotencyKey, d.ID); err != nil {
			return fmt.Errorf("bind idempotency key: %w", err)
		}
		created = true
		return nil
	})
	return d, created, err
}

type transitionParams struct {
	ID              string
	ExpectedVersion int
	To              Status
	ActorID         string
	ActorRole       string
	Note            string
}

// transition takes a row lock, then checks the caller's version (optimistic
// concurrency for the client) and the state machine before writing.
func (r *Repository) transition(ctx context.Context, p transitionParams) (DisputeDetail, error) {
	var detail DisputeDetail
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var current Status
		var version int
		err := tx.QueryRow(ctx, `SELECT status, version FROM disputes WHERE id = $1 FOR UPDATE`, p.ID).
			Scan(&current, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock dispute: %w", err)
		}
		if version != p.ExpectedVersion {
			return ErrStaleVersion
		}
		if !current.CanTransitionTo(p.To) {
			return invalidTransition(current, p.To)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE disputes SET status = $2, version = version + 1, updated_at = now() WHERE id = $1`,
			p.ID, p.To); err != nil {
			return fmt.Errorf("update dispute: %w", err)
		}
		if err := insertEvent(ctx, tx, p.ID, &current, p.To, p.ActorID, p.ActorRole, p.Note); err != nil {
			return err
		}
		if err := outbox.Enqueue(ctx, tx, outbox.TopicDisputeStatusChanged,
			map[string]string{"disputeId": p.ID, "toStatus": string(p.To), "note": p.Note}); err != nil {
			return err
		}

		detail, err = getDetail(ctx, tx, p.ID, nil)
		return err
	})
	return detail, err
}

func (r *Repository) detail(ctx context.Context, id string, customerID *string) (DisputeDetail, error) {
	return getDetail(ctx, r.db, id, customerID)
}

type listParams struct {
	CustomerID *string
	Status     *Status
	After      cursor.Position
	Limit      int
}

func (r *Repository) list(ctx context.Context, p listParams) (Page, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+columns+` FROM disputes
		WHERE ($1::uuid IS NULL OR customer_id = $1)
		  AND ($2::text IS NULL OR status = $2)
		  AND ($3::timestamptz IS NULL OR (created_at, id) < ($3, $4::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $5`, p.CustomerID, p.Status, p.After.At, p.After.ID, p.Limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list disputes: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Dispute, error) { return scanDispute(row) })
	if err != nil {
		return Page{}, fmt.Errorf("scan disputes: %w", err)
	}

	page := Page{Items: items}
	if len(items) > p.Limit {
		page.Items = items[:p.Limit]
		last := page.Items[p.Limit-1]
		page.NextCursor = cursor.Encode(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (r *Repository) summariesByTransaction(ctx context.Context, customerID string, transactionIDs []string) (map[string]Summary, error) {
	out := make(map[string]Summary, len(transactionIDs))
	if len(transactionIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ON (transaction_id) transaction_id, id, status
		FROM disputes
		WHERE customer_id = $1 AND transaction_id = ANY($2::uuid[])
		ORDER BY transaction_id, created_at DESC`, customerID, transactionIDs)
	if err != nil {
		return nil, fmt.Errorf("dispute summaries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var txID string
		var s Summary
		if err := rows.Scan(&txID, &s.ID, &s.Status); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		out[txID] = s
	}
	return out, rows.Err()
}

func (r *Repository) customers(ctx context.Context, ids []string) (map[string]Customer, error) {
	rows, err := r.db.Query(ctx, `SELECT id, full_name, email FROM users WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("load customers: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Customer, error) {
		var c Customer
		err := row.Scan(&c.ID, &c.FullName, &c.Email)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan customers: %w", err)
	}
	out := make(map[string]Customer, len(list))
	for _, c := range list {
		out[c.ID] = c
	}
	return out, nil
}

func getDispute(ctx context.Context, q querier, id string, customerID *string) (Dispute, error) {
	d, err := scanDispute(q.QueryRow(ctx, `
		SELECT `+columns+` FROM disputes WHERE id = $1 AND ($2::uuid IS NULL OR customer_id = $2)`,
		id, customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Dispute{}, ErrNotFound
	}
	if err != nil {
		return Dispute{}, fmt.Errorf("get dispute: %w", err)
	}
	return d, nil
}

func getDetail(ctx context.Context, q querier, id string, customerID *string) (DisputeDetail, error) {
	d, err := getDispute(ctx, q, id, customerID)
	if err != nil {
		return DisputeDetail{}, err
	}
	rows, err := q.Query(ctx, `
		SELECT id, from_status, to_status, actor_role, note, created_at
		FROM dispute_events WHERE dispute_id = $1 ORDER BY id`, id)
	if err != nil {
		return DisputeDetail{}, fmt.Errorf("list events: %w", err)
	}
	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) {
		var e Event
		err := row.Scan(&e.ID, &e.FromStatus, &e.ToStatus, &e.ActorRole, &e.Note, &e.CreatedAt)
		return e, err
	})
	if err != nil {
		return DisputeDetail{}, fmt.Errorf("scan events: %w", err)
	}
	return DisputeDetail{Dispute: d, Timeline: events}, nil
}

func insertEvent(ctx context.Context, tx pgx.Tx, disputeID string, from *Status, to Status, actorID, actorRole, note string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO dispute_events (dispute_id, from_status, to_status, actor_id, actor_role, note)
		VALUES ($1, $2, $3, $4, $5, $6)`, disputeID, from, to, actorID, actorRole, note)
	if err != nil {
		return fmt.Errorf("insert dispute event: %w", err)
	}
	return nil
}

func scanDispute(row pgx.Row) (Dispute, error) {
	var d Dispute
	err := row.Scan(&d.ID, &d.CustomerID, &d.TransactionID, &d.Merchant, &d.AmountCents, &d.Currency, &d.CardLast4,
		&d.TransactionOccurredAt, &d.Reason, &d.Description, &d.Status, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}
