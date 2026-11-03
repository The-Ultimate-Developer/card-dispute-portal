// Package outbox records messages inside the caller's database transaction.
// If the business change commits, the message is guaranteed to exist; if it
// rolls back, the message never existed. Delivery is the notify worker's job.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	TopicDisputeCreated       = "dispute.created"
	TopicDisputeStatusChanged = "dispute.status_changed"
	TopicAccountLocked        = "account.locked"
)

func Enqueue(ctx context.Context, tx pgx.Tx, topic string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode outbox payload: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox (topic, payload) VALUES ($1, $2)`, topic, body); err != nil {
		return fmt.Errorf("enqueue %s: %w", topic, err)
	}
	return nil
}
