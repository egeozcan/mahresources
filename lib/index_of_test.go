package lib

import "testing"

func TestIndexOf_Int(t *testing.T) {
	tests := []struct {
		name       string
		collection []int
		element    int
		want       int
	}{
		{
			name:       "found at beginning",
			collection: []int{1, 2, 3},
			element:    1,
			want:       0,
		},
		{
			name:       "found in middle",
			collection: []int{1, 2, 3},
			element:    2,
			want:       1,
		},
		{
			name:       "found at end",
			collection: []int{1, 2, 3},
			element:    3,
			want:       2,
		},
		{
			name:       "not found",
			collection: []int{1, 2, 3},
			element:    5,
			want:       -1,
		},
		{
			name:       "empty slice",
			collection: []int{},
			element:    1,
			want:       -1,
		},
		{
			name:       "nil slice",
			collection: nil,
			element:    1,
			want:       -1,
		},
		{
			name:       "duplicates returns first occurrence",
			collection: []int{1, 2, 2, 3},
			element:    2,
			want:       1,
		},
		{
			name:       "single element found",
			collection: []int{42},
			element:    42,
			want:       0,
		},
		{
			name:       "single element not found",
			collection: []int{42},
			element:    1,
			want:       -1,
		},
		{
			name:       "negative numbers",
			collection: []int{-3, -2, -1, 0, 1},
			element:    -2,
			want:       1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IndexOf(tt.collection, tt.element)
			if got != tt.want {
				t.Errorf("IndexOf(%v, %d) = %d, want %d", tt.collection, tt.element, got, tt.want)
			}
		})
	}
}

func TestIndexOf_String(t *testing.T) {
	tests := []struct {
		name       string
		collection []string
		element    string
		want       int
	}{
		{
			name:       "found",
			collection: []string{"a", "b", "c"},
			element:    "b",
			want:       1,
		},
		{
			name:       "not found",
			collection: []string{"a", "b", "c"},
			element:    "d",
			want:       -1,
		},
		{
			name:       "empty slice",
			collection: []string{},
			element:    "a",
			want:       -1,
		},
		{
			name:       "empty string in slice",
			collection: []string{"a", "", "c"},
			element:    "",
			want:       1,
		},
		{
			name:       "case sensitive",
			collection: []string{"A", "B", "C"},
			element:    "a",
			want:       -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IndexOf(tt.collection, tt.element)
			if got != tt.want {
				t.Errorf("IndexOf(%v, %q) = %d, want %d", tt.collection, tt.element, got, tt.want)
			}
		})
	}
}

func TestIndexOf_Uint(t *testing.T) {
	tests := []struct {
		name       string
		collection []uint
		element    uint
		want       int
	}{
		{
			name:       "found",
			collection: []uint{1, 2, 3},
			element:    2,
			want:       1,
		},
		{
			name:       "not found",
			collection: []uint{1, 2, 3},
			element:    5,
			want:       -1,
		},
		{
			name:       "zero value",
			collection: []uint{0, 1, 2},
			element:    0,
			want:       0,
		},
		{
			name:       "empty slice",
			collection: []uint{},
			element:    1,
			want:       -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IndexOf(tt.collection, tt.element)
			if got != tt.want {
				t.Errorf("IndexOf(%v, %d) = %d, want %d", tt.collection, tt.element, got, tt.want)
			}
		})
	}
}
