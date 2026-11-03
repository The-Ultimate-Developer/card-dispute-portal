// Package corebanking is the boundary to the bank's system of record.
// Everything else depends on the Provider interface, so the mock used here can
// be swapped for a real core-banking client without touching business logic.
package corebanking

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound      = errors.New("transaction not found")
	ErrInvalidCursor = errors.New("invalid cursor")
	ErrUnavailable   = errors.New("core banking unavailable")
)

type TransactionStatus string

const (
	StatusPending TransactionStatus = "PENDING"
	StatusPosted  TransactionStatus = "POSTED"
)

type Transaction struct {
	ID          string
	CardID      string
	CardBrand   string
	CardLast4   string
	Merchant    string
	Category    string
	AmountCents int64
	Currency    string
	Status      TransactionStatus
	OccurredAt  time.Time
}

type Page struct {
	Items      []Transaction
	NextCursor string
}

type Provider interface {
	ListTransactions(ctx context.Context, customerID, cursor string, limit int) (Page, error)
	GetTransaction(ctx context.Context, customerID, transactionID string) (Transaction, error)
}

// WithTimeout bounds every call. A slow core banking system must not hold
// request goroutines and database connections hostage.
func WithTimeout(p Provider, d time.Duration) Provider {
	return timeoutProvider{next: p, timeout: d}
}

type timeoutProvider struct {
	next    Provider
	timeout time.Duration
}

func (t timeoutProvider) ListTransactions(ctx context.Context, customerID, cursor string, limit int) (Page, error) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	page, err := t.next.ListTransactions(ctx, customerID, cursor, limit)
	return page, classify(ctx, err)
}

func (t timeoutProvider) GetTransaction(ctx context.Context, customerID, transactionID string) (Transaction, error) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	tx, err := t.next.GetTransaction(ctx, customerID, transactionID)
	return tx, classify(ctx, err)
}

func classify(ctx context.Context, err error) error {
	if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidCursor) || errors.Is(err, ErrUnavailable) {
		return err
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, ctx.Err())
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}
