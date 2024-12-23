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

// getOrCreateState finds or creates the idLockState for a given ID.
func (l *IDLock[T]) getOrCreateState(id T) *idLockState {
	state, ok := l.locks[id]
	if !ok {
		state = &idLockState{
			ch:   make(chan struct{}, 1),
			refs: 0,
		}
		l.locks[id] = state
	}
	return state
}

// rollbackState decrements refs and removes the state if no one else is using it.
func (l *IDLock[T]) rollbackState(id T, state *idLockState) {
	state.refs--
	if state.refs == 0 {
		delete(l.locks, id)
	}
}

// releaseGlobalToken frees one global token (if any are in use).
func (l *IDLock[T]) releaseGlobalToken() {
	if l.maxParallel > 0 {
		// We know we must have a token if Acquire succeeded,
		// but we still do 'select' to preserve your existing logic.
		select {
		case <-l.globalTokens:
			// Freed a global token
		default:
			// Shouldn’t happen
		}
	}
}

// Acquire grabs a global token (if needed) and then pushes into the channel for that ID,
// blocking until the channel is free.
func (l *IDLock[T]) Acquire(id T) {
	if l.maxParallel > 0 {
		l.globalTokens <- struct{}{}
	}

	l.mu.Lock()
	state := l.getOrCreateState(id)
	state.refs++
	l.mu.Unlock()

	// Acquire the ID's "lock" by pushing into the channel (blocks if channel is full).
	state.ch <- struct{}{}
}

// Release pops from the channel (thus unlocking) and frees a global token (if any).
func (l *IDLock[T]) Release(id T) {
	l.mu.Lock()
	state, ok := l.locks[id]
	if ok {
		// Pop from the channel
		select {
		case <-state.ch:
			l.rollbackState(id, state)
		default:
			// Shouldn’t happen if Acquire was truly successful
		}
	}
	l.mu.Unlock()

	l.releaseGlobalToken()
}

// AcquireWithTimeout tries to Acquire the ID lock (and global token) within 'timeout'.
// Returns true if successful, false otherwise. No leftover goroutine is spawned.
func (l *IDLock[T]) AcquireWithTimeout(id T, timeout time.Duration) bool {
	// 1) If timeout < 0, always fail
	if timeout < 0 {
		return false
	}

	// 2) If timeout == 0, do a purely non-blocking attempt
	if timeout == 0 {
		// Non-blocking attempt to acquire a global token (if needed)
		if l.maxParallel > 0 {
			select {
			case l.globalTokens <- struct{}{}:
				// success
			default:
				return false
			}
		}

		// Non-blocking attempt to "create or find" the ID’s channel
		l.mu.Lock()
		state := l.getOrCreateState(id)
		state.refs++
		l.mu.Unlock()

		// Non-blocking attempt to send into the channel
		select {
		case state.ch <- struct{}{}:
			// success, got the lock
			return true
		default:
			// lock is in use, revert
			l.mu.Lock()
			l.rollbackState(id, state)
			l.mu.Unlock()

			if l.maxParallel > 0 {
				l.releaseGlobalToken()
			}
			return false
		}
	}

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
	state := l.getOrCreateState(id)
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
		l.rollbackState(id, state)
		l.mu.Unlock()

		if l.maxParallel > 0 {
			l.releaseGlobalToken()
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
		// Timed out, but we did acquire the lock, so return true to indicate
		// the function got the lock (even though it’s still running).
		return true
	}
}
