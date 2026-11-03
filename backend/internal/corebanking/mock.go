package corebanking

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/cursor"
)

// Faults lets the mock behave like a real dependency on a bad day, so the
// timeout and error paths can be demonstrated, not just claimed.
type Faults struct {
	Latency   time.Duration
	ErrorRate float64
}

type Mock struct {
	db     *pgxpool.Pool
	faults Faults
}

func NewMock(db *pgxpool.Pool, faults Faults) *Mock {
	return &Mock{db: db, faults: faults}
}

func (m *Mock) inject(ctx context.Context) error {
	if m.faults.Latency > 0 {
		select {
		case <-time.After(m.faults.Latency):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if m.faults.ErrorRate > 0 && rand.Float64() < m.faults.ErrorRate { //nolint:gosec // fault injection, not security
		return fmt.Errorf("%w: injected fault", ErrUnavailable)
	}
	return nil
}

const selectTransaction = `
	SELECT t.id, t.card_id, c.brand, c.last4, t.merchant, t.category,
	       t.amount_cents, t.currency, t.status, t.occurred_at
	FROM transactions t
	JOIN cards c ON c.id = t.card_id`

func (m *Mock) ListTransactions(ctx context.Context, customerID, after string, limit int) (Page, error) {
	if err := m.inject(ctx); err != nil {
		return Page{}, err
	}

	pos, err := cursor.Decode(after)
	if err != nil {
		return Page{}, ErrInvalidCursor
	}

	rows, err := m.db.Query(ctx, selectTransaction+`
		WHERE c.customer_id = $1
		  AND ($2::timestamptz IS NULL OR (t.occurred_at, t.id) < ($2, $3::uuid))
		ORDER BY t.occurred_at DESC, t.id DESC
		LIMIT $4`, customerID, pos.At, pos.ID, limit+1)
	if err != nil {
		return Page{}, err
	}
	items, err := pgx.CollectRows(rows, scanTransaction)
	if err != nil {
		return Page{}, err
	}

	page := Page{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[limit-1]
		page.NextCursor = cursor.Encode(last.OccurredAt, last.ID)
	}
	return page, nil
}

func (m *Mock) GetTransaction(ctx context.Context, customerID, transactionID string) (Transaction, error) {
	if err := m.inject(ctx); err != nil {
		return Transaction{}, err
	}
	rows, err := m.db.Query(ctx, selectTransaction+` WHERE t.id = $1 AND c.customer_id = $2`, transactionID, customerID)
	if err != nil {
		return Transaction{}, err
	}
	tx, err := pgx.CollectExactlyOneRow(rows, scanTransaction)
	if errors.Is(err, pgx.ErrNoRows) {
		return Transaction{}, ErrNotFound
	}
	return tx, err
}

func scanTransaction(row pgx.CollectableRow) (Transaction, error) {
	var t Transaction
	err := row.Scan(&t.ID, &t.CardID, &t.CardBrand, &t.CardLast4, &t.Merchant, &t.Category,
		&t.AmountCents, &t.Currency, &t.Status, &t.OccurredAt)
	return t, err
}
