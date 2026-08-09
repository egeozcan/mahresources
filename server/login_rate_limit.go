package server

import (
	"crypto/sha256"
	"strings"
	"sync"
	"time"
)

// loginRateLimiter throttles failed login attempts per client IP and target
// username using a sliding window. It is in-memory and per-process (sufficient
// for the single-binary deployment model); counters reset on restart. A nil
// limiter or a non-positive limit disables throttling entirely.
const (
	loginRateLimiterMaxKeys       = 50000
	loginRateLimiterSweepInterval = time.Second
)

const (
	loginKeyOther byte = iota
	loginKeyIP
	loginKeyUsername
)

// loginRateLimitKey retains only a key kind and a fixed-size digest. In
// particular, attacker-controlled X-Forwarded-For values and usernames cannot
// make the bounded key map retain unbounded string memory.
type loginRateLimitKey [1 + sha256.Size]byte

type loginAttemptRecord struct {
	at            time.Time
	reservationID uint64 // zero once the reservation has completed as a failure
}

type loginRateLimiter struct {
	limit         int
	window        time.Duration
	maxKeys       int
	now           func() time.Time
	sweepInterval time.Duration

	mu                sync.Mutex
	fails             map[loginRateLimitKey][]loginAttemptRecord
	nextReservationID uint64
	nextSweepAt       time.Time
	onSweepVisit      func() // test hook: called once for each key visited by a capacity sweep
}

type loginReservation struct {
	limiter *loginRateLimiter
	id      uint64
	keys    []loginRateLimitKey
	once    sync.Once
}

func newLoginRateLimiter(limit int, window time.Duration) *loginRateLimiter {
	return &loginRateLimiter{
		limit:         limit,
		window:        window,
		maxKeys:       loginRateLimiterMaxKeys,
		now:           time.Now,
		sweepInterval: loginRateLimiterSweepInterval,
		fails:         make(map[loginRateLimitKey][]loginAttemptRecord),
	}
}

// reserve atomically checks and occupies capacity for every key. Holding the
// reservation while authentication runs prevents concurrent requests from all
// passing a separate check before any of them records its failure.
func (l *loginRateLimiter) reserve(rawKeys []string) (*loginReservation, bool) {
	if l == nil || l.limit <= 0 {
		return &loginReservation{}, true
	}

	keys := fixedLoginKeys(rawKeys)
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	for _, key := range keys {
		if l.countLockedAt(key, now) >= l.limit {
			return nil, false
		}
	}

	newKeys := l.newKeyCountLocked(keys)
	if len(l.fails)+newKeys > l.maxKeys {
		if l.nextSweepAt.IsZero() || !now.Before(l.nextSweepAt) {
			l.sweepLocked(now)
			l.nextSweepAt = now.Add(l.sweepInterval)
			newKeys = l.newKeyCountLocked(keys)
		}
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

// abandon resolves a reservation as "never judged". The pending record is
// dropped without becoming an in-window failure and without clearing prior
// failures. Used when authentication could not be completed for an
// infrastructure reason, so database contention neither locks out an account
// whose password was never mistyped nor resets an attacker's accumulated
// attempts.
func (r *loginReservation) abandon() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		if r.limiter != nil {
			r.limiter.abandon(r.id, r.keys)
		}
	})
}

func (l *loginRateLimiter) abandon(id uint64, keys []loginRateLimitKey) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, key := range keys {
		records := l.fails[key]
		kept := records[:0]
		for _, record := range records {
			if record.reservationID == id {
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

func (l *loginRateLimiter) complete(id uint64, keys []loginRateLimitKey, success bool) {
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
			if success && key[0] == loginKeyUsername && record.reservationID == 0 {
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

func fixedLoginKeys(rawKeys []string) []loginRateLimitKey {
	unique := make([]loginRateLimitKey, 0, len(rawKeys))
	seen := make(map[loginRateLimitKey]struct{}, len(rawKeys))
	for _, rawKey := range rawKeys {
		if rawKey == "" {
			continue
		}
		kind := loginKeyOther
		material := rawKey
		switch {
		case strings.HasPrefix(rawKey, "ip:"):
			kind = loginKeyIP
			material = strings.TrimPrefix(rawKey, "ip:")
		case strings.HasPrefix(rawKey, "user:"):
			kind = loginKeyUsername
			material = strings.TrimPrefix(rawKey, "user:")
		}
		digest := sha256.Sum256([]byte(material))
		var key loginRateLimitKey
		key[0] = kind
		copy(key[1:], digest[:])
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}
	return unique
}

func (l *loginRateLimiter) newKeyCountLocked(keys []loginRateLimitKey) int {
	count := 0
	for _, key := range keys {
		if _, exists := l.fails[key]; !exists {
			count++
		}
	}
	return count
}

// sweepLocked prunes stale completed failures while retaining every pending
// reservation. Caller must hold l.mu. Capacity-triggered sweeps are cadence
// limited so a flood of rejected new keys cannot repeatedly scan the full map.
func (l *loginRateLimiter) sweepLocked(now time.Time) {
	for key := range l.fails {
		if l.onSweepVisit != nil {
			l.onSweepVisit()
		}
		l.countLockedAt(key, now)
	}
}

// countLockedAt prunes completed failures outside the window and returns the
// live attempt count. Pending reservations never expire before completion.
// Caller must hold l.mu.
func (l *loginRateLimiter) countLockedAt(key loginRateLimitKey, now time.Time) int {
	cutoff := now.Add(-l.window)
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
