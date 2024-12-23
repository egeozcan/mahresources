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
	lock := NewIDLock[string](0)
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
		t.Errorf("Expected counter = %d, got %d", iterations, counter)
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
		t.Errorf("Expected maxActive <= %d, got %d", maxParallel, maxActive)
	}
}

func TestTryRunWithTimeout_LockAcquisitionTimeout(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	lock.Acquire(id)
	defer lock.Release(id)

	// Another goroutine tries to get the same ID lock with a short lockTimeout
	success := lock.TryRunWithTimeout(id, 100*time.Millisecond, 1*time.Second, func() {
		t.Error("Function shouldn't have run")
	})

	if success {
		t.Error("Expected false: lock acquisition should have timed out")
	}
}

func TestTryRunWithTimeout_RunTimeout(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	// We'll let the function exceed runTimeout
	success := lock.TryRunWithTimeout(id, 1*time.Second, 100*time.Millisecond, func() {
		time.Sleep(500 * time.Millisecond)
	})

	if !success {
		t.Error("Expected true (lock acquired), but got false")
	}
}

func TestTryRunWithTimeout_Success(t *testing.T) {
	lock := NewIDLock[string](0)
	id := "testID"

	success := lock.TryRunWithTimeout(id, 1*time.Second, 500*time.Millisecond, func() {
		time.Sleep(100 * time.Millisecond)
	})
	if !success {
		t.Error("Expected true, got false")
	}
}

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
			// We assume only 1 gets the lock, because the first to lock sleeps 600ms,
			// which is longer than the 500ms lockTimeout the others have.
			success := lock.TryRunWithTimeout(id, 500*time.Millisecond, 1*time.Second, func() {
				time.Sleep(600 * time.Millisecond)
			})
			results <- success
		}()
	}

	wg.Wait()
	close(results)

	successCount := 0
	for r := range results {
		if r {
			successCount++
		}
	}
	if successCount != 1 {
		t.Errorf("Expected 1 success, got %d", successCount)
	}
}

func TestTryRunWithTimeout_GlobalTokenTimeout(t *testing.T) {
	lock := NewIDLock[string](1)
	id := "testID"

	// Acquire the only global token
	lock.Acquire(id)
	defer lock.Release(id)

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

	// Should be released after panic
	success = lock.TryRunWithTimeout(id, 1*time.Second, 100*time.Millisecond, func() {})
	if !success {
		t.Error("Expected to succeed")
	}
}

func TestReleaseNonExistingLock(t *testing.T) {
	lock := NewIDLock[string](0)
	lock.Release("nonExistingID") // Should not panic
}
