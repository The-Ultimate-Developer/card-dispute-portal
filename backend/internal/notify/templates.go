package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/outbox"
)

type disputePayload struct {
	DisputeID string `json:"disputeId"`
	ToStatus  string `json:"toStatus"`
	Note      string `json:"note"`
}

type accountPayload struct {
	UserID string `json:"userId"`
}

var statusWording = map[string]string{
	"SUBMITTED":    "received",
	"UNDER_REVIEW": "under review",
	"RESOLVED":     "resolved",
	"REJECTED":     "declined",
}

// render builds an email from the message payload plus current read-only facts.
// What changed (status, note) comes from the payload, so a later change cannot
// rewrite an earlier email; who and what (name, merchant, amount) come from
// immutable rows.
func render(ctx context.Context, db *pgxpool.Pool, m message) (Email, error) {
	switch m.Topic {
	case outbox.TopicDisputeCreated, outbox.TopicDisputeStatusChanged:
		var p disputePayload
		if err := json.Unmarshal(m.Payload, &p); err != nil || p.DisputeID == "" {
			return Email{}, fmt.Errorf("%w: bad payload", errPermanent)
		}
		return renderDispute(ctx, db, m.Topic, p)
	case outbox.TopicAccountLocked:
		var p accountPayload
		if err := json.Unmarshal(m.Payload, &p); err != nil || p.UserID == "" {
			return Email{}, fmt.Errorf("%w: bad payload", errPermanent)
		}
		return renderLocked(ctx, db, p)
	default:
		return Email{}, fmt.Errorf("%w: unknown topic %q", errPermanent, m.Topic)
	}
}

func renderDispute(ctx context.Context, db *pgxpool.Pool, topic string, p disputePayload) (Email, error) {
	var to, name, merchant, currency string
	var amount int64
	var occurred time.Time
	err := db.QueryRow(ctx, `
		SELECT u.email, u.full_name, d.merchant, d.amount_cents, d.currency, d.transaction_occurred_at
		FROM disputes d JOIN users u ON u.id = d.customer_id
		WHERE d.id = $1`, p.DisputeID).Scan(&to, &name, &merchant, &amount, &currency, &occurred)
	if errors.Is(err, pgx.ErrNoRows) {
		return Email{}, fmt.Errorf("%w: dispute %s not found", errPermanent, p.DisputeID)
	}
	if err != nil {
		return Email{}, err
	}

	ref := strings.ToUpper(p.DisputeID[:8])
	what := fmt.Sprintf("your %s payment to %s on %s", money(amount, currency), merchant, occurred.Format("2 January 2006"))

	var b strings.Builder
	fmt.Fprintf(&b, "Hi %s,\n\n", firstName(name))
	var subject string
	if topic == outbox.TopicDisputeCreated {
		subject = "We've received your dispute (ref " + ref + ")"
		fmt.Fprintf(&b, "We've received your dispute about %s.\n\n", what)
		b.WriteString("We'll review it and keep you updated at every step. You can follow its progress in the app at any time.\n")
	} else {
		status := statusWording[p.ToStatus]
		subject = "Your dispute is " + status + " (ref " + ref + ")"
		fmt.Fprintf(&b, "Your dispute about %s is now %s.\n", what, status)
		if p.Note != "" {
			fmt.Fprintf(&b, "\nUpdate from our team: %s\n", p.Note)
		}
	}
	fmt.Fprintf(&b, "\nReference: %s\n\nYou never need to share your PIN or password to resolve a dispute.\n", ref)
	return Email{To: to, Subject: subject, Body: b.String()}, nil
}

func renderLocked(ctx context.Context, db *pgxpool.Pool, p accountPayload) (Email, error) {
	var to, name string
	err := db.QueryRow(ctx, `SELECT email, full_name FROM users WHERE id = $1`, p.UserID).Scan(&to, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return Email{}, fmt.Errorf("%w: user %s not found", errPermanent, p.UserID)
	}
	if err != nil {
		return Email{}, err
	}
	body := fmt.Sprintf("Hi %s,\n\n"+
		"We blocked sign-in to your account after several incorrect password attempts.\n\n"+
		"If this was you, please contact support to unlock your account.\n"+
		"If it wasn't you, contact support urgently. Someone may be trying to access your account.\n",
		firstName(name))
	return Email{To: to, Subject: "Sign-in to your account has been blocked", Body: body}, nil
}

func firstName(full string) string {
	if f := strings.Fields(full); len(f) > 0 {
		return f[0]
	}
	return "there"
}

// money formats minor units as e.g. "R1 234.56" (South African grouping).
func money(cents int64, currency string) string {
	symbol := currency + " "
	if currency == "ZAR" {
		symbol = "R"
	}
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	whole := strconv.FormatInt(cents/100, 10)
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + " " + whole[i:]
	}
	return fmt.Sprintf("%s%s%s.%02d", sign, symbol, whole, cents%100)
}
