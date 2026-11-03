package transactions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/auth"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/corebanking"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/disputes"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/testdb"
)

func TestListMarksDisputabilityAndLinksDisputes(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	now := time.Now()

	custID := testdb.CreateUser(t, db, "c@example.com", "CUSTOMER", "x")
	card := testdb.CreateCard(t, db, custID, "4821")
	posted := testdb.CreateTransaction(t, db, card, "POSTED", 1000, now.Add(-time.Hour))
	pending := testdb.CreateTransaction(t, db, card, "PENDING", 1000, now.Add(-time.Minute))
	old := testdb.CreateTransaction(t, db, card, "POSTED", 1000, now.Add(-130*24*time.Hour))
	disputed := testdb.CreateTransaction(t, db, card, "POSTED", 1000, now.Add(-2*time.Hour))

	bank := corebanking.NewMock(db, corebanking.Faults{})
	disputeSvc := disputes.NewService(disputes.NewRepository(db), bank, nil)
	p := auth.Principal{UserID: custID, Role: auth.RoleCustomer}

	d, _, err := disputeSvc.Create(ctx, p, disputes.CreateInput{
		TransactionID: disputed, Reason: disputes.ReasonDuplicate, Description: "Charged twice for one order.", IdempotencyKey: "txn-test-key-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	page, err := NewService(bank, disputeSvc).List(ctx, p, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 4 {
		t.Fatalf("got %d items, want 4", len(page.Items))
	}

	byID := map[string]Transaction{}
	for _, tx := range page.Items {
		byID[tx.ID] = tx
	}
	if !byID[posted].Disputable || byID[pending].Disputable || byID[old].Disputable || byID[disputed].Disputable {
		t.Fatalf("wrong disputability: %+v", byID)
	}
	if byID[disputed].Dispute == nil || byID[disputed].Dispute.ID != d.ID {
		t.Fatal("disputed transaction should link its dispute")
	}
	if page.Items[0].ID != pending {
		t.Fatal("transactions should be newest first")
	}
}

type failingBank struct{}

func (failingBank) ListTransactions(context.Context, string, string, int) (corebanking.Page, error) {
	return corebanking.Page{}, corebanking.ErrUnavailable
}

func (failingBank) GetTransaction(context.Context, string, string) (corebanking.Transaction, error) {
	return corebanking.Transaction{}, corebanking.ErrUnavailable
}

func TestCoreBankingOutageIsA503(t *testing.T) {
	_, err := NewService(failingBank{}, nil).List(context.Background(), auth.Principal{UserID: "x"}, "", 10)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Kind != apperr.KindUnavailable || ae.Code != "CORE_BANKING_UNAVAILABLE" {
		t.Fatalf("got %v", err)
	}
}

type slowBank struct{ failingBank }

func (slowBank) ListTransactions(ctx context.Context, _, _ string, _ int) (corebanking.Page, error) {
	<-ctx.Done()
	return corebanking.Page{}, ctx.Err()
}

func TestTimeoutBoundsSlowCoreBanking(t *testing.T) {
	bank := corebanking.WithTimeout(slowBank{}, 50*time.Millisecond)

	start := time.Now()
	_, err := bank.ListTransactions(context.Background(), "x", "", 10)
	if !errors.Is(err, corebanking.ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("timeout was not enforced")
	}
}
