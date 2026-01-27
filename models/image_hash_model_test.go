package models

import "testing"

func TestImageHash_GetDHash(t *testing.T) {
	tests := []struct {
		name     string
		hash     ImageHash
		expected uint64
	}{
		{
			name:     "returns uint64 value when DHashInt is set",
			hash:     ImageHash{ID: 1, DHashInt: ptr(uint64(0x1234567890abcdef))},
			expected: 0x1234567890abcdef,
		},
		{
			name:     "prefers DHashInt over DHash string",
			hash:     ImageHash{ID: 2, DHashInt: ptr(uint64(0x1111)), DHash: "2222"},
			expected: 0x1111,
		},
		{
			name:     "parses valid hex string when DHashInt is nil",
			hash:     ImageHash{ID: 3, DHash: "fedcba9876543210"},
			expected: 0xfedcba9876543210,
		},
		{
			name:     "returns 0 for empty string",
			hash:     ImageHash{ID: 4, DHash: ""},
			expected: 0,
		},
		{
			name:     "returns 0 for invalid hex string",
			hash:     ImageHash{ID: 5, DHash: "not-a-hex-value"},
			expected: 0,
		},
		{
			name:     "returns 0 for partially valid hex",
			hash:     ImageHash{ID: 6, DHash: "123xyz"},
			expected: 0,
		},
		{
			name:     "handles leading zeros",
			hash:     ImageHash{ID: 7, DHash: "0000000000000001"},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.hash.GetDHash()
			if result != tt.expected {
				t.Errorf("GetDHash() = %x, want %x", result, tt.expected)
			}
		})
	}
}

func TestImageHash_GetAHash(t *testing.T) {
	tests := []struct {
		name     string
		hash     ImageHash
		expected uint64
	}{
		{
			name:     "returns uint64 value when AHashInt is set",
			hash:     ImageHash{ID: 1, AHashInt: ptr(uint64(0xabcdef1234567890))},
			expected: 0xabcdef1234567890,
		},
		{
			name:     "prefers AHashInt over AHash string",
			hash:     ImageHash{ID: 2, AHashInt: ptr(uint64(0x3333)), AHash: "4444"},
			expected: 0x3333,
		},
		{
			name:     "parses valid hex string when AHashInt is nil",
			hash:     ImageHash{ID: 3, AHash: "0123456789abcdef"},
			expected: 0x0123456789abcdef,
		},
		{
			name:     "returns 0 for empty string",
			hash:     ImageHash{ID: 4, AHash: ""},
			expected: 0,
		},
		{
			name:     "returns 0 for invalid hex string",
			hash:     ImageHash{ID: 5, AHash: "invalid!@#$"},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.hash.GetAHash()
			if result != tt.expected {
				t.Errorf("GetAHash() = %x, want %x", result, tt.expected)
			}
		})
	}
}

func TestImageHash_IsMigrated(t *testing.T) {
	tests := []struct {
		name     string
		hash     ImageHash
		expected bool
	}{
		{
			name:     "returns true when DHashInt is set",
			hash:     ImageHash{DHashInt: ptr(uint64(123))},
			expected: true,
		},
		{
			name:     "returns false when DHashInt is nil",
			hash:     ImageHash{DHash: "abc"},
			expected: false,
		},
		{
			name:     "returns false for empty hash",
			hash:     ImageHash{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.hash.IsMigrated()
			if result != tt.expected {
				t.Errorf("IsMigrated() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// ptr is a helper to create pointers to values
func ptr[T any](v T) *T {
	return &v
}
