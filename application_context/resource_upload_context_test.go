package application_context

import (
	"errors"
	"fmt"
	"testing"
)

func TestResourceExistsError_Error(t *testing.T) {
	tests := []struct {
		name       string
		resourceID uint
		reason     string
		want       string
	}{
		// Finding 103: the wording used to be "existing resource (42) with same
		// parent" — an internal reason constant, and an id the reader could not
		// act on. It is a user-facing sentence now; the id stays because API and
		// CLI callers get nothing but this string.
		{
			name:       "same parent reason",
			resourceID: 42,
			reason:     ReasonSameParent,
			want:       "a resource with identical content already exists (#42)",
		},
		{
			name:       "same relation reason",
			resourceID: 100,
			reason:     ReasonSameRelation,
			want:       "a resource with identical content is already in that group (#100)",
		},
		{
			name:       "zero resource ID",
			resourceID: 0,
			reason:     ReasonSameParent,
			want:       "a resource with identical content already exists (#0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ResourceExistsError{
				ResourceID: tt.resourceID,
				Reason:     tt.reason,
			}

			got := err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResourceExistsError_Unwrap(t *testing.T) {
	original := &ResourceExistsError{ResourceID: 7, Reason: ReasonSameParent}
	wrapped := fmt.Errorf("upload failed: %w", original)

	var target *ResourceExistsError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed to unwrap ResourceExistsError")
	}

	if target.ResourceID != 7 {
		t.Errorf("ResourceID = %d, want 7", target.ResourceID)
	}

	if target.Reason != ReasonSameParent {
		t.Errorf("Reason = %q, want %q", target.Reason, ReasonSameParent)
	}
}
