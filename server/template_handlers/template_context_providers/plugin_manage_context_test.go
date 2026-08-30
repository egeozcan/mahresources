package template_context_providers

import "testing"

func TestScheduledDownloadTemplateStateOnlyStopsOwnerlessPending(t *testing.T) {
	cases := []struct {
		name   string
		status string
		owned  bool
		want   string
	}{
		{name: "owned pending", status: "pending", owned: true, want: "pending"},
		{name: "ownerless pending", status: "pending", owned: false, want: "stopped"},
		{name: "ownerless submitted", status: "submitted", owned: false, want: "submitted"},
		{name: "ownerless failed", status: "failed", owned: false, want: "failed"},
		{name: "ownerless cancelled", status: "cancelled", owned: false, want: "cancelled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scheduledDownloadTemplateState(tc.status, tc.owned); got != tc.want {
				t.Fatalf("template state = %q, want %q", got, tc.want)
			}
		})
	}
}
