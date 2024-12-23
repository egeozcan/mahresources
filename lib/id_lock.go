package lib

import (
	"context"
	"sync"
	"time"
)

// idLockState holds a mutex and a reference count for a particular ID.
type idLockState struct {
	mu   *sync.Mutex
	refs int
}

// IDLock controls concurrency for a given ID plus (optionally) a global concurrency limit.
type IDLock[T comparable] struct {
	lockMutex    sync.Mutex
	locks        map[T]*idLockState
	maxParallel  uint
	globalTokens chan struct{}
}

// NewIDLock initializes an IDLock with an optional global concurrency limit.
func NewIDLock[T comparable](maxParallel uint) *IDLock[T] {
	return &IDLock[T]{
		locks:        make(map[T]*idLockState),
		maxParallel:  maxParallel,
		globalTokens: make(chan struct{}, maxParallel),
	}
}

// Acquire blocks until it grabs both the global token (if any) and the per-ID lock.
func (l *IDLock[T]) Acquire(id T) {
	// Grab global token if needed.
	if l.maxParallel > 0 {
		l.globalTokens <- struct{}{}
	}

	// Bump the reference count for that ID.
	l.lockMutex.Lock()
	lockState, ok := l.locks[id]
	if !ok {
		lockState = &idLockState{mu: &sync.Mutex{}}
		l.locks[id] = lockState
	}
	lockState.refs++
	l.lockMutex.Unlock()

	// Actually lock the ID’s mutex (blocks until available).
	lockState.mu.Lock()
}

// Release frees the per-ID lock and returns a global token (if any).
func (l *IDLock[T]) Release(id T) {
	l.lockMutex.Lock()
	lockState, ok := l.locks[id]
	if ok {
		// Unlock the ID’s mutex
		lockState.mu.Unlock()

		// Decrement the reference count
		lockState.refs--
		if lockState.refs == 0 {
			delete(l.locks, id)
		}
	}
	l.lockMutex.Unlock()

	if l.maxParallel > 0 {
		<-l.globalTokens
	}
}

// AcquireWithTimeout tries to do Acquire within timeout. Returns true if successful, false otherwise.
func (l *IDLock[T]) AcquireWithTimeout(id T, timeout time.Duration) bool {
	done := make(chan struct{})

	// We do the normal Acquire in a goroutine. If it finishes, it signals 'done'.
	go func() {
		l.Acquire(id)
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return true
	case <-timer.C:
		// We timed out. The Acquire goroutine might still be blocked on the ID lock (or global token).
		// But we won’t call Release because we never actually succeeded in Acquire.
		return false
	}
}

// TryRunWithTimeout tries to lock within lockTimeout, then runs fn with a runTimeout limit.
// Returns true if lock acquired and false if not.
func (l *IDLock[T]) TryRunWithTimeout(id T, lockTimeout, runTimeout time.Duration, fn func()) bool {
	if !l.AcquireWithTimeout(id, lockTimeout) {
		return false // Didn’t get the lock in time
	}

	// We have the lock. Let’s run fn in a goroutine with a runTimeout context.
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			// Always release the lock once fn finishes or panics
			if r := recover(); r != nil {
				// Optional: log or handle the panic
			}
			l.Release(id)
		}()
		fn()
	}()

	select {
	case <-done:
		// The function finished in time
		return true
	case <-ctx.Done():
		// Timed out, but we did acquire the lock, so return true to indicate
		// we reached the function (even if it’s still running).
		return true
	}
}
