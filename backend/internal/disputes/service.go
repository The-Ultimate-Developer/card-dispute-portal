package disputes

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/auth"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/corebanking"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/cursor"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/validate"
)

const (
	DisputeWindow    = 120 * 24 * time.Hour
	defaultPageLimit = 20
	maxPageLimit     = 50
)

var (
	ErrNotFound             = apperr.NotFound("DISPUTE_NOT_FOUND", "Dispute not found.")
	ErrActiveDisputeExists  = apperr.Conflict("ACTIVE_DISPUTE_EXISTS", "This transaction already has an open dispute.")
	ErrIdempotencyKeyReused = apperr.Unprocessable("IDEMPOTENCY_KEY_REUSED",
		"This Idempotency-Key was already used for a different request.")
	ErrStaleVersion = apperr.Conflict("STALE_VERSION",
		"This dispute was changed by someone else. Refresh and try again.")

	idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
)

func invalidTransition(from, to Status) error {
	return apperr.Unprocessable("INVALID_STATUS_TRANSITION", fmt.Sprintf("A dispute cannot move from %s to %s.", from, to))
}

type Observer interface {
	ObserveDisputeStatus(status string)
}

type nopObserver struct{}

func (nopObserver) ObserveDisputeStatus(string) {}

type Service struct {
	repo *Repository
	bank corebanking.Provider
	obs  Observer
	now  func() time.Time
}

func NewService(repo *Repository, bank corebanking.Provider, obs Observer) *Service {
	if obs == nil {
		obs = nopObserver{}
	}
	return &Service{repo: repo, bank: bank, obs: obs, now: time.Now}
}

type CreateInput struct {
	TransactionID  string
	Reason         Reason
	Description    string
	IdempotencyKey string
}

// Create returns the dispute and whether this call created it (false on an idempotent replay).
func (s *Service) Create(ctx context.Context, p auth.Principal, in CreateInput) (Dispute, bool, error) {
	if in.IdempotencyKey == "" {
		return Dispute{}, false, apperr.BadRequest("IDEMPOTENCY_KEY_REQUIRED", "The Idempotency-Key header is required.")
	}
	if !idempotencyKeyPattern.MatchString(in.IdempotencyKey) {
		return Dispute{}, false, apperr.BadRequest("IDEMPOTENCY_KEY_INVALID",
			"Idempotency-Key must be 8-128 characters of letters, digits, '-' or '_'.")
	}

	in.Description = strings.TrimSpace(in.Description)
	errs := validate.Errors{}
	if !validate.UUID(in.TransactionID) {
		errs.Add("transactionId", "must be a valid ID")
	}
	if !in.Reason.Valid() {
		errs.Add("reason", "is not a supported reason")
	}
	errs.Length("description", in.Description, 10, 1000)
	if errs.Any() {
		return Dispute{}, false, apperr.Validation(errs)
	}

	tx, err := s.bank.GetTransaction(ctx, p.UserID, in.TransactionID)
	switch {
	case errors.Is(err, corebanking.ErrNotFound):
		return Dispute{}, false, apperr.NotFound("TRANSACTION_NOT_FOUND", "Transaction not found.")
	case errors.Is(err, corebanking.ErrUnavailable):
		return Dispute{}, false, bankUnavailable(err)
	case err != nil:
		return Dispute{}, false, err
	}

	if tx.Status != corebanking.StatusPosted {
		return Dispute{}, false, apperr.Unprocessable("TRANSACTION_PENDING",
			"Pending transactions can't be disputed yet. Please try again once it has cleared.")
	}
	if s.now().Sub(tx.OccurredAt) > DisputeWindow {
		return Dispute{}, false, apperr.Unprocessable("TRANSACTION_TOO_OLD",
			"Transactions older than 120 days can't be disputed online. Please contact support.")
	}

	d, created, err := s.repo.create(ctx, createParams{
		CustomerID:     p.UserID,
		IdempotencyKey: in.IdempotencyKey,
		RequestHash:    requestHash(in),
		Transaction:    tx,
		Reason:         in.Reason,
		Description:    in.Description,
	})
	if err == nil && created {
		s.obs.ObserveDisputeStatus(string(StatusSubmitted))
	}
	return d, created, err
}

type ListInput struct {
	Status string
	Cursor string
	Limit  int
}

// List scopes by role: customers only ever see their own disputes; agents see the queue.
func (s *Service) List(ctx context.Context, p auth.Principal, in ListInput) (Page, error) {
	params := listParams{Limit: clampLimit(in.Limit)}

	if in.Status != "" {
		st := Status(in.Status)
		if !st.Valid() {
			return Page{}, apperr.Validation(map[string]string{"status": "is not a valid status"})
		}
		params.Status = &st
	}

	pos, err := cursor.Decode(in.Cursor)
	if err != nil {
		return Page{}, apperr.BadRequest("INVALID_CURSOR", "The pagination cursor is invalid.")
	}
	params.After = pos

	if p.Role == auth.RoleCustomer {
		params.CustomerID = &p.UserID
	}
	page, err := s.repo.list(ctx, params)
	if err != nil || !p.Role.Staff() {
		return page, err
	}
	return page, s.attachCustomers(ctx, page.Items)
}

// Get returns 404, not 403, for another customer's dispute so its existence is not revealed.
func (s *Service) Get(ctx context.Context, p auth.Principal, id string) (DisputeDetail, error) {
	if !validate.UUID(id) {
		return DisputeDetail{}, ErrNotFound
	}
	var owner *string
	if p.Role == auth.RoleCustomer {
		owner = &p.UserID
	}
	d, err := s.repo.detail(ctx, id, owner)
	if err != nil {
		return DisputeDetail{}, err
	}
	return s.withCustomer(ctx, p, d)
}

type ChangeStatusInput struct {
	Status  Status
	Version int
	Note    string
}

func (s *Service) ChangeStatus(ctx context.Context, p auth.Principal, id string, in ChangeStatusInput) (DisputeDetail, error) {
	if p.Role != auth.RoleAgent && p.Role != auth.RoleAdmin {
		return DisputeDetail{}, apperr.Forbidden("FORBIDDEN", "Only agents and admins can change a dispute's status.")
	}
	if !validate.UUID(id) {
		return DisputeDetail{}, ErrNotFound
	}

	in.Note = strings.TrimSpace(in.Note)
	errs := validate.Errors{}
	if !in.Status.Valid() {
		errs.Add("status", "is not a valid status")
	}
	if in.Version < 1 {
		errs.Add("version", "is required")
	}
	if in.Status.Final() {
		errs.Length("note", in.Note, 3, 500)
	} else {
		errs.Length("note", in.Note, 0, 500)
	}
	if errs.Any() {
		return DisputeDetail{}, apperr.Validation(errs)
	}

	d, err := s.repo.transition(ctx, transitionParams{
		ID:              id,
		ExpectedVersion: in.Version,
		To:              in.Status,
		ActorID:         p.UserID,
		ActorRole:       string(p.Role),
		Note:            in.Note,
	})
	if err != nil {
		return DisputeDetail{}, err
	}
	s.obs.ObserveDisputeStatus(string(in.Status))
	return s.withCustomer(ctx, p, d)
}

func (s *Service) withCustomer(ctx context.Context, p auth.Principal, d DisputeDetail) (DisputeDetail, error) {
	if !p.Role.Staff() {
		return d, nil
	}
	items := []Dispute{d.Dispute}
	if err := s.attachCustomers(ctx, items); err != nil {
		return DisputeDetail{}, err
	}
	d.Dispute = items[0]
	return d, nil
}

// attachCustomers loads every customer for a page in one query, avoiding N+1 lookups.
func (s *Service) attachCustomers(ctx context.Context, items []Dispute) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, d := range items {
		ids = append(ids, d.CustomerID)
	}
	customers, err := s.repo.customers(ctx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		if c, ok := customers[items[i].CustomerID]; ok {
			items[i].Customer = &c
		}
	}
	return nil
}

func (s *Service) SummariesByTransaction(ctx context.Context, customerID string, transactionIDs []string) (map[string]Summary, error) {
	return s.repo.summariesByTransaction(ctx, customerID, transactionIDs)
}

func bankUnavailable(cause error) error {
	return apperr.Unavailable("CORE_BANKING_UNAVAILABLE",
		"We can't reach our transaction system right now. Please try again shortly.", cause)
}

func requestHash(in CreateInput) []byte {
	h := sha256.New()
	for _, part := range []string{in.TransactionID, string(in.Reason), in.Description} {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum(nil)
}

func clampLimit(n int) int {
	switch {
	case n <= 0:
		return defaultPageLimit
	case n > maxPageLimit:
		return maxPageLimit
	}
	return n
}
