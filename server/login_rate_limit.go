package server

import (
	"sync"
	"time"
)

// loginRateLimiter throttles failed login attempts per client IP using a sliding
// window, to blunt online password-guessing. It is in-memory and per-process
// (sufficient for the single-binary deployment model); counters reset on
// restart. A nil limiter or a non-positive limit disables throttling entirely,
// so the no-auth and unconfigured paths are unaffected.
type loginRateLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu    sync.Mutex
	fails map[string][]time.Time
}

func newLoginRateLimiter(limit int, window time.Duration) *loginRateLimiter {
	return &loginRateLimiter{
		limit:  limit,
		window: window,
		now:    time.Now,
		fails:  make(map[string][]time.Time),
	}
}

// allowed reports whether key (a client IP) may make another login attempt.
func (l *loginRateLimiter) allowed(key string) bool {
	if l == nil || l.limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.countLocked(key) < l.limit
}

// recordFailure registers a failed attempt for key.
func (l *loginRateLimiter) recordFailure(key string) {
	if l == nil || l.limit <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.countLocked(key) // prune stale entries first
	l.fails[key] = append(l.fails[key], l.now())
}

// reset clears a key's recorded failures, e.g. after a successful login.
func (l *loginRateLimiter) reset(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
}

// countLocked prunes timestamps outside the window and returns the live count.
// The caller must hold l.mu.
func (l *loginRateLimiter) countLocked(key string) int {
	cutoff := l.now().Add(-l.window)
	times := l.fails[key]
	kept := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.fails, key)
		return 0
	}
	l.fails[key] = kept
	return len(kept)
}
