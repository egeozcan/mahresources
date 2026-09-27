package jobs

import (
	"errors"
	"testing"
)

// An executor that gives an accepted Job an id of its own afterwards records it
// as one more handle the Job answers to. A handle another Job already answers to
// is refused, and so is one for a Job that does not exist.
func TestAddLegacyHandleRecordsOneMoreHandle(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	job := acceptQueued(t, svc, deps, nil)
	other := acceptQueued(t, svc, deps, nil)

	ref := LegacyRef{Namespace: "download", Handle: "queue-entry-1"}
	if err := svc.AddLegacyHandle(deps, job.ID, ref); err != nil {
		t.Fatalf("add a handle: %v", err)
	}
	if resolved, err := svc.ResolveLegacyHandle(deps, ref.Namespace, ref.Handle); err != nil || resolved != job.ID {
		t.Fatalf("the handle resolves to %q (%v), want %s", resolved, err, job.ID)
	}
	if err := svc.AddLegacyHandle(deps, other.ID, ref); err == nil {
		t.Fatalf("a handle another Job answers to was given to a second Job")
	}
	if resolved, _ := svc.ResolveLegacyHandle(deps, ref.Namespace, ref.Handle); resolved != job.ID {
		t.Fatalf("the refused handle moved to %s", resolved)
	}
	if err := svc.AddLegacyHandle(deps, "01a0dead-0000-7000-8000-000000000000", LegacyRef{Namespace: "download", Handle: "orphan"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a handle for a missing Job answered %v, want not found", err)
	}
	if err := svc.AddLegacyHandle(deps, job.ID, LegacyRef{Namespace: "download"}); !errors.Is(err, ErrInvalidAcceptance) {
		t.Fatalf("an empty handle answered %v, want invalid", err)
	}
}
