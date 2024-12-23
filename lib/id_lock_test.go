package lib

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewIDLock(t *testing.T) {
	lock := NewIDLock[string](5)
	if lock == nil {
		t.Error("NewIDLock returned nil")
	}
	if lock.maxParallel != 5 {
		t.Errorf("Expected maxParallel to be 5, got %d", lock.maxParallel)
	}
}

func TestAcquireAndRelease(t *testing.T) {
	lock := NewIDLock[string](0) // No global limit
	id := "testID"

	// Basic acquire/release
	lock.Acquire(id)
	lock.Release(id)
}

func TestAcquireMultipleTimes(t *testing.T) {
	lock := NewIDLock[string](0)
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
	lock := NewIDLock[int](0)
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
	lock := NewIDLock[string](uint(maxParallel))
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
			activeCount++
			if activeCount > maxActive {
				maxActive = activeCount
			}
			mu.Unlock()

			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			activeCount--
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
	lock := NewIDLock[string](0)
	lock.Release("nonExistingID") // This triggers the "ok == false" branch, no panic
}

// Tests calling Release more times than Acquire
func TestDoubleRelease(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	lock.Acquire(id)
	lock.Release(id)
	// second release triggers the 'default:' case inside the select { case <-state.ch: ... }
	lock.Release(id)
}

// Test AcquireWithTimeout -> Zero Timeout => immediate fail
func TestAcquireWithTimeout_ZeroTimeout(t *testing.T) {
	lock := NewIDLock[string](0)
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
	lock := NewIDLock[string](0)
	id := "testID"

	ok := lock.AcquireWithTimeout(id, -1*time.Second)
	if ok {
		t.Error("Expected failure when timeout is negative")
	}
}

func TestAcquireWithTimeout_Success(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	ok := lock.AcquireWithTimeout(id, 100*time.Millisecond)
	if !ok {
		t.Error("Expected to acquire with 100ms timeout")
	}
	lock.Release(id)
}

func TestAcquireWithTimeout_TimesOutIfLocked(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	lock.Acquire(id)
	defer lock.Release(id)

	ok := lock.AcquireWithTimeout(id, 50*time.Millisecond)
	if ok {
		t.Error("Expected AcquireWithTimeout to fail because the ID is locked")
	}
}

// The test from before that ensures we get false if lock acquisition times out
func TestTryRunWithTimeout_LockAcquisitionTimeout(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	lock.Acquire(id)
	defer lock.Release(id)

	success := lock.TryRunWithTimeout(id, 50*time.Millisecond, 1*time.Second, func() {
		t.Error("Should not run")
	})
	if success {
		t.Error("Expected false due to lock acquisition timeout")
	}
}

func TestTryRunWithTimeout_RunTimeout(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	success := lock.TryRunWithTimeout(id, 1*time.Second, 50*time.Millisecond, func() {
		time.Sleep(200 * time.Millisecond)
	})
	if !success {
		t.Error("Expected true (lock acquired), but got false")
	}
}

func TestTryRunWithTimeout_Success(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	success := lock.TryRunWithTimeout(id, 1*time.Second, 500*time.Millisecond, func() {
		time.Sleep(10 * time.Millisecond)
	})
	if !success {
		t.Error("Expected success")
	}
}

// If the function runs too long, we still return true once it's timed out because we did acquire the lock
func TestTryRunWithTimeout_TimeoutButAcquired(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	started := false
	success := lock.TryRunWithTimeout(id, 200*time.Millisecond, 100*time.Millisecond, func() {
		started = true
		time.Sleep(300 * time.Millisecond) // exceeds runTimeout
	})
	if !success {
		t.Error("Expected true because we did acquire the lock, even though it timed out later")
	}
	if !started {
		t.Error("Expected the function to have started executing")
	}
}

// We only want exactly one goroutine to succeed. The first acquires the lock and
// holds it for 600ms. Others have a 300ms lockTimeout, so they fail.
func TestTryRunWithTimeout_Concurrent(t *testing.T) {
	lock := NewIDLock[int](0)
	id := 1
	const numRoutines = 5
	var wg sync.WaitGroup
	results := make(chan bool, numRoutines)

	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			success := lock.TryRunWithTimeout(id, 300*time.Millisecond, 1*time.Second, func() {
				time.Sleep(600 * time.Millisecond)
			})
			results <- success
		}()
	}

	wg.Wait()
	close(results)

	var successCount int
	for s := range results {
		if s {
			successCount++
		}
	}
	if successCount != 1 {
		t.Errorf("Expected exactly 1 success, got %d", successCount)
	}
}

func TestTryRunWithTimeout_GlobalTokenTimeout(t *testing.T) {
	lock := NewIDLock[string](1)
	id := "testID"

	// Acquire the single global token
	lock.Acquire(id)
	defer lock.Release(id)

	// Should fail because no global tokens left
	success := lock.TryRunWithTimeout(id, 100*time.Millisecond, 1*time.Second, func() {
		t.Error("Should not execute")
	})

	if success {
		t.Error("Expected false due to no global tokens")
	}
}

func TestTryRunWithTimeout_PanicRecovery(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	success := lock.TryRunWithTimeout(id, 1*time.Second, 500*time.Millisecond, func() {
		panic("Intentional panic")
	})
	if !success {
		t.Error("Expected true, got false")
	}

	// Confirm the lock is free now
	success = lock.TryRunWithTimeout(id, 1*time.Second, 100*time.Millisecond, func() {})
	if !success {
		t.Error("Expected to succeed")
	}
}

// This test attempts AcquireWithTimeout and normal Acquire concurrently
func TestAcquireWithTimeout_ConcurrentUsage(t *testing.T) {
	lock := NewIDLock[int](0)
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
