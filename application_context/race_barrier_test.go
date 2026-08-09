package application_context

import (
	"sync"
	"testing"
	"time"
)

// Concurrency tests in this package coordinate goroutines through GORM callback
// barriers. Every wait goes through these helpers rather than a bare channel
// receive: when a barrier stops matching — a reformatted SQL statement, a new
// early return that skips the instrumented call — a bare receive parks forever
// and `go test` eventually aborts the whole package with a timeout panic,
// destroying every other test's result. A bounded wait fails one test and names
// the barrier that never fired.

const (
	raceBarrierWait    = 5 * time.Second
	raceBlockedDwell   = 150 * time.Millisecond
	raceUnblockedLimit = 3 * time.Second
)

func waitBarrier(t *testing.T, barrier <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-barrier:
	case <-time.After(raceBarrierWait):
		t.Fatal(message)
	}
}

// assertStillBlocked proves serialization actually happened instead of merely
// being scheduled. Signalling that an operation has *attempted* a lock says
// nothing about who won; only observing that it cannot finish while the lock is
// held elsewhere does. Without this the surrounding test passes whenever the
// racing operations happen to complete in the intended order, which is exactly
// how a guard silently stops guarding.
func assertStillBlocked(t *testing.T, result <-chan error, message string) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("%s (it completed, err=%v)", message, err)
	case <-time.After(raceBlockedDwell):
	}
}

// assertCompletes is the converse: an operation that must not be blocked has to
// finish promptly and cleanly.
func assertCompletes(t *testing.T, result <-chan error, message string) {
	t.Helper()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("%s: %v", message, err)
		}
	case <-time.After(raceUnblockedLimit):
		t.Fatal(message + ": it is still blocked")
	}
}

// releaseBarrier returns an idempotent closer for a barrier's release channel and
// registers it for cleanup, so a t.Fatal on the test goroutine cannot leave a
// paused callback goroutine parked for the rest of the run.
func releaseBarrier(t *testing.T, release chan struct{}) func() {
	t.Helper()
	var once sync.Once
	closer := func() { once.Do(func() { close(release) }) }
	t.Cleanup(closer)
	return closer
}
