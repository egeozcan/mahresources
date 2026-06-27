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
}

type generationRateBucket struct {
	start time.Time
	count int
}

func NewMRQLGenerationRateLimiter(max int, window time.Duration) *MRQLGenerationRateLimiter {
	return &MRQLGenerationRateLimiter{
		max:    max,
		window: window,
		keys:   map[string]generationRateBucket{},
	}
}

func (l *MRQLGenerationRateLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.keys[key]
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
