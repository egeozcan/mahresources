package lib

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MockLogger is a simple logger for testing purposes.
type MockLogger struct {
	mu         sync.Mutex
	Logs       []string
	PrintfFunc func(format string, args ...interface{})
}

func (m *MockLogger) Printf(format string, args ...interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.PrintfFunc != nil {
		m.PrintfFunc(format, args...)
	} else {
		m.Logs = append(m.Logs, fmt.Sprintf(format, args...))
	}
}

func TestNewIDLock(t *testing.T) {
	lock := NewIDLock[string](5, nil)
	if lock == nil {
		t.Error("NewIDLock returned nil")
		return
	}
	if lock.maxParallel != 5 {
		t.Errorf("Expected maxParallel to be 5, got %d", lock.maxParallel)
	}
}

func TestAcquireAndRelease(t *testing.T) {
	lock := NewIDLock[string](0, nil) // No global limit
	id := "testID"

	// Basic acquire/release
	lock.Acquire(id)
	lock.Release(id)
}

func TestAcquireMultipleTimes(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "testID"

	lock.Acquire(id)
	lock.Release(id) // release after first acquire

	lock.Acquire(id)
	lock.Release(id) // release after second acquire

	// Acquire/Release again
	lock.Acquire(id)
	lock.Release(id)
}

func TestConcurrentAccessSameID(t *testing.T) {
	lock := NewIDLock[int](0, nil)
	id := 1
	iterations := 100
	var counter int
	var wg sync.WaitGroup

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lock.Acquire(id)
			counter++
			lock.Release(id)
		}()
	}
	wg.Wait()

	if counter != iterations {
		t.Errorf("Expected counter to be %d, got %d", iterations, counter)
	}
}

func TestMaxParallelLimit(t *testing.T) {
	maxParallel := 3
	lock := NewIDLock[string](uint(maxParallel), nil)
	id := "testID"
	var activeCount int32
	var maxActive int32
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 0; i < maxParallel*10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lock.Acquire(id)

			mu.Lock()
			atomic.AddInt32(&activeCount, 1)
			if activeCount > maxActive {
				maxActive = activeCount
			}
			mu.Unlock()

			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			atomic.AddInt32(&activeCount, -1)
			mu.Unlock()

			lock.Release(id)
		}()
	}

	wg.Wait()
	if maxActive > int32(maxParallel) {
		t.Errorf("Expected maxActive <= %d, got %d", maxParallel, maxActive)
	}
}

// Tests releasing an ID that was never acquired, hitting the default case in 'Release'.
func TestReleaseNonExistingLock(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	lock.Release("nonExistingID") // This triggers the "ok == false" branch, no panic
}

// Tests calling Release more times than Acquire
func TestDoubleRelease(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "testID"

	lock.Acquire(id)
	lock.Release(id)
	// second release triggers the 'default:' case inside the select { case <-state.ch: ... }
	lock.Release(id)
}

func TestAcquireWithTimeout_ZeroTimeout(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "testID"

	// 1) Lock is free, so zero-timeout should succeed
	ok := lock.AcquireWithTimeout(id, 0)
	if !ok {
		t.Error("Expected success if the lock is immediately available at zero timeout")
	}

	// Remember to release before acquiring again!
	lock.Release(id)

	// 2) Now Acquire for real (blocks) and hold the lock
	lock.Acquire(id)
	// 3) Another zero-timeout AcquireWithTimeout should fail, because the lock is taken
	ok = lock.AcquireWithTimeout(id, 0)
	if ok {
		t.Error("Expected failure when ID is locked and zero-timeout is used")
	}

	// Finally, release after the test is done
	lock.Release(id)
}

// Negative timeout also yields immediate fail
func TestAcquireWithTimeout_NegativeTimeout(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "testID"

	ok := lock.AcquireWithTimeout(id, -1*time.Second)
	if ok {
		t.Error("Expected failure when timeout is negative")
	}
}

func TestAcquireWithTimeout_Success(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "testID"

	ok := lock.AcquireWithTimeout(id, 100*time.Millisecond)
	if !ok {
		t.Error("Expected to acquire with 100ms timeout")
	}
	lock.Release(id)
}

func TestAcquireWithTimeout_TimesOutIfLocked(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "testID"

	lock.Acquire(id)
	defer lock.Release(id)

	ok := lock.AcquireWithTimeout(id, 50*time.Millisecond)
	if ok {
		t.Error("Expected AcquireWithTimeout to fail because the ID is locked")
	}
}

// The test from before that ensures we get false if lock acquisition times out
func TestRunWithLockTimeout_LockAcquisitionTimeout(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "testID"

	lock.Acquire(id)
	defer lock.Release(id)

	success, err := lock.RunWithLockTimeout(id, 50*time.Millisecond, 1*time.Second, func() error {
		t.Error("Should not run")
		return nil
	})
	if success || err != nil {
		t.Error("Expected false due to lock acquisition timeout")
	}
}

func TestRunWithLockTimeout_RunTimeout(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "testID"

	success, err := lock.RunWithLockTimeout(id, 1*time.Second, 50*time.Millisecond, func() error {
		time.Sleep(200 * time.Millisecond)
		return nil
	})
	if !success || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected true (lock acquired) and DeadlineExceeded error but got %v, %v", success, err)
	}
}

func TestRunWithLockTimeout_Success(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "testID"

	success, err := lock.RunWithLockTimeout(id, 1*time.Second, 500*time.Millisecond, func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	if !success || err != nil {
		t.Errorf("Expected success but got %v, %v", success, err)
	}
}

// If the function runs too long, we still return true once it's timed out because we did acquire the lock
func TestRunWithLockTimeout_TimeoutButAcquired(t *testing.T) {
	// t.Parallel() // Avoid parallel if modifying shared state like this without atomic/channels
	lock := NewIDLock[string](0, nil)
	id := "testIDTimeoutButAcquired" // Use distinct ID

	var mu sync.Mutex // Mutex to protect 'started'
	started := false  // Shared variable

	success, err := lock.RunWithLockTimeout(id, 200*time.Millisecond, 100*time.Millisecond, func() error {
		mu.Lock()
		started = true // Write under lock
		mu.Unlock()
		time.Sleep(300 * time.Millisecond) // exceeds runTimeout
		return nil
	})

	if !success || !errors.Is(err, context.DeadlineExceeded) {
		// Use Fatalf as the test state is invalid if this fails
		t.Fatalf("Expected true (lock acquired), and DeadlineExceeded error but got: success=%v, err=%v", success, err)
	}

	// Read the value under the lock to ensure visibility
	mu.Lock()
	localStarted := started
	mu.Unlock()

	if !localStarted {
		t.Error("Expected the function to have started executing")
	}
}

func TestRunWithLockTimeout_Concurrent(t *testing.T) {
	lock := NewIDLock[int](0, nil)
	id := 1
	const numRoutines = 5
	var wg sync.WaitGroup
	var successCount int32
	var mu sync.Mutex

	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			success, err := lock.RunWithLockTimeout(id, 100*time.Millisecond, 1*time.Second, func() error {
				mu.Lock()
				successCount++
				mu.Unlock()
				time.Sleep(200 * time.Millisecond)
				return nil
			})
			if success && err == nil {
				t.Log("Goroutine successfully acquired lock and ran")
			} else {
				t.Log("Goroutine failed to acquire lock within timeout")
			}
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("Expected exactly 1 success, got %d", successCount)
	}
}

func TestRunWithLockTimeout_GlobalTokenTimeout(t *testing.T) {
	lock := NewIDLock[string](1, nil)
	id := "testID"

	// Acquire the single global token
	lock.Acquire(id)
	defer lock.Release(id)

	// Should fail because no global tokens left
	success, err := lock.RunWithLockTimeout(id, 100*time.Millisecond, 1*time.Second, func() error {
		t.Error("Should not execute")
		return nil
	})

	if success || err != nil {
		t.Error("Expected false due to no global tokens")
	}
}

func TestRunWithLockTimeout_PanicRecovery(t *testing.T) {
	// t.Parallel() // This test modifies shared state (logger), maybe avoid parallel if using mock assertions heavily
	mockLogger := &MockLogger{} // Use mock logger to check output if needed
	lock := NewIDLock[string](0, mockLogger)
	id := "testIDPanicRecovery" // Use a distinct ID
	panicMsg := "Intentional panic"
	expectedErrStr := fmt.Sprintf("panic in locked function: %v", panicMsg)

	// --- First execution that panics ---
	success, err := lock.RunWithLockTimeout(id, 1*time.Second, 500*time.Millisecond, func() error {
		panic(panicMsg)
	})
	if !success {
		t.Errorf("Expected success=true (lock acquired) even if function panicked")
	}
	// Updated check for the new error format
	if err == nil || err.Error() != expectedErrStr {
		t.Fatalf("Expected panic error '%s', got: %v", expectedErrStr, err) // Use Fatalf to stop if basic check fails
	}

	// Check logger output
	foundLog := false
	mockLogger.mu.Lock() // Lock the mock logger for reading
	logs := make([]string, len(mockLogger.Logs))
	copy(logs, mockLogger.Logs)
	mockLogger.mu.Unlock()

	for _, logMsg := range logs {
		if strings.Contains(logMsg, panicMsg) && strings.Contains(logMsg, id) {
			foundLog = true
			break
		}
	}
	if !foundLog {
		t.Errorf("Expected log message containing panic info for ID '%s'", id)
	}

	// --- Second execution to confirm lock release ---
	// Confirm the lock is free now by trying RunWithLockTimeout again
	var ranAfter bool
	successAfter, errAfter := lock.RunWithLockTimeout(id, 100*time.Millisecond, 100*time.Millisecond, func() error {
		ranAfter = true
		return nil
	})
	if !successAfter || errAfter != nil {
		t.Errorf("Expected RunWithLockTimeout to succeed after panic recovery but got success=%v, err=%v", successAfter, errAfter)
	}
	if !ranAfter {
		t.Errorf("Expected function to run in the second RunWithLockTimeout call after panic")
	}
}

// This test attempts AcquireWithTimeout and normal Acquire concurrently
func TestAcquireWithTimeout_ConcurrentUsage(t *testing.T) {
	lock := NewIDLock[int](0, nil)
	id := 42
	var acquiredCount int32

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if lock.AcquireWithTimeout(id, 50*time.Millisecond) {
				atomic.AddInt32(&acquiredCount, 1)
				lock.Release(id)
			}
		}()
		go func() {
			defer wg.Done()
			lock.Acquire(id)
			atomic.AddInt32(&acquiredCount, 1)
			lock.Release(id)
		}()
	}
	wg.Wait()

	if acquiredCount < 10 {
		t.Errorf("Expected at least 10 total acquisitions, got %d", acquiredCount)
	}
}

// TestAcquireContext_Success ensures we can acquire a lock with enough time.
func TestAcquireContext_Success(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "contextTestID"

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := lock.AcquireContext(ctx, id); err != nil {
		t.Errorf("Expected to acquire lock within 1s, got error: %v", err)
	}
	lock.Release(id)
}

// TestAcquireContext_Canceled ensures that if our context is canceled, we fail to acquire.
func TestAcquireContext_Canceled(t *testing.T) {
	t.Parallel()
	lock := NewIDLock[string](0, nil)
	id := "contextCanceledIDOriginal" // Use a distinct ID

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	// This call should now reliably return an error because of the fix
	if err := lock.AcquireContext(ctx, id); !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled due to immediate context cancellation, got: %v", err)
	}

	// Verify internal state cleanup
	lock.mu.Lock()
	_, exists := lock.locks[id]
	lock.mu.Unlock()
	if exists {
		t.Errorf("Lock state should not exist after failed acquire with canceled context")
	}
}

// TestAcquireContext_Timeout ensures that if our context times out, AcquireContext fails.
func TestAcquireContext_Timeout(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "contextTimeoutID"

	// Acquire first so the ID is locked
	lock.Acquire(id)
	defer lock.Release(id)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := lock.AcquireContext(ctx, id); err == nil {
		t.Errorf("Expected context deadline exceeded, got nil")
	}
}

// TestAcquireContext_GlobalLimit ensures global tokens are respected by AcquireContext.
func TestAcquireContext_GlobalLimit(t *testing.T) {
	mockLogger := &MockLogger{}
	lock := NewIDLock[string](1, mockLogger)
	id := "globalLimitID"

	// Acquire one so global limit is used up
	lock.Acquire(id)
	defer lock.Release(id)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := lock.AcquireContext(ctx, "anotherID"); err == nil {
		t.Errorf("Expected context deadline exceeded due to no global tokens, got nil")
	}
}

// TestAcquireContext_AlreadyCanceled_LockFree verifies fix for race condition
// where context is canceled *before* calling AcquireContext and the lock is free.
func TestAcquireContext_AlreadyCanceled_LockFree(t *testing.T) {
	t.Parallel()                      // Mark as parallelizable
	lock := NewIDLock[string](0, nil) // No global limit needed for this specific race
	id := "alreadyCanceledFree"

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := lock.AcquireContext(ctx, id)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled when acquiring with already canceled context (lock free), got: %v", err)
	}

	// Verify internal state: refs should be 0 and map entry deleted
	lock.mu.Lock()
	state, exists := lock.locks[id]
	if exists {
		t.Errorf("Lock state for ID '%s' should not exist after failed acquire, but found state with refs: %d", id, state.refs)
	}
	lock.mu.Unlock()

	// Verify lock is actually free by acquiring normally
	acquired := lock.AcquireWithTimeout(id, 0)
	if !acquired {
		t.Errorf("Lock should be free after failed acquire with canceled context, but AcquireWithTimeout(0) failed")
	} else {
		lock.Release(id)
	}
}

// TestAcquireContext_AlreadyCanceled_LockFree_GlobalLimit does the same check
// but with a global limit active, ensuring the fix works with global tokens.
func TestAcquireContext_AlreadyCanceled_LockFree_GlobalLimit(t *testing.T) {
	t.Parallel()
	lock := NewIDLock[string](1, nil) // With global limit
	id := "alreadyCanceledFreeGlobal"

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := lock.AcquireContext(ctx, id)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled when acquiring with already canceled context (global limit, lock free), got: %v", err)
	}

	// Verify internal state: refs should be 0 and map entry deleted
	lock.mu.Lock()
	state, exists := lock.locks[id]
	if exists {
		t.Errorf("Lock state for ID '%s' should not exist after failed acquire (global limit), but found state with refs: %d", id, state.refs)
	}
	// Verify global token count (should be 0 used, buffer capacity 1)
	if len(lock.globalTokens) != 0 {
		t.Errorf("Expected 0 global tokens in use after failed acquire, got %d", len(lock.globalTokens))
	}
	lock.mu.Unlock()

	// Verify lock is actually free by acquiring normally
	acquired := lock.AcquireWithTimeout(id, 0)
	if !acquired {
		t.Errorf("Lock should be free after failed acquire with canceled context (global limit), but AcquireWithTimeout(0) failed")
	} else {
		lock.Release(id)
	}
}

// TestRunWithLockTimeout_HoldsLockUntilFnCompletes verifies that when fn exceeds
// runTimeout, the lock is NOT released until fn actually finishes.
func TestRunWithLockTimeout_HoldsLockUntilFnCompletes(t *testing.T) {
	lock := NewIDLock[string](0, nil)
	id := "testHoldsLock"

	fnStarted := make(chan struct{})
	fnDone := make(chan struct{})

	// Start RunWithLockTimeout with a short runTimeout but long fn
	go func() {
		lock.RunWithLockTimeout(id, 1*time.Second, 50*time.Millisecond, func() error {
			close(fnStarted)
			time.Sleep(300 * time.Millisecond) // Exceeds runTimeout
			close(fnDone)
			return nil
		})
	}()

	<-fnStarted
	// fn is running and runTimeout will expire. Try to acquire the same lock.
	// It should NOT succeed until fn finishes (fnDone closes).
	acquired := lock.AcquireWithTimeout(id, 100*time.Millisecond)
	if acquired {
		lock.Release(id)
		t.Fatal("Lock was acquired while fn was still running — mutual exclusion violated")
	}

	// Now wait for fn to complete, then the lock should be available
	<-fnDone
	time.Sleep(50 * time.Millisecond) // Give time for Release to execute

	acquired = lock.AcquireWithTimeout(id, 500*time.Millisecond)
	if !acquired {
		t.Fatal("Lock should be free after fn completes")
	}
	lock.Release(id)
}

// TestAcquireContext_AlreadyCanceled_LockHeld verifies behavior when context
// is canceled *before* the call, but the lock is already held by someone else.
func TestAcquireContext_AlreadyCanceled_LockHeld(t *testing.T) {
	t.Parallel()
	lock := NewIDLock[string](0, nil)
	id := "alreadyCanceledHeld"

	// Acquire the lock first
	lock.Acquire(id)
	defer lock.Release(id) // Ensure release even on test failure

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Attempt to acquire with the canceled context
	err := lock.AcquireContext(ctx, id)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled when acquiring with already canceled context (lock held), got: %v", err)
	}

	// Verify internal state: The original holder's ref should still be there
	lock.mu.Lock()
	state, exists := lock.locks[id]
	if !exists || state.refs != 1 {
		t.Errorf("Lock state for ID '%s' should still exist with 1 ref after failed acquire attempt, but exists=%v, refs=%d", id, exists, state.refs)
	}
	lock.mu.Unlock()
}
