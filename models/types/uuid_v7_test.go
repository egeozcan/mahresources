package types

import (
	"regexp"
	"testing"
	"time"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUIDv7_Format(t *testing.T) {
	id := NewUUIDv7()
	if !uuidPattern.MatchString(id) {
		t.Fatalf("invalid UUID v7 format: %s", id)
	}
}

func TestNewUUIDv7_Unique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := NewUUIDv7()
		if seen[id] {
			t.Fatalf("duplicate UUID v7: %s", id)
		}
		seen[id] = true
	}
}

func TestNewUUIDv7_TimeSorted(t *testing.T) {
	// UUIDv7 time-ordering is only guaranteed across distinct milliseconds.
	// Within a millisecond the random bits dominate and can sort either way.
	// Sleep > 1 ms between generations to force distinct-millisecond timestamps.
	a := NewUUIDv7()
	time.Sleep(2 * time.Millisecond)
	b := NewUUIDv7()
	if a >= b {
		t.Fatalf("expected %s < %s (time-sorted across distinct milliseconds)", a, b)
	}
}
