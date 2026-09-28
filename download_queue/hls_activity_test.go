package download_queue

import (
	"testing"
	"time"

	"mahresources/jobs"
)

func TestHLSActivityWindowKeepsTheReadTimeAcrossCoalescing(t *testing.T) {
	start := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	var window hlsActivityWindow

	window.record(start)
	if got := window.take(start.Add(jobs.ProgressRateFreshFor)); !got.Equal(start) {
		t.Fatalf("fresh coalesced read at the deadline = %v; want original read time %v", got, start)
	}

	// A later completion/key callback may flush a pending notification, but it
	// must not make an old segment read fresh from the time it was mirrored.
	delayedRead := start.Add(time.Second)
	window.record(delayedRead)
	if got := window.take(delayedRead.Add(jobs.ProgressRateFreshFor + time.Nanosecond)); !got.IsZero() {
		t.Fatalf("a delayed callback rebased stale segment activity to %v", got)
	}

	window.record(start.Add(2 * time.Second))
	window.reset() // rendition/phase boundary
	if got := window.take(start.Add(2 * time.Second)); !got.IsZero() {
		t.Fatalf("activity crossed a phase boundary: %v", got)
	}
}
