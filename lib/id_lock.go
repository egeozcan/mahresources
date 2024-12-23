package lib

import (
	"context"
	"sync"
	"time"
)

// idLockState holds a channel of capacity 1 plus a ref counter.
type idLockState struct {
	ch   chan struct{}
	refs int
}

// IDLock limits concurrency globally (if maxParallel > 0) and per-ID.
type IDLock[T comparable] struct {
	mu           sync.Mutex // guards 'locks'
	locks        map[T]*idLockState
	maxParallel  uint
	globalTokens chan struct{}
}

// NewIDLock returns an IDLock with optional global concurrency limit.
func NewIDLock[T comparable](maxParallel uint) *IDLock[T] {
	return &IDLock[T]{
		locks:        make(map[T]*idLockState),
		maxParallel:  maxParallel,
		globalTokens: make(chan struct{}, maxParallel),
	}
}

// Acquire grabs a global token (if needed) and then pushes into the channel for that ID.
// This blocks until the channel is free (i.e., until the ID is “unlocked”).
func (l *IDLock[T]) Acquire(id T) {
	if l.maxParallel > 0 {
		l.globalTokens <- struct{}{}
	}

	l.mu.Lock()
	state, ok := l.locks[id]
	if !ok {
		state = &idLockState{
			ch:   make(chan struct{}, 1),
			refs: 0,
		}
		l.locks[id] = state
	}
	state.refs++
	l.mu.Unlock()

	// Acquire the ID's "lock" by pushing into the channel.
	state.ch <- struct{}{}
}

// Release pops from the channel (thus “unlocking”) and frees a global token (if any).
func (l *IDLock[T]) Release(id T) {
	l.mu.Lock()
	state, ok := l.locks[id]
	if ok {
		// Pop from the channel to unlock
		select {
		case <-state.ch:
			state.refs--
			if state.refs == 0 {
				delete(l.locks, id)
			}
		default:
			// Shouldn’t happen if we only call Release after Acquire
		}
	}
	l.mu.Unlock()

	if l.maxParallel > 0 {
		<-l.globalTokens
	}
}

// AcquireWithTimeout tries to Acquire the ID lock (and global token) within 'timeout'.
// Returns true if successful, false otherwise. No leftover goroutine is spawned.
func (l *IDLock[T]) AcquireWithTimeout(id T, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	// 1) Acquire global token or fail in time
	if l.maxParallel > 0 {
		select {
		case l.globalTokens <- struct{}{}:
			// success
		case <-time.After(timeout):
			return false
		}
	}

	// 2) Now handle the per-ID lock (channel).
	l.mu.Lock()
	state, ok := l.locks[id]
	if !ok {
		state = &idLockState{
			ch:   make(chan struct{}, 1),
			refs: 0,
		}
		l.locks[id] = state
	}
	state.refs++
	l.mu.Unlock()

	// 3) Attempt to send into the channel (lock) before the deadline
	remaining := time.Until(deadline)
	select {
	case state.ch <- struct{}{}:
		// success
		return true
	case <-time.After(remaining):
		// timed out, so revert the ref count and free global token if needed
		l.mu.Lock()
		state.refs--
		if state.refs == 0 {
			delete(l.locks, id)
		}
		l.mu.Unlock()

		if l.maxParallel > 0 {
			<-l.globalTokens
		}
		return false
	}
}

// TryRunWithTimeout tries to lock the ID within lockTimeout, then runs fn
// with a runTimeout limit. Returns false if we failed to acquire the lock at all.
func (l *IDLock[T]) TryRunWithTimeout(id T, lockTimeout, runTimeout time.Duration, fn func()) bool {
	if !l.AcquireWithTimeout(id, lockTimeout) {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			// Always release once fn finishes or panics
			if r := recover(); r != nil {
				// optional: handle the panic
			}
			l.Release(id)
		}()
		fn()
	}()

	select {
	case <-done:
		return true
	case <-ctx.Done():
		// The function timed out. We can’t forcibly kill the goroutine,
		// but logically we’re done waiting.
		return true
	}
}
