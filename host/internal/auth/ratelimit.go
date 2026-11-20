package auth

import (
	"sync"
	"time"
)

const (
	MaxFailures = 5
	Lockout     = 60 * time.Second
)

// Limiter is the global login limiter (single user behind one proxy IP).
type Limiter struct {
	mu          sync.Mutex
	now         Clock
	failures    int
	lockedUntil time.Time
}

func NewLimiter(now Clock) *Limiter { return &Limiter{now: now} }

// Attempt reserves one failed-attempt slot before the password is checked; Succeed releases all.
// In-flight attempts count, so concurrent requests cannot exceed MaxFailures per Lockout window.
// When locked it returns the time left.
func (l *Limiter) Attempt() (retryAfter time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Before(l.lockedUntil) {
		return l.lockedUntil.Sub(now), false
	}
	if !l.lockedUntil.IsZero() {
		l.failures = 0
		l.lockedUntil = time.Time{}
	}
	l.failures++
	if l.failures >= MaxFailures {
		l.lockedUntil = now.Add(Lockout)
	}
	return 0, true
}

func (l *Limiter) Succeed() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures = 0
	l.lockedUntil = time.Time{}
}
