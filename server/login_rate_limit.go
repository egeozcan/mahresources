package server

import (
	"strings"
	"sync"
	"time"
)

// loginRateLimiter throttles failed login attempts per client IP and target
// username using a sliding window. It is in-memory and per-process (sufficient
// for the single-binary deployment model); counters reset on restart. A nil
// limiter or a non-positive limit disables throttling entirely.
const loginRateLimiterMaxKeys = 50000

type loginAttemptRecord struct {
	at            time.Time
	reservationID uint64 // zero once the reservation has completed as a failure
}

type loginRateLimiter struct {
	limit   int
	window  time.Duration
	maxKeys int
	now     func() time.Time

	mu                sync.Mutex
	fails             map[string][]loginAttemptRecord
	nextReservationID uint64
}

type loginReservation struct {
	limiter *loginRateLimiter
	id      uint64
	keys    []string
	once    sync.Once
}

func newLoginRateLimiter(limit int, window time.Duration) *loginRateLimiter {
	return &loginRateLimiter{
		limit:   limit,
		window:  window,
		maxKeys: loginRateLimiterMaxKeys,
		now:     time.Now,
		fails:   make(map[string][]loginAttemptRecord),
	}
}

// reserve atomically checks and occupies capacity for every key. Holding the
// reservation while authentication runs prevents concurrent requests from all
// passing a separate check before any of them records its failure.
func (l *loginRateLimiter) reserve(keys []string) (*loginReservation, bool) {
	if l == nil || l.limit <= 0 {
		return &loginReservation{}, true
	}

	keys = uniqueLoginKeys(keys)
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, key := range keys {
		if l.countLocked(key) >= l.limit {
			return nil, false
		}
	}

	newKeys := l.newKeyCountLocked(keys)
	if len(l.fails)+newKeys > l.maxKeys {
		l.sweepLocked()
		newKeys = l.newKeyCountLocked(keys)
		if len(l.fails)+newKeys > l.maxKeys {
			// Existing, live security state is never discarded to admit an
			// untracked key. Fail closed until stale failures can be swept.
			return nil, false
		}
	}

	l.nextReservationID++
	if l.nextReservationID == 0 {
		l.nextReservationID++
	}
	id := l.nextReservationID
	now := l.now()
	for _, key := range keys {
		l.fails[key] = append(l.fails[key], loginAttemptRecord{
			at:            now,
			reservationID: id,
		})
	}
	return &loginReservation{limiter: l, id: id, keys: keys}, true
}

// complete resolves a reservation exactly once. A failure becomes an in-window
// attempt on every key. A success releases the pending reservation and clears
// historical failures only for the successfully authenticated username; IP
// failures remain so logging into another account cannot reset an attacker's IP.
func (r *loginReservation) complete(success bool) {
	if r == nil {
		return
	}
	r.once.Do(func() {
		if r.limiter != nil {
			r.limiter.complete(r.id, r.keys, success)
		}
	})
}

func (l *loginRateLimiter) complete(id uint64, keys []string, success bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	for _, key := range keys {
		records := l.fails[key]
		kept := records[:0]
		for _, record := range records {
			if record.reservationID == id {
				if success {
					continue
				}
				record.at = now
				record.reservationID = 0
			}
			if success && strings.HasPrefix(key, "user:") && record.reservationID == 0 {
				continue
			}
			kept = append(kept, record)
		}
		if len(kept) == 0 {
			delete(l.fails, key)
		} else {
			l.fails[key] = kept
		}
	}
}

func uniqueLoginKeys(keys []string) []string {
	unique := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}
	return unique
}

func (l *loginRateLimiter) newKeyCountLocked(keys []string) int {
	count := 0
	for _, key := range keys {
		if _, exists := l.fails[key]; !exists {
			count++
		}
	}
	return count
}

// sweepLocked prunes stale completed failures while retaining every pending
// reservation. Caller must hold l.mu.
func (l *loginRateLimiter) sweepLocked() {
	for key := range l.fails {
		l.countLocked(key)
	}
}

// countLocked prunes completed failures outside the window and returns the live
// attempt count. Pending reservations never expire before completion. Caller
// must hold l.mu.
func (l *loginRateLimiter) countLocked(key string) int {
	cutoff := l.now().Add(-l.window)
	records := l.fails[key]
	kept := records[:0]
	for _, record := range records {
		if record.reservationID != 0 || record.at.After(cutoff) {
			kept = append(kept, record)
		}
	}
	if len(kept) == 0 {
		delete(l.fails, key)
		return 0
	}
	l.fails[key] = kept
	return len(kept)
}
