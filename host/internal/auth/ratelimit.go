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

// Allow reports whether a login attempt may be checked; when locked it returns the time left.
func (l *Limiter) Allow() (retryAfter time.Duration, ok bool) {
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
	return 0, true
}

func (l *Limiter) Fail() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures++
	if l.failures >= MaxFailures {
		l.lockedUntil = l.now().Add(Lockout)
	}
}

func (l *Limiter) Succeed() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures = 0
	l.lockedUntil = time.Time{}
}
