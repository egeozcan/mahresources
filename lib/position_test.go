// lib/position_test.go
package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPositionBetween(t *testing.T) {
	tests := []struct {
		name   string
		before string
		after  string
	}{
		{"empty_empty", "", ""},
		{"empty_n", "", "n"},
		{"n_empty", "n", ""},
		{"a_c", "a", "c"},
		{"a_b", "a", "b"},
		{"an_b", "an", "b"},
		{"aaa_aab", "aaa", "aab"},
		{"a_z", "a", "z"},
		{"y_z", "y", "z"},
		{"za_zz", "za", "zz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PositionBetween(tt.before, tt.after)
			if tt.before != "" {
				assert.True(t, result > tt.before, "result %q should be > before %q", result, tt.before)
			}
			if tt.after != "" {
				assert.True(t, result < tt.after, "result %q should be < after %q", result, tt.after)
			}
			// Verify result is non-empty
			assert.NotEmpty(t, result)
		})
	}
}

func TestPositionBetween_SpecificValues(t *testing.T) {
	// Test that empty,empty returns the middle
	assert.Equal(t, "n", PositionBetween("", ""))
}

func TestFirstPosition(t *testing.T) {
	assert.Equal(t, "n", FirstPosition())
}

func TestPositionBetween_Sequence(t *testing.T) {
	// Test that we can generate a sequence of positions
	positions := make([]string, 10)
	positions[0] = FirstPosition()

	// Add items after
	for i := 1; i < 5; i++ {
		positions[i] = PositionBetween(positions[i-1], "")
		assert.True(t, positions[i] > positions[i-1], "position %d (%q) should be > position %d (%q)", i, positions[i], i-1, positions[i-1])
	}

	// Add items before
	for i := 5; i < 10; i++ {
		positions[i] = PositionBetween("", positions[0])
		assert.True(t, positions[i] < positions[0], "position %d (%q) should be < position 0 (%q)", i, positions[i], positions[0])
	}
}

func TestPositionBetween_InsertBetween(t *testing.T) {
	// Start with two positions
	first := "a"
	last := "z"

	// Insert multiple items between them
	var positions []string
	positions = append(positions, first)

	current := first
	for i := 0; i < 20; i++ {
		next := PositionBetween(current, last)
		assert.True(t, next > current, "next %q should be > current %q", next, current)
		assert.True(t, next < last, "next %q should be < last %q", next, last)
		positions = append(positions, next)
		current = next
	}

	// Verify all positions are in order
	for i := 1; i < len(positions); i++ {
		assert.True(t, positions[i] > positions[i-1], "position %d (%q) should be > position %d (%q)", i, positions[i], i-1, positions[i-1])
	}
}
