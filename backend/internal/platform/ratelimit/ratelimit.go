// Package ratelimit is an in-memory token bucket keyed by caller.
// It is correct for a single instance; running several instances needs a shared
// store (e.g. Redis) behind the same Allow signature.
package ratelimit

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/httpx"
)

type bucket struct {
	tokens float64
	last   time.Time
}

type Limiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	burst     float64
	perSecond float64
	idleTTL   time.Duration
	lastSweep time.Time
	now       func() time.Time
}

// New allows `limit` requests per `window` per key, refilling continuously.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		buckets:   make(map[string]*bucket),
		burst:     float64(limit),
		perSecond: float64(limit) / window.Seconds(),
		idleTTL:   2 * window,
		now:       time.Now,
	}
}

func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}

	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.perSecond)
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.perSecond * float64(time.Second))
	return false, wait
}

// sweep drops idle buckets so an attacker rotating keys cannot grow memory forever.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	for k, b := range l.buckets {
		if now.Sub(b.last) > l.idleTTL {
			delete(l.buckets, k)
		}
	}
}

func (l *Limiter) Middleware(log *slog.Logger, key func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ok, wait := l.Allow(key(r)); !ok {
				httpx.WriteError(w, r, log, apperr.RateLimited(wait))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
