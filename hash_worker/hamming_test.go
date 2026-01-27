package hash_worker

import "testing"

func TestHammingDistance(t *testing.T) {
	tests := []struct {
		name     string
		a, b     uint64
		expected int
	}{
		{"identical", 0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF, 0},
		{"completely different", 0x0, 0xFFFFFFFFFFFFFFFF, 64},
		{"one bit different", 0x0, 0x1, 1},
		{"half bits different", 0xAAAAAAAAAAAAAAAA, 0x5555555555555555, 64},
		{"few bits different", 0xFFFFFFFFFFFFFFF0, 0xFFFFFFFFFFFFFFFF, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HammingDistance(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("HammingDistance(%x, %x) = %d, want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}
