// Package seed loads demo users, cards, transactions and a few disputes.
// It runs only when SEED_DEMO_DATA=true and is idempotent per user, so adding
// a new demo account later does not duplicate anything that already exists.
package seed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/auth"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/corebanking"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/disputes"
)

const DemoPassword = "Demo@2026!" //nolint:gosec // published demo credential, documented in the README

type user struct {
	email, name string
	role        auth.Role
	cards       []card
}

type card struct {
	brand, last4 string
	txCount      int
}

var demoUsers = []user{
	{"thandi@demo.capitec.local", "Thandi Mokoena", auth.RoleCustomer, []card{{"VISA", "4821", 24}, {"MASTERCARD", "1934", 12}}},
	{"sipho@demo.capitec.local", "Sipho Dlamini", auth.RoleCustomer, []card{{"VISA", "7710", 15}}},
	{"agent@demo.capitec.local", "Ayesha Patel", auth.RoleAgent, nil},
	{"support@demo.capitec.local", "Johan van Wyk", auth.RoleSupport, nil},
	{"admin@demo.capitec.local", "Nomsa Khumalo", auth.RoleAdmin, nil},
}

type merchant struct {
	name, category string
	minRand        int
	maxRand        int
}

var merchants = []merchant{
	{"Checkers Sixty60", "Groceries", 150, 1800},
	{"Woolworths Food", "Groceries", 120, 1500},
	{"Pick n Pay", "Groceries", 80, 1400},
	{"Uber", "Transport", 45, 320},
	{"Bolt", "Transport", 40, 280},
	{"Takealot.com", "Online shopping", 199, 4999},
	{"Netflix", "Subscriptions", 199, 199},
	{"Spotify", "Subscriptions", 70, 70},
	{"Engen Garage", "Fuel", 400, 1200},
	{"Vida e Caffe", "Restaurants", 38, 120},
	{"Mr D Food", "Restaurants", 90, 450},
	{"Dis-Chem", "Health", 60, 900},
	{"Superbalist", "Clothing", 299, 2400},
	{"Vodacom Airtime", "Utilities", 29, 500},
}

func Run(ctx context.Context, db *pgxpool.Pool, log *slog.Logger) error {
	hash, err := auth.HashPassword(DemoPassword)
	if err != nil {
		return err
	}

	rng := rand.New(rand.NewPCG(2026, 10)) //nolint:gosec // deterministic demo data, not security
	now := time.Now().UTC()
	ids := map[string]string{}
	created := 0

	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		for _, u := range demoUsers {
			var id string
			err := tx.QueryRow(ctx, `
				INSERT INTO users (email, full_name, password_hash, role) VALUES ($1, $2, $3, $4)
				ON CONFLICT (email) DO NOTHING RETURNING id`, u.email, u.name, hash, u.role).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("insert user %s: %w", u.email, err)
			}
			ids[u.email] = id
			created++

			for _, c := range u.cards {
				if err := insertCard(ctx, tx, rng, now, id, c); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if created == 0 {
		log.Info("demo data already present, skipping seed")
		return nil
	}

	if customerID, ok := ids[demoUsers[0].email]; ok {
		if agentID, err := userID(ctx, db, "agent@demo.capitec.local"); err == nil {
			if err := seedDisputes(ctx, db, customerID, agentID); err != nil {
				return err
			}
		}
	}

	log.Info("demo data seeded", "new_users", created)
	return nil
}

func insertCard(ctx context.Context, tx pgx.Tx, rng *rand.Rand, now time.Time, customerID string, c card) error {
	var cardID string
	if err := tx.QueryRow(ctx, `INSERT INTO cards (customer_id, brand, last4) VALUES ($1, $2, $3) RETURNING id`,
		customerID, c.brand, c.last4).Scan(&cardID); err != nil {
		return fmt.Errorf("insert card: %w", err)
	}

	batch := &pgx.Batch{}
	for i := 0; i < c.txCount; i++ {
		m := merchants[rng.IntN(len(merchants))]
		amount := int64(m.minRand*100 + rng.IntN((m.maxRand-m.minRand)*100+1))
		occurred := now.Add(-time.Duration(rng.IntN(75*24*60)) * time.Minute)
		status := "POSTED"
		if i < 2 {
			status = "PENDING"
			occurred = now.Add(-time.Duration(rng.IntN(36*60)) * time.Minute)
		}
		batch.Queue(`
			INSERT INTO transactions (card_id, merchant, category, amount_cents, status, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6)`, cardID, m.name, m.category, amount, status, occurred)
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("insert transactions: %w", err)
	}
	return nil
}

// seedDisputes goes through the real dispute service, so demo data obeys the
// same rules as production data and gets a genuine timeline.
func seedDisputes(ctx context.Context, db *pgxpool.Pool, customerID, agentID string) error {
	rows, err := db.Query(ctx, `
		SELECT t.id FROM transactions t JOIN cards c ON c.id = t.card_id
		WHERE c.customer_id = $1 AND t.status = 'POSTED'
		ORDER BY t.occurred_at DESC LIMIT 3`, customerID)
	if err != nil {
		return fmt.Errorf("pick transactions: %w", err)
	}
	txIDs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(txIDs) < 3 {
		return fmt.Errorf("pick transactions: %w", err)
	}

	svc := disputes.NewService(disputes.NewRepository(db), corebanking.NewMock(db, corebanking.Faults{}), nil)
	customer := auth.Principal{UserID: customerID, Role: auth.RoleCustomer}
	agent := auth.Principal{UserID: agentID, Role: auth.RoleAgent}

	plans := []struct {
		reason      disputes.Reason
		description string
		steps       []disputes.ChangeStatusInput
	}{
		{disputes.ReasonDuplicate, "I was charged twice for the same order.", nil},
		{disputes.ReasonNotReceived, "The order never arrived and the merchant is not responding.",
			[]disputes.ChangeStatusInput{{Status: disputes.StatusUnderReview, Version: 1, Note: "Requested proof of delivery from the merchant."}}},
		{disputes.ReasonIncorrectAmount, "I was charged more than the price shown at checkout.",
			[]disputes.ChangeStatusInput{
				{Status: disputes.StatusUnderReview, Version: 1, Note: "Comparing receipt with the settled amount."},
				{Status: disputes.StatusResolved, Version: 2, Note: "Difference refunded to the customer's card."},
			}},
	}

	for i, plan := range plans {
		d, _, err := svc.Create(ctx, customer, disputes.CreateInput{
			TransactionID: txIDs[i], Reason: plan.reason, Description: plan.description,
			IdempotencyKey: fmt.Sprintf("seed-dispute-%d", i),
		})
		if err != nil {
			return fmt.Errorf("seed dispute: %w", err)
		}
		for _, step := range plan.steps {
			if _, err := svc.ChangeStatus(ctx, agent, d.ID, step); err != nil {
				return fmt.Errorf("seed dispute status: %w", err)
			}
		}
	}
	return nil
}

func userID(ctx context.Context, db *pgxpool.Pool, email string) (string, error) {
	var id string
	err := db.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&id)
	return id, err
}
