package jobs

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"mahresources/models"
)

// seededExecution writes a Job that a claim owns. Every executor-facing write —
// progress, events, outputs, finishing — is made under the token a claim
// created, so this is the state those tests start from.
func seededExecution(t *testing.T, deps Deps, state State, token string) models.Job {
	t.Helper()
	job := seedJob(t, deps, state, deps.now(), 1)
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", job.ID).
		Update("execution_token", token).Error; err != nil {
		t.Fatalf("seed execution token: %v", err)
	}
	job.ExecutionToken = token
	return job
}

// TestEventAppendRecordsSignificantEventsOnTheJobsTimeline covers what an
// appended event is: an immutable significant fact that continues the Job's own
// sequence, carries the Job version it belongs to, and — unlike a progress tick
// — does not move that version. A transition after an append must continue from
// the appended fact rather than reuse its position.
func TestEventAppendRecordsSignificantEventsOnTheJobsTimeline(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2032, 2, 3, 4, 5, 6, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}

	if err := svc.AppendEvent(deps, ref, EventInput{
		Type:   "checkpoint",
		Detail: []byte(`{"segment":120}`),
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	clock = clock.Add(time.Second)
	if err := svc.AppendEvent(deps, ref, EventInput{Type: "recovered"}); err != nil {
		t.Fatalf("second AppendEvent: %v", err)
	}

	events := jobEvents(t, deps, job.ID)
	if len(events) != 2 {
		t.Fatalf("appended %d events, want 2", len(events))
	}
	first := events[0]
	if first.Type != "checkpoint" || first.Sequence != 1 || first.JobVersion != job.Version {
		t.Errorf("first event = %+v", first)
	}
	if string(first.Detail) != `{"segment":120}` {
		t.Errorf("first event detail = %s", first.Detail)
	}
	if first.ReservedHost {
		t.Error("an adapter's own event must not claim the reserved host capacity")
	}
	if first.CreatedAt.Location() != time.UTC {
		t.Errorf("first event recorded %v, want UTC", first.CreatedAt.Location())
	}
	if events[1].Type != "recovered" || events[1].Sequence != 2 {
		t.Errorf("second event = %+v", events[1])
	}
	if stored := jobRow(t, deps, job.ID); stored.Version != job.Version {
		t.Errorf("an appended event moved the lifecycle version to %d", stored.Version)
	}

	// A lifecycle transition afterwards continues the same timeline: the
	// unique index on (job_id, sequence) is what would refuse a repeat of a
	// position an appended fact already occupies.
	snap, err := svc.Transition(deps, Transition{JobID: job.ID, ExpectedVersion: job.Version, ExecutionToken: "claim-a", To: StateSucceeded})
	if err != nil {
		t.Fatalf("Transition after appends: %v", err)
	}
	if snap.State != StateSucceeded {
		t.Fatalf("snapshot = %s", snap.State)
	}
	events = jobEvents(t, deps, job.ID)
	if len(events) != 3 || events[2].Type != EventSucceeded || events[2].Sequence != 3 {
		t.Fatalf("terminal event = %+v, want the third position on the timeline", events[len(events)-1])
	}
	if !events[2].ReservedHost {
		t.Error("a lifecycle event is a reserved host event")
	}
}

// TestEventAppendRefusesAStaleExecutionTokenAndOutOfBoundsEvents is the refusal
// half of the seam: the token fence, and the size/typing bounds that keep the
// event table from becoming an unindexed log of whatever an adapter sends.
func TestEventAppendRefusesAStaleExecutionTokenAndOutOfBoundsEvents(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2032, 2, 3, 4, 5, 6, 0, time.UTC) }

	job := seededExecution(t, deps, StateRunning, "claim-a")

	tests := []struct {
		name       string
		ref        ExecutionRef
		event      EventInput
		wantTarget error
	}{
		{
			name:       "stale token",
			ref:        ExecutionRef{JobID: job.ID, ExecutionToken: "claim-b"},
			event:      EventInput{Type: "checkpoint"},
			wantTarget: ErrStaleExecution,
		},
		{
			name:       "no event type",
			ref:        ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"},
			event:      EventInput{Detail: []byte(`{"a":1}`)},
			wantTarget: ErrInvalidEvent,
		},
		{
			name:       "event type over its ceiling",
			ref:        ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"},
			event:      EventInput{Type: string(make([]byte, MaxEventTypeBytes+1))},
			wantTarget: ErrInvalidEvent,
		},
		{
			name:       "detail over its ceiling",
			ref:        ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"},
			event:      EventInput{Type: "checkpoint", Detail: []byte(`"` + string(make([]byte, MaxEventDetailBytes+1)) + `"`)},
			wantTarget: ErrInvalidEvent,
		},
		{
			name:       "malformed detail",
			ref:        ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"},
			event:      EventInput{Type: "checkpoint", Detail: []byte(`{"unterminated"`)},
			wantTarget: ErrInvalidEvent,
		},
		{
			name:       "no job id",
			ref:        ExecutionRef{ExecutionToken: "claim-a"},
			event:      EventInput{Type: "checkpoint"},
			wantTarget: ErrInvalidExecution,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.AppendEvent(deps, tt.ref, tt.event); !errors.Is(err, tt.wantTarget) {
				t.Fatalf("AppendEvent = %v, want %v", err, tt.wantTarget)
			}
			if events := jobEvents(t, deps, job.ID); len(events) != 0 {
				t.Fatalf("a refused append recorded %d events", len(events))
			}
		})
	}
}

// TestEventAppendTruncatesOptionalTrafficIntoOneReservedWarning is the capacity
// contract of §6: optional Kind traffic is bounded, it cannot consume the
// headroom reserved for host facts, and because blocking a lifecycle transition
// on an adapter's chatter would be far worse than losing the chatter, the
// reservation is what guarantees a terminal event always fits.
func TestEventAppendTruncatesOptionalTrafficIntoOneReservedWarning(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2032, 2, 3, 4, 5, 6, 0, time.UTC) }

	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}

	for i := 0; i < MaxOptionalEventsPerJob+5; i++ {
		if err := svc.AppendEvent(deps, ref, EventInput{Type: "phase-tick"}); err != nil {
			t.Fatalf("AppendEvent %d: %v", i, err)
		}
	}

	optional := 0
	truncations := 0
	for _, event := range jobEvents(t, deps, job.ID) {
		switch event.Type {
		case EventTruncated:
			truncations++
			if !event.ReservedHost {
				t.Error("the truncation warning must be a reserved host event")
			}
		default:
			optional++
		}
	}
	if optional != MaxOptionalEventsPerJob {
		t.Errorf("stored %d optional events, want the %d-event ceiling", optional, MaxOptionalEventsPerJob)
	}
	if truncations != 1 {
		t.Errorf("recorded %d truncation warnings, want exactly one", truncations)
	}

	// The reservation is the point: with optional capacity exhausted, a
	// lifecycle fact is still recorded and the Job still finishes.
	snap, err := svc.Transition(deps, Transition{JobID: job.ID, ExpectedVersion: job.Version, ExecutionToken: "claim-a", To: StateSucceeded})
	if err != nil {
		t.Fatalf("Transition with optional capacity exhausted: %v", err)
	}
	if snap.State != StateSucceeded {
		t.Fatalf("snapshot = %s, want succeeded", snap.State)
	}
	events := jobEvents(t, deps, job.ID)
	last := events[len(events)-1]
	if last.Type != EventSucceeded || !last.ReservedHost {
		t.Fatalf("the terminal event was not recorded: %+v", last)
	}
	if len(events) > MaxEventsPerJob {
		t.Fatalf("%d events exceed the %d-event ceiling", len(events), MaxEventsPerJob)
	}

	// And an adapter cannot take the reserved headroom for itself by claiming
	// its own event is a host fact.
	for i := 0; i < 3; i++ {
		if err := svc.AppendEvent(deps, ref, EventInput{Type: "post-terminal"}); err != nil {
			t.Fatalf("post-terminal append: %v", err)
		}
	}
	if after := jobEvents(t, deps, job.ID); len(after) != len(events) {
		t.Fatalf("appends after truncation stored %d more events", len(after)-len(events))
	}
}

// TestLinkRecordsTypedLineageIdempotently covers what a link is: one typed
// durable relation between two Jobs that keep their own identity and outcome
// inside it. Recording the same relation twice is the same fact, not a second
// one, so it stays one row.
func TestLinkRecordsTypedLineageIdempotently(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2032, 4, 5, 6, 7, 8, 0, time.UTC) }

	parent := seededExecution(t, deps, StateQueued, "")
	child := seededExecution(t, deps, StateRunning, "claim-a")

	if err := svc.Link(deps, LinkRequest{Type: LinkParentChild, FromJobID: parent.ID, ToJobID: child.ID}); err != nil {
		t.Fatalf("Link: %v", err)
	}
	// The same relation again is the same fact.
	if err := svc.Link(deps, LinkRequest{Type: LinkParentChild, FromJobID: parent.ID, ToJobID: child.ID}); err != nil {
		t.Fatalf("repeated Link: %v", err)
	}
	// The reverse direction is a different relation, not a duplicate.
	if err := svc.Link(deps, LinkRequest{Type: LinkRetryOf, FromJobID: child.ID, ToJobID: parent.ID}); err != nil {
		t.Fatalf("reverse Link: %v", err)
	}

	var links []models.JobLink
	if err := deps.DB.Order("type, from_job_id").Find(&links).Error; err != nil {
		t.Fatalf("read links: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("stored %d links, want 2", len(links))
	}
	if links[0].Type != models.JobLinkParentChild || links[0].FromJobID != parent.ID || links[0].ToJobID != child.ID {
		t.Errorf("parent-child link = %+v", links[0])
	}
	if links[1].Type != models.JobLinkRetryOf || links[1].FromJobID != child.ID || links[1].ToJobID != parent.ID {
		t.Errorf("retry-of link = %+v", links[1])
	}

	// Lineage is not a lifecycle fact recorded inline: linking writes one row
	// and touches neither ancestor's outcome or timeline.
	for _, job := range []models.Job{parent, child} {
		stored := jobRow(t, deps, job.ID)
		if stored.State != job.State || stored.Version != job.Version {
			t.Errorf("linking moved job %s to %s v%d", job.ID, stored.State, stored.Version)
		}
		if events := jobEvents(t, deps, job.ID); len(events) != 0 {
			t.Errorf("linking recorded %d events on %s", len(events), job.ID)
		}
	}
}

// TestLinkRefusesUnknownTypesSelfLinksAndUnknownJobs covers the refusals. A
// relation outside the vocabulary would be a relation nothing can follow, and a
// link to a Job that does not exist would be dangling, because these tables
// carry no foreign keys on purpose.
func TestLinkRefusesUnknownTypesSelfLinksAndUnknownJobs(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2032, 4, 5, 6, 7, 8, 0, time.UTC) }

	a := seededExecution(t, deps, StateQueued, "")
	b := seededExecution(t, deps, StateQueued, "")

	tests := []struct {
		name string
		link LinkRequest
	}{
		{"unknown type", LinkRequest{Type: "child-of", FromJobID: a.ID, ToJobID: b.ID}},
		{"no type", LinkRequest{FromJobID: a.ID, ToJobID: b.ID}},
		{"self link", LinkRequest{Type: LinkParentChild, FromJobID: a.ID, ToJobID: a.ID}},
		{"no from", LinkRequest{Type: LinkParentChild, ToJobID: b.ID}},
		{"no to", LinkRequest{Type: LinkParentChild, FromJobID: a.ID}},
		{"unknown from", LinkRequest{Type: LinkParentChild, FromJobID: "0193b0a0-0000-7000-8000-000000000000", ToJobID: b.ID}},
		{"unknown to", LinkRequest{Type: LinkParentChild, FromJobID: a.ID, ToJobID: "0193b0a0-0000-7000-8000-000000000000"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.Link(deps, tt.link)
			if !errors.Is(err, ErrInvalidLink) && !errors.Is(err, ErrNotFound) {
				t.Fatalf("Link = %v, want ErrInvalidLink or ErrNotFound", err)
			}
			var count int64
			deps.DB.Model(&models.JobLink{}).Count(&count)
			if count != 0 {
				t.Fatalf("a refused link stored %d rows", count)
			}
		})
	}
}

// TestLinkDoesNotMakeHiddenRelativesVisibleOrCountable is §8's lineage rule:
// every related Job is authorized independently, and a hidden parent, child,
// retry or repeat is neither named nor counted. The link is durable — an
// authorized reader can follow it — but it is not a capability, so it cannot be
// used to reach or to enumerate an execution the asker may not see.
func TestLinkDoesNotMakeHiddenRelativesVisibleOrCountable(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2032, 4, 5, 6, 7, 8, 0, time.UTC) }

	owner := uint(7)
	visible, err := svc.Accept(deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api", OwnerUserID: &owner,
		Title: "mine",
	})
	if err != nil {
		t.Fatalf("accept visible job: %v", err)
	}
	hidden, err := svc.Accept(deps, Acceptance{
		Kind: "plugin-command", KindVersion: 1, State: StateQueued, Origin: "plugin",
		OwnerUserID: &owner, Visibility: VisibilityAdmin, Title: "not theirs to read",
	})
	if err != nil {
		t.Fatalf("accept admin-class job: %v", err)
	}

	before, err := svc.Get(deps, Access{UserID: owner}, visible.ID)
	if err != nil {
		t.Fatalf("Get before linking: %v", err)
	}

	for _, link := range []LinkRequest{
		{Type: LinkParentChild, FromJobID: visible.ID, ToJobID: hidden.ID},
		{Type: LinkRetryOf, FromJobID: hidden.ID, ToJobID: visible.ID},
	} {
		if err := svc.Link(deps, link); err != nil {
			t.Fatalf("Link: %v", err)
		}
	}

	// The relative is still hidden, in both directions of the relation.
	if _, err := svc.Get(deps, Access{UserID: owner}, hidden.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a linked hidden Job = %v, want ErrNotFound", err)
	}

	// And the visible Job reads exactly as it did before anything was linked to
	// it: no relative is named, so none can be counted.
	after, err := svc.Get(deps, Access{UserID: owner}, visible.ID)
	if err != nil {
		t.Fatalf("Get after linking: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("linking changed what a viewer reads:\nbefore %+v\nafter  %+v", before, after)
	}
	encoded, err := json.Marshal(after)
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}
	if strings.Contains(string(encoded), hidden.ID) {
		t.Fatalf("a visible snapshot names the hidden relative: %s", encoded)
	}
}

// TestProgressUpdatesReplaceOneBoundedSnapshotWithoutEvents covers what a
// progress tick is: the latest snapshot of work in flight, replaced in place.
//
// Two properties are the whole point. A tick is not a Job Event — at a tick per
// few hundred milliseconds, storing each one would drown the timeline an
// operator reads — and it does not consume the lifecycle version, because a
// version bump per tick would invalidate the version a transition in flight
// decided from, and a Job could then never finish while its executor reported
// progress.
func TestProgressUpdatesReplaceOneBoundedSnapshotWithoutEvents(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2032, 1, 2, 3, 4, 5, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := seededExecution(t, deps, StateRunning, "claim-a")

	total := int64(1000)
	eta := clock.Add(10 * time.Minute)
	first, err := svc.UpdateProgress(deps, ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}, Progress{
		Phase:     "downloading segments",
		Completed: int64Ptr(250),
		Total:     &total,
		Unit:      "segments",
		Message:   "250 of 1000 segments",
		ETA:       &eta,
	})
	if err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}

	stored := jobRow(t, deps, job.ID)
	if stored.ProgressCompleted == nil || *stored.ProgressCompleted != 250 {
		t.Errorf("stored completed = %v, want 250", stored.ProgressCompleted)
	}
	if stored.ProgressTotal == nil || *stored.ProgressTotal != 1000 {
		t.Errorf("stored total = %v, want 1000", stored.ProgressTotal)
	}
	if stored.ProgressUnit != "segments" || stored.ProgressMessage != "250 of 1000 segments" {
		t.Errorf("stored unit/message = %q/%q", stored.ProgressUnit, stored.ProgressMessage)
	}
	if stored.ProgressETA == nil || !stored.ProgressETA.Equal(eta) || stored.ProgressETA.Location() != time.UTC {
		t.Errorf("stored ETA = %v, want %v in UTC", stored.ProgressETA, eta)
	}
	if stored.Phase != "downloading segments" {
		t.Errorf("stored phase = %q, want the phase the snapshot named", stored.Phase)
	}
	if first.Progress.Completed == nil || *first.Progress.Completed != 250 || first.Progress.Unit != "segments" {
		t.Errorf("snapshot progress = %+v", first.Progress)
	}
	if first.Version != job.Version {
		t.Errorf("a progress tick moved the lifecycle version to %d", first.Version)
	}

	// The second tick replaces the snapshot rather than merging into it: the
	// fields it leaves out are cleared. The phase is the one exception, and
	// deliberately so — a byte tick does not mean the Job left the phase a
	// previous tick or the transition into running named.
	clock = clock.Add(time.Second)
	second, err := svc.UpdateProgress(deps, ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}, Progress{
		Completed: int64Ptr(400),
		Message:   "400 of 1000 segments",
	})
	if err != nil {
		t.Fatalf("second UpdateProgress: %v", err)
	}

	stored = jobRow(t, deps, job.ID)
	if stored.ProgressCompleted == nil || *stored.ProgressCompleted != 400 {
		t.Errorf("stored completed = %v, want the replacement 400", stored.ProgressCompleted)
	}
	if stored.ProgressTotal != nil {
		t.Errorf("stored total = %v, want cleared by the replacement", stored.ProgressTotal)
	}
	if stored.ProgressUnit != "" || stored.ProgressETA != nil {
		t.Errorf("stored unit/ETA = %q/%v, want cleared by the replacement", stored.ProgressUnit, stored.ProgressETA)
	}
	if stored.ProgressMessage != "400 of 1000 segments" {
		t.Errorf("stored message = %q", stored.ProgressMessage)
	}
	if stored.Phase != "downloading segments" {
		t.Errorf("phase = %q, want the earlier phase kept when the tick named none", stored.Phase)
	}
	if second.Progress.Total != nil || second.Progress.Unit != "" {
		t.Errorf("snapshot progress = %+v, want the replacement", second.Progress)
	}
	if second.Version != job.Version {
		t.Errorf("a progress tick moved the lifecycle version to %d", second.Version)
	}

	if events := jobEvents(t, deps, job.ID); len(events) != 0 {
		t.Fatalf("progress ticks recorded %d events; a tick is a snapshot, not a Job Event", len(events))
	}
}

// TestProgressRefusesAStaleExecutionToken is the fence that keeps a replaced
// executor from publishing: the claim that owns a Job holds a token, and a
// worker whose claim expired carries one that no longer matches.
func TestProgressRefusesAStaleExecutionToken(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2032, 1, 2, 3, 4, 5, 0, time.UTC) }

	job := seededExecution(t, deps, StateRunning, "claim-a")

	_, err := svc.UpdateProgress(deps, ExecutionRef{JobID: job.ID, ExecutionToken: "claim-b"}, Progress{
		Completed: int64Ptr(500),
	})
	if !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("stale token = %v, want ErrStaleExecution", err)
	}
	if stored := jobRow(t, deps, job.ID); stored.ProgressCompleted != nil {
		t.Fatalf("a refused progress update wrote %v", *stored.ProgressCompleted)
	}

	if _, err := svc.UpdateProgress(deps, ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}, Progress{
		Completed: int64Ptr(500),
	}); err != nil {
		t.Fatalf("the owning token: %v", err)
	}
	if stored := jobRow(t, deps, job.ID); stored.ProgressCompleted == nil || *stored.ProgressCompleted != 500 {
		t.Fatalf("the owning token wrote %v", stored.ProgressCompleted)
	}
}

// TestProgressRefusesMalformedRequestsAndTerminalJobs covers the two other ways
// a progress update is refused: the request itself is out of bounds, or the Job
// it addresses has already finished. A terminal Job never changes, and progress
// is not an exception to that.
func TestProgressRefusesMalformedRequestsAndTerminalJobs(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2032, 1, 2, 3, 4, 5, 0, time.UTC) }

	negative := int64(-1)
	tests := []struct {
		name       string
		ref        ExecutionRef
		progress   Progress
		wantTarget error
	}{
		{
			name:       "no job id",
			ref:        ExecutionRef{ExecutionToken: "claim-a"},
			progress:   Progress{Message: "x"},
			wantTarget: ErrInvalidExecution,
		},
		{
			name:       "phase over its ceiling",
			ref:        ExecutionRef{JobID: "job", ExecutionToken: "claim-a"},
			progress:   Progress{Phase: string(make([]byte, MaxPhaseBytes+1))},
			wantTarget: ErrInvalidProgress,
		},
		{
			name:       "unit over its ceiling",
			ref:        ExecutionRef{JobID: "job", ExecutionToken: "claim-a"},
			progress:   Progress{Unit: string(make([]byte, MaxProgressUnitBytes+1))},
			wantTarget: ErrInvalidProgress,
		},
		{
			name:       "message over its ceiling",
			ref:        ExecutionRef{JobID: "job", ExecutionToken: "claim-a"},
			progress:   Progress{Message: string(make([]byte, MaxProgressMessageBytes+1))},
			wantTarget: ErrInvalidProgress,
		},
		{
			name:       "negative completed",
			ref:        ExecutionRef{JobID: "job", ExecutionToken: "claim-a"},
			progress:   Progress{Completed: &negative},
			wantTarget: ErrInvalidProgress,
		},
		{
			name:       "negative total",
			ref:        ExecutionRef{JobID: "job", ExecutionToken: "claim-a"},
			progress:   Progress{Total: &negative},
			wantTarget: ErrInvalidProgress,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.UpdateProgress(deps, tt.ref, tt.progress)
			if !errors.Is(err, tt.wantTarget) {
				t.Fatalf("UpdateProgress = %v, want %v", err, tt.wantTarget)
			}
		})
	}

	for _, terminal := range []State{StateSucceeded, StateFailed, StateCancelled, StateInterrupted} {
		t.Run("terminal "+string(terminal), func(t *testing.T) {
			job := seededExecution(t, deps, terminal, "claim-a")
			_, err := svc.UpdateProgress(deps, ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}, Progress{
				Message: "late progress",
			})
			if !errors.Is(err, ErrIllegalTransition) {
				t.Fatalf("progress on a %s Job = %v, want ErrIllegalTransition", terminal, err)
			}
			if stored := jobRow(t, deps, job.ID); stored.ProgressMessage != "" {
				t.Fatalf("a terminal Job's progress was rewritten to %q", stored.ProgressMessage)
			}
		})
	}
}
