package disputes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/auth"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/corebanking"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/testdb"
)

type fixture struct {
	db        *pgxpool.Pool
	svc       *Service
	customer  auth.Principal
	other     auth.Principal
	agent     auth.Principal
	posted    string
	pending   string
	old       string
	othersTxn string
}

func setup(t *testing.T) fixture {
	t.Helper()
	db := testdb.New(t)
	now := time.Now()

	custID := testdb.CreateUser(t, db, "c1@example.com", "CUSTOMER", "x")
	otherID := testdb.CreateUser(t, db, "c2@example.com", "CUSTOMER", "x")
	agentID := testdb.CreateUser(t, db, "agent@example.com", "AGENT", "x")

	card := testdb.CreateCard(t, db, custID, "4821")
	otherCard := testdb.CreateCard(t, db, otherID, "7710")

	return fixture{
		db:        db,
		svc:       NewService(NewRepository(db), corebanking.NewMock(db, corebanking.Faults{}), nil),
		customer:  auth.Principal{UserID: custID, Role: auth.RoleCustomer},
		other:     auth.Principal{UserID: otherID, Role: auth.RoleCustomer},
		agent:     auth.Principal{UserID: agentID, Role: auth.RoleAgent},
		posted:    testdb.CreateTransaction(t, db, card, "POSTED", 49_900, now.Add(-48*time.Hour)),
		pending:   testdb.CreateTransaction(t, db, card, "PENDING", 12_000, now.Add(-time.Hour)),
		old:       testdb.CreateTransaction(t, db, card, "POSTED", 5_000, now.Add(-200*24*time.Hour)),
		othersTxn: testdb.CreateTransaction(t, db, otherCard, "POSTED", 9_900, now.Add(-24*time.Hour)),
	}
}

func input(txID, key string) CreateInput {
	return CreateInput{TransactionID: txID, Reason: ReasonUnrecognised, Description: "I did not make this purchase.", IdempotencyKey: key}
}

func code(err error) string {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return fmt.Sprintf("non-apperr: %v", err)
}

func TestCreateSnapshotsTransactionAndStartsTimeline(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	d, created, err := f.svc.Create(ctx, f.customer, input(f.posted, "key-00000001"))
	if err != nil || !created {
		t.Fatalf("create: created=%v err=%v", created, err)
	}
	if d.Status != StatusSubmitted || d.AmountCents != 49_900 || d.CardLast4 != "4821" || d.Version != 1 {
		t.Fatalf("unexpected dispute: %+v", d)
	}

	detail, err := f.svc.Get(ctx, f.customer, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Timeline) != 1 || detail.Timeline[0].ToStatus != StatusSubmitted || detail.Timeline[0].FromStatus != nil {
		t.Fatalf("unexpected timeline: %+v", detail.Timeline)
	}
}

func TestIdempotentReplayReturnsOriginal(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	first, _, err := f.svc.Create(ctx, f.customer, input(f.posted, "key-00000002"))
	if err != nil {
		t.Fatal(err)
	}
	again, created, err := f.svc.Create(ctx, f.customer, input(f.posted, "key-00000002"))
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("replay: id=%s created=%v err=%v", again.ID, created, err)
	}

	changed := input(f.posted, "key-00000002")
	changed.Description = "A different description entirely."
	if _, _, err := f.svc.Create(ctx, f.customer, changed); !errors.Is(err, ErrIdempotencyKeyReused) {
		t.Fatalf("same key, different body: got %v", err)
	}
}

func TestConcurrentSubmissionsCreateOneDispute(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := f.svc.Create(ctx, f.customer, input(f.posted, fmt.Sprintf("parallel-key-%02d", i)))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	ok, conflicts := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrActiveDisputeExists):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || conflicts != 9 {
		t.Fatalf("got %d created / %d conflicts, want 1 / 9", ok, conflicts)
	}
}

func TestBusinessRules(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	tests := []struct {
		name string
		in   CreateInput
		want string
	}{
		{"pending transaction", input(f.pending, "key-00000010"), "TRANSACTION_PENDING"},
		{"older than window", input(f.old, "key-00000011"), "TRANSACTION_TOO_OLD"},
		{"someone else's transaction", input(f.othersTxn, "key-00000012"), "TRANSACTION_NOT_FOUND"},
		{"missing idempotency key", input(f.posted, ""), "IDEMPOTENCY_KEY_REQUIRED"},
		{"bad idempotency key", input(f.posted, "short"), "IDEMPOTENCY_KEY_INVALID"},
		{"bad reason", CreateInput{TransactionID: f.posted, Reason: "BORED", Description: "long enough text", IdempotencyKey: "key-00000013"}, "VALIDATION_FAILED"},
		{"short description", CreateInput{TransactionID: f.posted, Reason: ReasonOther, Description: "  no  ", IdempotencyKey: "key-00000014"}, "VALIDATION_FAILED"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := f.svc.Create(ctx, f.customer, tc.in); code(err) != tc.want {
				t.Fatalf("got %s, want %s", code(err), tc.want)
			}
		})
	}
}

func TestCustomersCannotSeeEachOthersDisputes(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	d, _, err := f.svc.Create(ctx, f.customer, input(f.posted, "key-00000020"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Get(ctx, f.other, d.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other customer: got %v, want not found", err)
	}
	page, err := f.svc.List(ctx, f.other, ListInput{})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("other customer's list should be empty: %+v %v", page, err)
	}
	if _, err := f.svc.Get(ctx, f.agent, d.ID); err != nil {
		t.Fatalf("agent should see every dispute: %v", err)
	}
}

func TestLifecycleWithOptimisticLocking(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	d, _, err := f.svc.Create(ctx, f.customer, input(f.posted, "key-00000030"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.ChangeStatus(ctx, f.customer, d.ID, ChangeStatusInput{Status: StatusUnderReview, Version: 1}); code(err) != "FORBIDDEN" {
		t.Fatalf("customer changing status: got %s", code(err))
	}

	review, err := f.svc.ChangeStatus(ctx, f.agent, d.ID, ChangeStatusInput{Status: StatusUnderReview, Version: 1})
	if err != nil || review.Version != 2 || review.Status != StatusUnderReview {
		t.Fatalf("to review: %+v %v", review.Dispute, err)
	}

	if _, err := f.svc.ChangeStatus(ctx, f.agent, d.ID, ChangeStatusInput{Status: StatusResolved, Version: 1, Note: "Refunded"}); !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("stale version: got %v", err)
	}
	if _, err := f.svc.ChangeStatus(ctx, f.agent, d.ID, ChangeStatusInput{Status: StatusResolved, Version: 2}); code(err) != "VALIDATION_FAILED" {
		t.Fatalf("resolving without a note: got %s", code(err))
	}

	done, err := f.svc.ChangeStatus(ctx, f.agent, d.ID, ChangeStatusInput{Status: StatusResolved, Version: 2, Note: "Refunded in full"})
	if err != nil {
		t.Fatal(err)
	}
	if len(done.Timeline) != 3 || *done.Timeline[2].FromStatus != StatusUnderReview || done.Timeline[2].Note != "Refunded in full" {
		t.Fatalf("unexpected timeline: %+v", done.Timeline)
	}

	if _, err := f.svc.ChangeStatus(ctx, f.agent, d.ID, ChangeStatusInput{Status: StatusRejected, Version: 3, Note: "Changed mind"}); code(err) != "INVALID_STATUS_TRANSITION" {
		t.Fatalf("leaving a final state: got %s", code(err))
	}

	if _, _, err := f.svc.Create(ctx, f.customer, input(f.posted, "key-00000031")); code(err) != "DISPUTE_ALREADY_RESOLVED" {
		t.Fatalf("a transaction already refunded through a dispute can't be disputed again: got %s", code(err))
	}
}

func TestTimelineIsAppendOnly(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, _, err := f.svc.Create(ctx, f.customer, input(f.posted, "key-00000040")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(ctx, `UPDATE dispute_events SET note = 'tampered'`); err == nil {
		t.Fatal("editing the timeline must be refused by the database")
	}
}

func TestListPaginatesAndFilters(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	card := testdb.CreateCard(t, f.db, f.customer.UserID, "0001")
	for i := 0; i < 5; i++ {
		tx := testdb.CreateTransaction(t, f.db, card, "POSTED", 1000, time.Now().Add(-time.Duration(i+1)*time.Hour))
		if _, _, err := f.svc.Create(ctx, f.customer, input(tx, fmt.Sprintf("page-key-%04d", i))); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]bool{}
	next := ""
	for pages := 0; ; pages++ {
		page, err := f.svc.List(ctx, f.customer, ListInput{Limit: 2, Cursor: next})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range page.Items {
			if seen[d.ID] {
				t.Fatalf("dispute %s returned twice", d.ID)
			}
			seen[d.ID] = true
		}
		if next = page.NextCursor; next == "" {
			break
		}
		if pages > 5 {
			t.Fatal("pagination did not terminate")
		}
	}
	if len(seen) != 5 {
		t.Fatalf("saw %d disputes, want 5", len(seen))
	}

	page, err := f.svc.List(ctx, f.agent, ListInput{Status: "RESOLVED"})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("status filter: %+v %v", page, err)
	}
	if _, err := f.svc.List(ctx, f.agent, ListInput{Cursor: "garbage"}); code(err) != "INVALID_CURSOR" {
		t.Fatalf("bad cursor: got %s", code(err))
	}
}
