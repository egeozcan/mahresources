package application_context

import (
	"sync"
	"time"
)

type MRQLGenerationRateLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	keys   map[string]generationRateBucket
	// maxKeys bounds the map; nextSweep schedules the expiry sweep.
	maxKeys   int
	nextSweep time.Time
}

// generationRateLimiterMaxKeys caps tracked keys. Past it, new keys are refused
// (fail closed) rather than evicting live buckets, as loginRateLimiter does.
const generationRateLimiterMaxKeys = 10000

type generationRateBucket struct {
	start time.Time
	count int
}

func NewMRQLGenerationRateLimiter(max int, window time.Duration) *MRQLGenerationRateLimiter {
	return &MRQLGenerationRateLimiter{
		max:     max,
		window:  window,
		keys:    map[string]generationRateBucket{},
		maxKeys: generationRateLimiterMaxKeys,
	}
}

func (l *MRQLGenerationRateLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !now.Before(l.nextSweep) {
		l.sweep(now)
	}

	b := l.keys[key]
	if _, exists := l.keys[key]; !exists && len(l.keys) >= l.maxKeys {
		l.sweep(now)
		if len(l.keys) >= l.maxKeys {
			return false
		}
	}
	if b.start.IsZero() || now.Sub(b.start) >= l.window {
		l.keys[key] = generationRateBucket{start: now, count: 1}
		return true
	}
	if b.count >= l.max {
		return false
	}
	b.count++
	l.keys[key] = b
	return true
}

// sweep drops every bucket whose window has elapsed. Caller holds l.mu.
func (l *MRQLGenerationRateLimiter) sweep(now time.Time) {
	for k, b := range l.keys {
		if now.Sub(b.start) >= l.window {
			delete(l.keys, k)
		}
	}
	l.nextSweep = now.Add(l.window)
}
