package corebanking

import (
	"context"
	"errors"
	"testing"
	"time"
)

type scriptedProvider struct {
	err   error
	calls int
}

func (s *scriptedProvider) ListTransactions(context.Context, string, string, int) (Page, error) {
	s.calls++
	return Page{}, s.err
}

func (s *scriptedProvider) GetTransaction(context.Context, string, string) (Transaction, error) {
	s.calls++
	return Transaction{}, s.err
}

type nopObserver struct{ open bool }

func (*nopObserver) ObserveCoreBankingCall(string, string) {}
func (o *nopObserver) SetCircuitOpen(v bool)               { o.open = v }

func newBreaker(p Provider) (*Breaker, *time.Time, *nopObserver) {
	now := time.Unix(1_700_000_000, 0)
	obs := &nopObserver{}
	b := WithCircuitBreaker(p, BreakerConfig{FailureThreshold: 3, OpenFor: 30 * time.Second}, obs)
	b.now = func() time.Time { return now }
	return b, &now, obs
}

func call(b *Breaker) error {
	_, err := b.ListTransactions(context.Background(), "c", "", 10)
	return err
}

func TestOpensAfterThresholdAndFailsFast(t *testing.T) {
	p := &scriptedProvider{err: ErrUnavailable}
	b, _, obs := newBreaker(p)

	for i := 0; i < 3; i++ {
		_ = call(b)
	}
	if !obs.open {
		t.Fatal("circuit should be open after 3 failures")
	}

	err := call(b)
	if !errors.Is(err, ErrUnavailable) || p.calls != 3 {
		t.Fatalf("open circuit must fail fast without calling through: err=%v calls=%d", err, p.calls)
	}
}

func TestNotFoundDoesNotTrip(t *testing.T) {
	p := &scriptedProvider{err: ErrNotFound}
	b, _, obs := newBreaker(p)
	for i := 0; i < 10; i++ {
		_ = call(b)
	}
	if obs.open || p.calls != 10 {
		t.Fatal("not-found answers must not open the circuit")
	}
}

func TestHalfOpenProbeClosesOnSuccess(t *testing.T) {
	p := &scriptedProvider{err: ErrUnavailable}
	b, now, obs := newBreaker(p)
	for i := 0; i < 3; i++ {
		_ = call(b)
	}

	*now = now.Add(31 * time.Second)
	p.err = nil
	if err := call(b); err != nil {
		t.Fatalf("probe should be allowed through: %v", err)
	}
	if obs.open {
		t.Fatal("successful probe should close the circuit")
	}
	if err := call(b); err != nil {
		t.Fatalf("closed circuit should pass calls: %v", err)
	}
}

func TestHalfOpenProbeReopensOnFailure(t *testing.T) {
	p := &scriptedProvider{err: ErrUnavailable}
	b, now, obs := newBreaker(p)
	for i := 0; i < 3; i++ {
		_ = call(b)
	}

	*now = now.Add(31 * time.Second)
	_ = call(b)
	if !obs.open {
		t.Fatal("failed probe should reopen the circuit")
	}
	before := p.calls
	_ = call(b)
	if p.calls != before {
		t.Fatal("reopened circuit must fail fast again")
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	p := &scriptedProvider{err: ErrUnavailable}
	b, _, obs := newBreaker(p)
	_ = call(b)
	_ = call(b)
	p.err = nil
	_ = call(b)
	p.err = ErrUnavailable
	_ = call(b)
	_ = call(b)
	if obs.open {
		t.Fatal("failures must be consecutive to open the circuit")
	}
}
