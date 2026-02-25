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
		{
			name:       "same parent reason",
			resourceID: 42,
			reason:     ReasonSameParent,
			want:       "existing resource (42) with same parent",
		},
		{
			name:       "same relation reason",
			resourceID: 100,
			reason:     ReasonSameRelation,
			want:       "existing resource (100) with same relation",
		},
		{
			name:       "zero resource ID",
			resourceID: 0,
			reason:     ReasonSameParent,
			want:       "existing resource (0) with same parent",
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
