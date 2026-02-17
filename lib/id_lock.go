package lib

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// idLockState holds a channel of capacity 1 plus a reference counter.
type idLockState struct {
	ch   chan struct{} // Channel used as a semaphore for the specific ID
	refs int           // Number of goroutines currently holding or waiting for this ID lock
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
// It ensures that only one operation per ID can run at a time,
// and optionally limits the total number of concurrent operations across all IDs.
type IDLock[T comparable] struct {
	mu           sync.Mutex         // Protects access to the 'locks' map
	locks        map[T]*idLockState // Map from ID to its lock state
	maxParallel  uint               // Maximum global concurrency (0 means unlimited)
	globalTokens chan struct{}      // Semaphore for enforcing global concurrency limit
	log          logger             // Logger for warnings or errors
}

// NewIDLock returns an IDLock with an optional global concurrency limit.
func NewIDLock[T comparable](maxParallel uint, log logger) *IDLock[T] {
	if log == nil {
		log = stdLogger{} // Use default logger if none provided
	}
	// Create the global token channel with buffer size maxParallel.
	// If maxParallel is 0, the channel remains nil, simplifying checks later.
	var globalTokens chan struct{}
	if maxParallel > 0 {
		globalTokens = make(chan struct{}, maxParallel)
	}

	return &IDLock[T]{
		locks:        make(map[T]*idLockState),
		maxParallel:  maxParallel,
		globalTokens: globalTokens, // Will be nil if maxParallel is 0
		log:          log,
	}
}

// releaseGlobalToken tries to release one global token.
// It's safe to call even if maxParallel is 0 or no token was held.
func (l *IDLock[T]) releaseGlobalToken() {
	// Only attempt to release if a global limit exists (globalTokens is not nil)
	if l.globalTokens != nil {
		select {
		case <-l.globalTokens:
			// Token successfully released.
		default:
			// See previous comments - warning removed.
		}
	}
}

// Acquire blocks until it acquires the lock for the given ID.
// If maxParallel > 0, it also blocks until a global token is available.
func (l *IDLock[T]) Acquire(id T) {
	// 1. Acquire global token if needed (blocks if limit reached)
	if l.globalTokens != nil {
		l.globalTokens <- struct{}{} // Block until a global token is free
	}

	// 2. Get or create the state for the ID, increment refs under mutex
	l.mu.Lock()
	state, ok := l.locks[id]
	if !ok {
		state = &idLockState{
			ch:   make(chan struct{}, 1), // Capacity 1 allows one holder
			refs: 0,
		}
		l.locks[id] = state
	}
	state.refs++ // Increment ref count (represents this waiting/holding goroutine)
	l.mu.Unlock()

	// 3. Acquire the ID-specific lock (blocks if already held)
	state.ch <- struct{}{}
}

// AcquireContext attempts to acquire the lock for the given ID, respecting the context.
// Returns nil on success, or the context error if the context is cancelled or times out
// before the lock (both global and ID-specific) can be acquired.
func (l *IDLock[T]) AcquireContext(ctx context.Context, id T) error {
	// --- Initial Context Check ---
	// Immediately return if context is already cancelled. Avoids unnecessary work.
	if err := ctx.Err(); err != nil {
		return err
	}

	// 1. Acquire global token *with context* if needed
	acquiredGlobal := false // Track if we acquired a global token
	if l.globalTokens != nil {
		select {
		case l.globalTokens <- struct{}{}:
			acquiredGlobal = true // Mark that we hold a global token
		case <-ctx.Done():
			return ctx.Err() // Failed to get global token due to context
		}
		// Re-check context after potentially waiting for the global token.
		// If cancelled while waiting, release the token we just acquired.
		if err := ctx.Err(); err != nil {
			// acquiredGlobal must be true here if we entered this block
			l.releaseGlobalToken()
			return err
		}
	}

	// 2. Get or create the state for the ID, increment refs under mutex
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

	// 3. Try to acquire the ID-specific lock *with context*
	select {
	case state.ch <- struct{}{}:
		// Acquired ID lock. Now, double-check if the context was cancelled *just* before
		// or during the (potentially non-blocking) send operation. This handles the
		// race condition where <-ctx.Done() and the send are simultaneously ready.
		if err := ctx.Err(); err != nil {
			// Context is done. We must revert the acquisition.
			<-state.ch // Immediately release the ID lock we just spuriously acquired.

			// Clean up state and potentially global token
			l.mu.Lock()
			state.refs--
			if state.refs == 0 {
				delete(l.locks, id)
			}
			l.mu.Unlock()

			if acquiredGlobal { // Release global token only if we acquired one
				l.releaseGlobalToken()
			}
			return err // Return the context error
		}
		// Success: Lock acquired and context is still valid.
		return nil
	case <-ctx.Done():
		// Failed to acquire ID lock due to context. Clean up state and potentially global token.
		l.mu.Lock()
		state.refs--
		if state.refs == 0 {
			delete(l.locks, id)
		}
		l.mu.Unlock()

		if acquiredGlobal { // Release global token only if we acquired one
			l.releaseGlobalToken()
		}
		return ctx.Err() // Return the context error
	}
}

// Release unlocks the lock for the given ID and releases a global token if applicable.
// It's crucial that Release is called exactly once for every successful Acquire call.
func (l *IDLock[T]) Release(id T) {
	l.mu.Lock()
	state, ok := l.locks[id]
	if !ok {
		l.mu.Unlock()
		l.log.Printf("IDLock.Release: Attempted to release lock for ID '%v' which has no active state.\n", id)
		return
	}

	var releasedIDLock bool
	select {
	case <-state.ch:
		// Successfully received from the channel, meaning the lock was held by the caller.
		releasedIDLock = true
		state.refs-- // Decrement ref count as this goroutine is no longer holding/waiting.
		if state.refs == 0 {
			delete(l.locks, id)
		}
	default:
		l.log.Printf("IDLock.Release: Attempted to release lock for ID '%v' which was not held (double release?)\n", id)
	}
	l.mu.Unlock() // Unlock before potentially blocking on global token release

	// Only release a global token if we successfully released the ID lock.
	if releasedIDLock {
		l.releaseGlobalToken()
	}
}

// AcquireWithTimeout attempts to acquire the lock for the given ID within the specified timeout.
// Returns true if the lock (both global and ID-specific) was acquired, false otherwise.
// A timeout of 0 attempts to acquire immediately without blocking.
// A negative timeout always returns false.
func (l *IDLock[T]) AcquireWithTimeout(id T, timeout time.Duration) bool {
	if timeout < 0 {
		return false // Negative timeout is invalid
	}

	// --- Handle Zero Timeout (Non-blocking attempt) ---
	if timeout == 0 {
		// Check context just in case background context is already cancelled? Generally not needed for timeout=0.
		// We can directly try non-blocking operations.

		// 1. Try acquiring global token non-blockingly
		acquiredGlobal := false
		if l.globalTokens != nil {
			select {
			case l.globalTokens <- struct{}{}:
				acquiredGlobal = true // Got global token
			default:
				return false // Failed to get global token immediately
			}
		}

		// 2. Manage state and try acquiring ID lock non-blockingly
		l.mu.Lock()
		state, ok := l.locks[id]
		if !ok {
			state = &idLockState{ch: make(chan struct{}, 1), refs: 0}
			l.locks[id] = state
		}
		state.refs++
		l.mu.Unlock()

		select {
		case state.ch <- struct{}{}:
			// Acquired ID lock immediately
			return true
		default:
			// Failed to acquire ID lock immediately, must revert state changes and release global token
			l.mu.Lock()
			state.refs--
			if state.refs == 0 {
				delete(l.locks, id)
			}
			l.mu.Unlock()

			if acquiredGlobal { // Release the global token acquired earlier only if we got one
				l.releaseGlobalToken()
			}
			return false
		}
	}

	// --- Handle Positive Timeout ---
	// Create a context that governs the entire acquisition attempt (global + ID lock)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel() // Ensure context resources are cleaned up

	// Use the robust AcquireContext logic with the derived context
	err := l.AcquireContext(ctx, id)

	// Return true if successful (err is nil), false otherwise
	return err == nil
}

// RunWithLockTimeout tries to acquire the lock for the ID within 'lockTimeout'.
// If successful, it runs the function 'fn' with a separate 'runTimeout'.
// It returns (true, nil) if the lock was acquired and 'fn' completed successfully.
// It returns (true, error) if the lock was acquired but 'fn' returned an error or timed out (context.DeadlineExceeded).
// It returns (false, nil) if the lock could not be acquired within 'lockTimeout'.
func (l *IDLock[T]) RunWithLockTimeout(id T, lockTimeout, runTimeout time.Duration, fn func() error) (lockAcquired bool, err error) {
	// Try to acquire the lock using the robust AcquireWithTimeout
	acquired := l.AcquireWithTimeout(id, lockTimeout)
	if !acquired {
		return false, nil // Lock not acquired within the specified timeout
	}
	// Use defer to ensure Release is always called if the lock was acquired
	defer l.Release(id)

	// Lock acquired, now prepare to run the function with its own timeout
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel() // Ensure run context resources are cleaned up

	errChan := make(chan error, 1) // Buffered channel to receive result from fn

	go func() {
		// Defer panic recovery within the goroutine
		defer func() {
			if r := recover(); r != nil {
				l.log.Printf("IDLock.RunWithLockTimeout: Recovered panic in function for ID '%v': %v\n", id, r)
				// Send a specific error indicating a panic occurred
				errChan <- fmt.Errorf("panic in locked function: %v", r)
			}
		}()
		// Execute the user function and send its result (or nil) to the channel
		errChan <- fn()
	}()

	// Wait for the function to complete, or for the runTimeout context to expire
	select {
	case <-ctx.Done():
		// Timeout exceeded. Wait for fn to finish before releasing the lock.
		<-errChan
		return true, context.DeadlineExceeded // Lock was acquired, but run timed out
	case runErr := <-errChan:
		// The function completed (or panicked)
		return true, runErr // Lock was acquired, return the function's result/error
	}
}
