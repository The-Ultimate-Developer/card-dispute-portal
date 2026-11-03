package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/outbox"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/testdb"
)

type captureSender struct {
	mu   sync.Mutex
	fail error
	sent []Email
}

func (c *captureSender) Send(_ context.Context, e Email) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	c.sent = append(c.sent, e)
	return nil
}

type countingObserver struct {
	mu       sync.Mutex
	outcomes []string
}

func (o *countingObserver) ObserveDelivery(_, outcome string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.outcomes = append(o.outcomes, outcome)
}

type fixture struct {
	db     *pgxpool.Pool
	sender *captureSender
	worker *Worker
}

func setup(t *testing.T, maxAttempts int) fixture {
	t.Helper()
	db := testdb.New(t)
	sender := &captureSender{}
	w := NewWorker(db, sender, &countingObserver{}, slog.New(slog.NewTextHandler(io.Discard, nil)),
		Config{PollInterval: time.Second, BatchSize: 10, MaxAttempts: maxAttempts, Lease: time.Minute})
	return fixture{db: db, sender: sender, worker: w}
}

func (f fixture) enqueue(t *testing.T, topic string, payload any) int64 {
	t.Helper()
	ctx := context.Background()
	if err := pgx.BeginFunc(ctx, f.db, func(tx pgx.Tx) error { return outbox.Enqueue(ctx, tx, topic, payload) }); err != nil {
		t.Fatal(err)
	}
	var id int64
	_ = f.db.QueryRow(ctx, `SELECT max(id) FROM outbox`).Scan(&id)
	return id
}

func (f fixture) status(t *testing.T, id int64) (string, int) {
	t.Helper()
	var status string
	var attempts int
	if err := f.db.QueryRow(context.Background(), `SELECT status, attempts FROM outbox WHERE id = $1`, id).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	return status, attempts
}

func (f fixture) makeDue(t *testing.T) {
	t.Helper()
	if _, err := f.db.Exec(context.Background(), `UPDATE outbox SET available_at = now() WHERE status = 'PENDING'`); err != nil {
		t.Fatal(err)
	}
}

func seedDispute(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	user := testdb.CreateUser(t, db, "thandi@example.com", "CUSTOMER", "x")
	var id string
	err := db.QueryRow(context.Background(), `
		INSERT INTO disputes (customer_id, transaction_id, merchant, amount_cents, currency, card_last4,
			transaction_occurred_at, reason, description, status)
		VALUES ($1, gen_random_uuid(), 'Takealot.com', 123456, 'ZAR', '4821', '2026-10-01', 'DUPLICATE', 'Charged twice.', 'SUBMITTED')
		RETURNING id`, user).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDeliversDisputeEmail(t *testing.T) {
	f := setup(t, 5)
	disputeID := seedDispute(t, f.db)
	id := f.enqueue(t, outbox.TopicDisputeStatusChanged, map[string]string{
		"disputeId": disputeID, "toStatus": "RESOLVED", "note": "Refunded in full.",
	})

	if n, err := f.worker.RunOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("run: n=%d err=%v", n, err)
	}
	if status, _ := f.status(t, id); status != "SENT" {
		t.Fatalf("status %s, want SENT", status)
	}

	e := f.sender.sent[0]
	if e.To != "thandi@example.com" || e.ID == "" || !strings.Contains(e.Subject, "resolved") {
		t.Fatalf("unexpected email: %+v", e)
	}
	for _, want := range []string{"Hi Test,", "R1 234.56", "Takealot.com", "Refunded in full."} {
		if !strings.Contains(e.Body, want) {
			t.Errorf("body missing %q:\n%s", want, e.Body)
		}
	}
}

func TestFailedSendIsRetriedWithBackoff(t *testing.T) {
	f := setup(t, 5)
	id := f.enqueue(t, outbox.TopicDisputeCreated, map[string]string{"disputeId": seedDispute(t, f.db)})

	f.sender.fail = errors.New("smtp: connection refused")
	_, _ = f.worker.RunOnce(context.Background())

	status, attempts := f.status(t, id)
	if status != "PENDING" || attempts != 1 {
		t.Fatalf("after failure: %s/%d, want PENDING/1", status, attempts)
	}
	if n, _ := f.worker.RunOnce(context.Background()); n != 0 {
		t.Fatal("message should wait for its backoff before retrying")
	}

	f.sender.fail = nil
	f.makeDue(t)
	_, _ = f.worker.RunOnce(context.Background())
	if status, attempts := f.status(t, id); status != "SENT" || attempts != 2 {
		t.Fatalf("after retry: %s/%d, want SENT/2", status, attempts)
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	f := setup(t, 2)
	id := f.enqueue(t, outbox.TopicDisputeCreated, map[string]string{"disputeId": seedDispute(t, f.db)})
	f.sender.fail = errors.New("provider down")

	_, _ = f.worker.RunOnce(context.Background())
	f.makeDue(t)
	_, _ = f.worker.RunOnce(context.Background())

	if status, attempts := f.status(t, id); status != "DEAD" || attempts != 2 {
		t.Fatalf("got %s/%d, want DEAD/2", status, attempts)
	}
}

func TestUnrenderableMessageGoesStraightToDead(t *testing.T) {
	f := setup(t, 5)
	id := f.enqueue(t, outbox.TopicDisputeCreated, map[string]string{"disputeId": "00000000-0000-0000-0000-000000000000"})

	_, _ = f.worker.RunOnce(context.Background())
	if status, attempts := f.status(t, id); status != "DEAD" || attempts != 1 {
		t.Fatalf("got %s/%d, want DEAD/1 without pointless retries", status, attempts)
	}
}

// A worker that crashes mid-send leaves its claimed message leased; once the
// lease expires another worker delivers it.
func TestExpiredLeaseIsRedelivered(t *testing.T) {
	f := setup(t, 5)
	id := f.enqueue(t, outbox.TopicAccountLocked, map[string]string{"userId": testdb.CreateUser(t, f.db, "x@example.com", "CUSTOMER", "x")})

	if _, err := f.worker.claim(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n, _ := f.worker.RunOnce(context.Background()); n != 0 {
		t.Fatal("a leased message must not be claimed twice")
	}

	f.makeDue(t)
	_, _ = f.worker.RunOnce(context.Background())
	if status, _ := f.status(t, id); status != "SENT" {
		t.Fatalf("expired lease should be redelivered, got %s", status)
	}
	if !strings.Contains(f.sender.sent[0].Subject, "blocked") {
		t.Fatalf("unexpected subject: %s", f.sender.sent[0].Subject)
	}
}

func TestConcurrentWorkersNeverDoubleSend(t *testing.T) {
	f := setup(t, 5)
	disputeID := seedDispute(t, f.db)
	for i := 0; i < 30; i++ {
		f.enqueue(t, outbox.TopicDisputeCreated, map[string]string{"disputeId": disputeID})
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if n, err := f.worker.RunOnce(context.Background()); err != nil || n == 0 {
					return
				}
			}
		}()
	}
	wg.Wait()

	seen := map[string]bool{}
	for _, e := range f.sender.sent {
		if seen[e.ID] {
			t.Fatalf("email %s sent twice", e.ID)
		}
		seen[e.ID] = true
	}
	if len(seen) != 30 {
		t.Fatalf("sent %d emails, want 30", len(seen))
	}
}

func TestHelpers(t *testing.T) {
	cases := map[int64]string{5: "R0.05", 123456: "R1 234.56", 100000000: "R1 000 000.00", -2550: "-R25.50"}
	for cents, want := range cases {
		if got := money(cents, "ZAR"); got != want {
			t.Errorf("money(%d) = %q, want %q", cents, got, want)
		}
	}
	if maskEmail("thandi@example.com") != "t***@example.com" || maskEmail("nonsense") != "***" {
		t.Error("maskEmail")
	}
	if backoff(1) != 2*time.Second || backoff(3) != 8*time.Second || backoff(50) != 10*time.Minute {
		t.Errorf("backoff: %v %v %v", backoff(1), backoff(3), backoff(50))
	}
}

func TestBatchIsDeliveredInOrder(t *testing.T) {
	f := setup(t, 5)
	disputeID := seedDispute(t, f.db)
	for i := 0; i < 20; i++ {
		f.enqueue(t, outbox.TopicDisputeCreated, map[string]string{"disputeId": disputeID})
	}
	for {
		n, err := f.worker.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	ids := make([]int, len(f.sender.sent))
	for i, e := range f.sender.sent {
		ids[i], _ = strconv.Atoi(strings.TrimPrefix(e.ID, "outbox-"))
	}
	if len(ids) != 20 || !slices.IsSorted(ids) {
		t.Fatalf("emails out of order: %v", ids)
	}
}
