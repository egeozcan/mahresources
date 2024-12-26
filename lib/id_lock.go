package lib

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// idLockState holds a channel of capacity 1 plus a reference counter.
type idLockState struct {
	ch   chan struct{}
	refs int
}

// logger is a basic logging interface.
type logger interface {
	Printf(format string, args ...interface{})
}

// stdLogger logs to stdout by default.
type stdLogger struct{}

func (stdLogger) Printf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// IDLock limits concurrency globally (if maxParallel > 0) and per-ID.
type IDLock[T comparable] struct {
	mu           sync.Mutex
	locks        map[T]*idLockState
	maxParallel  uint
	globalTokens chan struct{}
	log          logger
}

// NewIDLock returns an IDLock with an optional global concurrency limit.
func NewIDLock[T comparable](maxParallel uint, log logger) *IDLock[T] {
	if log == nil {
		log = stdLogger{}
	}
	return &IDLock[T]{
		locks:        make(map[T]*idLockState),
		maxParallel:  maxParallel,
		globalTokens: make(chan struct{}, maxParallel),
		log:          log,
	}
}

// releaseGlobalToken frees one global token (if any are in use).
func (l *IDLock[T]) releaseGlobalToken() {
	if l.maxParallel > 0 {
		select {
		case <-l.globalTokens:
			// Freed
		default:
			l.log.Printf("Warning: Release called but no global token in use\n")
		}
	}
}

// Acquire grabs a global token (if needed) and then pushes into the channel for that ID,
// blocking until the channel is free.
func (l *IDLock[T]) Acquire(id T) {
	// If there's a global limit, grab a token first
	if l.maxParallel > 0 {
		l.globalTokens <- struct{}{}
	}

	// Lock the map before reading or creating
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

	// Send into the channel (blocks if full)
	state.ch <- struct{}{}
}

// AcquireContext tries to Acquire within a context deadline or cancellation.
func (l *IDLock[T]) AcquireContext(ctx context.Context, id T) error {
	// Acquire global token if needed
	if l.maxParallel > 0 {
		select {
		case l.globalTokens <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Lock the map before reading or creating
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

	// If ctx was canceled after acquiring the global token but before sending to the channel:
	if err := ctx.Err(); err != nil {
		// Revert the ref
		l.mu.Lock()
		state.refs--
		if state.refs == 0 {
			delete(l.locks, id)
		}
		l.mu.Unlock()

		if l.maxParallel > 0 {
			l.releaseGlobalToken()
		}
		return err
	}

	select {
	case state.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		l.mu.Lock()
		state.refs--
		if state.refs == 0 {
			delete(l.locks, id)
		}
		l.mu.Unlock()

		if l.maxParallel > 0 {
			l.releaseGlobalToken()
		}
		return ctx.Err()
	}
}

// Release pops from the channel (thus unlocking) and frees a global token (if any).
func (l *IDLock[T]) Release(id T) {
	// Lock the map to safely access the state
	l.mu.Lock()
	state, ok := l.locks[id]
	if !ok {
		l.mu.Unlock()
		l.log.Printf("IDLock.Release called for id '%v' with no corresponding Acquire.\n", id)
		return
	}

	select {
	case <-state.ch:
		// Success, decrement refs
		state.refs--
		if state.refs == 0 {
			delete(l.locks, id)
		}
	default:
		l.log.Printf("IDLock.Release called for id '%v' without an Acquire on this goroutine.\n", id)
	}
	l.mu.Unlock()

	// Now free the global token if needed
	l.releaseGlobalToken()
}

// AcquireWithTimeout tries to Acquire the ID lock (and global token) within 'timeout'.
func (l *IDLock[T]) AcquireWithTimeout(id T, timeout time.Duration) bool {
	if timeout < 0 {
		return false
	}
	if timeout == 0 {
		// Purely non-blocking
		if l.maxParallel > 0 {
			select {
			case l.globalTokens <- struct{}{}:
			default:
				return false
			}
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

		select {
		case state.ch <- struct{}{}:
			return true
		default:
			// Revert
			l.mu.Lock()
			state.refs--
			if state.refs == 0 {
				delete(l.locks, id)
			}
			l.mu.Unlock()

			if l.maxParallel > 0 {
				l.releaseGlobalToken()
			}
			return false
		}
	}

	deadline := time.Now().Add(timeout)

	// Grab a token with timeout
	if l.maxParallel > 0 {
		select {
		case l.globalTokens <- struct{}{}:
		case <-time.After(timeout):
			return false
		}
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

	// Try to acquire the channel with the remaining time
	remaining := deadline.Sub(time.Now())
	if remaining <= 0 {
		// Timeout already
		l.mu.Lock()
		state.refs--
		if state.refs == 0 {
			delete(l.locks, id)
		}
		l.mu.Unlock()

		if l.maxParallel > 0 {
			l.releaseGlobalToken()
		}
		return false
	}

	select {
	case state.ch <- struct{}{}:
		return true
	case <-time.After(remaining):
		// Revert
		l.mu.Lock()
		state.refs--
		if state.refs == 0 {
			delete(l.locks, id)
		}
		l.mu.Unlock()

		if l.maxParallel > 0 {
			l.releaseGlobalToken()
		}
		return false
	}
}

// RunWithLockTimeout tries to lock the ID within 'lockTimeout', then runs fn
// with a runTimeout limit. Returns (bool, error) where bool indicates if the lock was acquired,
// and error represents any execution error or timeout.
func (l *IDLock[T]) RunWithLockTimeout(id T, lockTimeout, runTimeout time.Duration, fn func() error) (bool, error) {
	// Try to acquire the lock with timeout
	lockAcquired := l.AcquireWithTimeout(id, lockTimeout)
	if !lockAcquired {
		return false, nil // Lock not acquired within lockTimeout
	}
	defer l.Release(id)

	// Create a context with runTimeout
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	// Channel to receive the function's error
	errChan := make(chan error, 1)

	// Run the function in a separate goroutine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				l.log.Printf("Recovered from panic: %v", r)
				errChan <- fmt.Errorf("panic: %v", r)
			}
		}()
		errChan <- fn()
	}()

	// Wait for the function to complete or timeout
	select {
	case <-ctx.Done():
		return true, context.DeadlineExceeded
	case err := <-errChan:
		return true, err
	}
}
