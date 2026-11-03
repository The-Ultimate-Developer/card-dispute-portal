package corebanking

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type BreakerConfig struct {
	FailureThreshold int
	OpenFor          time.Duration
}

type Observer interface {
	ObserveCoreBankingCall(operation, outcome string)
	SetCircuitOpen(open bool)
}

type breakerState int

const (
	closed breakerState = iota
	open
	halfOpen
)

// Breaker stops calling core banking after repeated failures, so an outage
// costs callers a fast 503 instead of a timeout each, and the struggling
// system gets room to recover. After OpenFor it lets one probe through:
// success closes the circuit, failure reopens it.
//
// Only availability failures count. "Not found" is a correct answer, not a fault.
type Breaker struct {
	next Provider
	cfg  BreakerConfig
	obs  Observer
	now  func() time.Time

	mu       sync.Mutex
	state    breakerState
	failures int
	openedAt time.Time
	probing  bool
}

func WithCircuitBreaker(p Provider, cfg BreakerConfig, obs Observer) *Breaker {
	return &Breaker{next: p, cfg: cfg, obs: obs, now: time.Now}
}

var errCircuitOpen = fmt.Errorf("%w: circuit open", ErrUnavailable)

func (b *Breaker) ListTransactions(ctx context.Context, customerID, cursor string, limit int) (Page, error) {
	if err := b.acquire(); err != nil {
		b.obs.ObserveCoreBankingCall("list_transactions", "rejected")
		return Page{}, err
	}
	page, err := b.next.ListTransactions(ctx, customerID, cursor, limit)
	b.record("list_transactions", err)
	return page, err
}

func (b *Breaker) GetTransaction(ctx context.Context, customerID, transactionID string) (Transaction, error) {
	if err := b.acquire(); err != nil {
		b.obs.ObserveCoreBankingCall("get_transaction", "rejected")
		return Transaction{}, err
	}
	tx, err := b.next.GetTransaction(ctx, customerID, transactionID)
	b.record("get_transaction", err)
	return tx, err
}

func (b *Breaker) acquire() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case open:
		if b.now().Sub(b.openedAt) < b.cfg.OpenFor {
			return errCircuitOpen
		}
		b.state = halfOpen
		b.probing = true
		return nil
	case halfOpen:
		if b.probing {
			return errCircuitOpen
		}
		b.probing = true
	}
	return nil
}

func (b *Breaker) record(op string, err error) {
	failed := errors.Is(err, ErrUnavailable)
	outcome := "success"
	switch {
	case failed:
		outcome = "unavailable"
	case err != nil:
		outcome = "client_error"
	}
	b.obs.ObserveCoreBankingCall(op, outcome)

	b.mu.Lock()
	defer b.mu.Unlock()

	if !failed {
		if b.state != closed {
			b.obs.SetCircuitOpen(false)
		}
		b.state, b.failures, b.probing = closed, 0, false
		return
	}

	b.failures++
	if b.state == halfOpen || b.failures >= b.cfg.FailureThreshold {
		if b.state != open {
			b.obs.SetCircuitOpen(true)
		}
		b.state, b.openedAt, b.probing = open, b.now(), false
	}
}
