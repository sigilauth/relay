package ratelimit

import (
	"golang.org/x/time/rate"
	"sync"
)

// Limiter implements per-fingerprint rate limiting.
// Default: 10 requests per minute.
type Limiter struct {
	limiters sync.Map // fingerprint -> *rate.Limiter
	rps      rate.Limit
	burst    int
}

// New creates a rate limiter with the specified rate per second and burst.
// For 10/min: rps = 10.0/60.0, burst = 1.
func New(rps float64, burst int) *Limiter {
	return &Limiter{
		rps:   rate.Limit(rps),
		burst: burst,
	}
}

// Allow checks if a request for the given fingerprint is allowed.
// Returns false if rate limit exceeded.
func (l *Limiter) Allow(fingerprint string) bool {
	limiter, _ := l.limiters.LoadOrStore(fingerprint, rate.NewLimiter(l.rps, l.burst))
	return limiter.(*rate.Limiter).Allow()
}

// Reset clears the rate limiter for a fingerprint.
// Used for testing or manual reset.
func (l *Limiter) Reset(fingerprint string) {
	l.limiters.Delete(fingerprint)
}
