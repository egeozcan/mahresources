package application_context

import (
	"testing"
	"time"
)

// Finding 42: group compare flagged CREATED and UPDATED as changed while
// rendering identical values. Timestamps are compared at full precision but
// rendered to the minute, so any two distinct groups almost always reported at
// least these two phantom differences — "Groups differ | Core 2 changed" on a
// group and its own fresh clone.
func TestSameTimestampComparesAtRenderedPrecision(t *testing.T) {
	base := time.Date(2026, 7, 29, 11, 27, 3, 0, time.UTC)

	cases := []struct {
		name string
		a, b time.Time
		same bool
	}{
		{"identical", base, base, true},
		{"same minute, different seconds", base, base.Add(41 * time.Second), true},
		{"same minute, different nanoseconds", base, base.Add(7 * time.Nanosecond), true},
		{"different minute", base, base.Add(time.Minute), false},
		{"different day", base, base.AddDate(0, 0, 1), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameRenderedTimestamp(tc.a, tc.b); got != tc.same {
				t.Errorf("sameRenderedTimestamp(%v, %v) = %v, want %v.\n"+
					"Compare at the precision the page renders, or identical-looking values are flagged as changed.",
					tc.a, tc.b, got, tc.same)
			}
		})
	}
}
