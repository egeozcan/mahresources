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
	lock := NewIDLock[string](0, nil)
	id := "testID"

	started := false
	success, err := lock.RunWithLockTimeout(id, 200*time.Millisecond, 100*time.Millisecond, func() error {
		started = true
		time.Sleep(300 * time.Millisecond) // exceeds runTimeout
		return nil
	})
	if !success || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected true (lock acquired), and DeadlineExceeded error but got: %v, %v", success, err)
	}
	if !started {
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
	lock := NewIDLock[string](0, nil)
	id := "testID"

	success, err := lock.RunWithLockTimeout(id, 1*time.Second, 500*time.Millisecond, func() error {
		panic("Intentional panic")
	})
	if !success {
		t.Error("Expected true (lock acquired) even if function panicked")
	}
	if err == nil || !strings.Contains(err.Error(), "panic: Intentional panic") {
		t.Errorf("Expected panic error, got: %v", err)
	}

	// Confirm the lock is free now
	success, err = lock.RunWithLockTimeout(id, 1*time.Second, 100*time.Millisecond, func() error {
		return nil
	})
	if !success || err != nil {
		t.Errorf("Expected to succeed but got %v, %v", success, err)
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
	lock := NewIDLock[string](0, nil)
	id := "contextCanceledID"

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	if err := lock.AcquireContext(ctx, id); err == nil {
		t.Errorf("Expected error due to context cancellation, got nil")
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
