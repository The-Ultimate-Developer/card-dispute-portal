package ratelimit

import (
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestLimiter(limit int, window time.Duration) (*Limiter, *clock) {
	c := &clock{t: time.Unix(1_700_000_000, 0)}
	l := New(limit, window)
	l.now = c.now
	return l, c
}

func TestAllowsBurstThenBlocks(t *testing.T) {
	l, _ := newTestLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("ip"); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	ok, wait := l.Allow("ip")
	if ok {
		t.Fatal("4th request should be blocked")
	}
	if wait <= 0 || wait > 20*time.Second {
		t.Fatalf("unexpected retry-after %v", wait)
	}
}

func TestRefillsOverTime(t *testing.T) {
	l, c := newTestLimiter(2, time.Minute)
	l.Allow("ip")
	l.Allow("ip")
	if ok, _ := l.Allow("ip"); ok {
		t.Fatal("should be empty")
	}
	c.advance(30 * time.Second)
	if ok, _ := l.Allow("ip"); !ok {
		t.Fatal("one token should have refilled after half the window")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l, _ := newTestLimiter(1, time.Minute)
	l.Allow("a")
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("a different key must have its own bucket")
	}
}

func TestIdleBucketsAreSwept(t *testing.T) {
	l, c := newTestLimiter(1, time.Minute)
	l.Allow("a")
	c.advance(5 * time.Minute)
	l.Allow("b")
	if _, exists := l.buckets["a"]; exists {
		t.Fatal("idle bucket should have been swept")
	}
}
