package lib

import (
	"sync"
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

			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			activeCount--
			mu.Unlock()
			lock.Release(id)
		}()
	}

	wg.Wait()

	if maxActive > int32(maxParallel) {
		t.Errorf("Expected maxActive to be at most %d, got %d", maxParallel, maxActive)
	}
}

func TestTryRunWithTimeout_LockAcquisitionTimeout(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	// Acquire the lock so it’s not available
	lock.Acquire(id)
	defer lock.Release(id)

	// Try to Acquire with a short lock timeout
	success := lock.TryRunWithTimeout(id, 100*time.Millisecond, 1*time.Second, func() {
		t.Error("Function should not have been executed")
	})

	if success {
		t.Error("Expected TryRunWithTimeout to return false due to lock acquisition timeout")
	}
}

func TestTryRunWithTimeout_RunTimeout(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	// We'll let the function exceed its runTimeout
	success := lock.TryRunWithTimeout(id, 1*time.Second, 100*time.Millisecond, func() {
		time.Sleep(500 * time.Millisecond)
	})

	if !success {
		t.Error("Expected TryRunWithTimeout to return true (lock acquired), but got false")
	}
}

func TestTryRunWithTimeout_Success(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	success := lock.TryRunWithTimeout(id, 1*time.Second, 500*time.Millisecond, func() {
		time.Sleep(100 * time.Millisecond)
	})

	if !success {
		t.Error("Expected TryRunWithTimeout to return true, but got false")
	}
}

// We only want exactly one goroutine to succeed. We do this by sleeping
// longer than the total lockTimeout in the “work” function:
func TestTryRunWithTimeout_Concurrent(t *testing.T) {
	lock := NewIDLock[int](0)
	id := 1
	numRoutines := 10
	var wg sync.WaitGroup
	results := make(chan bool, numRoutines)

	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine tries to get the lock within 500ms
			// but the first one that gets it will sleep 600ms,
			// so others won't get a chance in time.
			success := lock.TryRunWithTimeout(id, 500*time.Millisecond, 1*time.Second, func() {
				time.Sleep(600 * time.Millisecond)
			})
			results <- success
		}()
	}

	wg.Wait()
	close(results)

	successCount := 0
	for result := range results {
		if result {
			successCount++
		}
	}

	if successCount != 1 {
		t.Errorf("Expected only 1 successful TryRunWithTimeout, got %d", successCount)
	}
}

func TestTryRunWithTimeout_GlobalTokenTimeout(t *testing.T) {
	lock := NewIDLock[string](1)
	id := "testID"

	// Acquire the lock (and thus the single global token).
	lock.Acquire(id)
	defer lock.Release(id)

	// Should fail because no global tokens left
	success := lock.TryRunWithTimeout(id, 100*time.Millisecond, 1*time.Second, func() {
		t.Error("Should not execute")
	})

	if success {
		t.Error("Expected TryRunWithTimeout to return false due to global token timeout")
	}
}

func TestTryRunWithTimeout_PanicRecovery(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	success := lock.TryRunWithTimeout(id, 1*time.Second, 500*time.Millisecond, func() {
		panic("Intentional panic for testing")
	})

	if !success {
		t.Error("Expected TryRunWithTimeout to return true (lock acquired), but got false")
	}

	// Confirm the lock is free now
	success = lock.TryRunWithTimeout(id, 1*time.Second, 100*time.Millisecond, func() {
		// do nothing
	})

	if !success {
		t.Error("Expected TryRunWithTimeout to succeed (lock should be free), but got false")
	}
}

func TestReleaseNonExistingLock(t *testing.T) {
	lock := NewIDLock[string](0)
	lock.Release("nonExistingID") // Should not panic
}
