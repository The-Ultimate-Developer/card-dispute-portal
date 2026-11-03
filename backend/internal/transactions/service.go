package transactions

import (
	"context"
	"errors"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/auth"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/corebanking"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/disputes"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
)

const (
	defaultPageLimit = 20
	maxPageLimit     = 50
)

type DisputeIndex interface {
	SummariesByTransaction(ctx context.Context, customerID string, transactionIDs []string) (map[string]disputes.Summary, error)
}

type Transaction struct {
	ID          string                        `json:"id"`
	CardID      string                        `json:"cardId"`
	CardBrand   string                        `json:"cardBrand"`
	CardLast4   string                        `json:"cardLast4"`
	Merchant    string                        `json:"merchant"`
	Category    string                        `json:"category"`
	AmountCents int64                         `json:"amountCents"`
	Currency    string                        `json:"currency"`
	Status      corebanking.TransactionStatus `json:"status"`
	OccurredAt  time.Time                     `json:"occurredAt"`
	Disputable  bool                          `json:"disputable"`
	Dispute     *disputes.Summary             `json:"dispute"`
}

type Page struct {
	Items      []Transaction `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

type Service struct {
	bank     corebanking.Provider
	disputes DisputeIndex
	now      func() time.Time
}

func NewService(bank corebanking.Provider, idx DisputeIndex) *Service {
	return &Service{bank: bank, disputes: idx, now: time.Now}
}

// List enriches the core banking page with each transaction's latest dispute,
// and decides disputability on the server so the UI and API never disagree.
func (s *Service) List(ctx context.Context, p auth.Principal, after string, limit int) (Page, error) {
	switch {
	case limit <= 0:
		limit = defaultPageLimit
	case limit > maxPageLimit:
		limit = maxPageLimit
	}

	page, err := s.bank.ListTransactions(ctx, p.UserID, after, limit)
	switch {
	case errors.Is(err, corebanking.ErrInvalidCursor):
		return Page{}, apperr.BadRequest("INVALID_CURSOR", "The pagination cursor is invalid.")
	case errors.Is(err, corebanking.ErrUnavailable):
		return Page{}, apperr.Unavailable("CORE_BANKING_UNAVAILABLE",
			"We can't load your transactions right now. Please try again shortly.", err)
	case err != nil:
		return Page{}, err
	}

	ids := make([]string, len(page.Items))
	for i, t := range page.Items {
		ids[i] = t.ID
	}
	summaries, err := s.disputes.SummariesByTransaction(ctx, p.UserID, ids)
	if err != nil {
		return Page{}, err
	}

	out := Page{Items: make([]Transaction, len(page.Items)), NextCursor: page.NextCursor}
	now := s.now()
	for i, t := range page.Items {
		item := Transaction{
			ID: t.ID, CardID: t.CardID, CardBrand: t.CardBrand, CardLast4: t.CardLast4,
			Merchant: t.Merchant, Category: t.Category, AmountCents: t.AmountCents,
			Currency: t.Currency, Status: t.Status, OccurredAt: t.OccurredAt,
		}
		if sum, ok := summaries[t.ID]; ok {
			item.Dispute = &sum
		}
		item.Disputable = t.Status == corebanking.StatusPosted &&
			now.Sub(t.OccurredAt) <= disputes.DisputeWindow &&
			(item.Dispute == nil || item.Dispute.Status.Final())
		out.Items[i] = item
	}
	return out, nil
}
