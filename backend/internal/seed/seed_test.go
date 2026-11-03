package seed

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/testdb"
)

func TestSeedIsIdempotent(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	for i := 0; i < 2; i++ {
		if err := Run(ctx, db, log); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	for table, want := range map[string]int{"users": 5, "cards": 3, "transactions": 51, "disputes": 3, "dispute_events": 6} {
		var n int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("%s: got %d, want %d", table, n, want)
		}
	}
}

func TestSeedAddsNewUsersToExistingData(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := Run(ctx, db, log); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM users WHERE email = 'admin@demo.capitec.local'`); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, db, log); err != nil {
		t.Fatal(err)
	}

	var admins, disputes int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'ADMIN'`).Scan(&admins)
	_ = db.QueryRow(ctx, `SELECT count(*) FROM disputes`).Scan(&disputes)
	if admins != 1 || disputes != 3 {
		t.Fatalf("got %d admins and %d disputes, want 1 and 3", admins, disputes)
	}
}
