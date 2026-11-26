package models

import (
	"errors"
	"testing"
	"time"
)

// With nothing able to grant the lock, the wait ends at its bound with
// errAdvisoryLockTimeout instead of polling forever; the caller logs and retries
// later with its connection released.
func TestBoundedAdvisoryLockWaitGivesUp(t *testing.T) {
	tries := 0
	start := time.Now()
	err := pollLockFor(func() (bool, error) { tries++; return false, nil }, time.Millisecond, 20*time.Millisecond)
	if !errors.Is(err, errAdvisoryLockTimeout) {
		t.Fatalf("err = %v, want errAdvisoryLockTimeout", err)
	}
	if tries < 2 || time.Since(start) > time.Second {
		t.Fatalf("tries %d after %s", tries, time.Since(start))
	}
}
