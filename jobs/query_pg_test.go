//go:build postgres && json1 && fts5

package jobs

import (
	"encoding/json"
	"testing"
	"time"

	"mahresources/models"
)

// These are the cross-engine regressions for the read and retention SQL: a
// qualified `DELETE ... WHERE NOT EXISTS`, a JSON column searched through a cast,
// a case-insensitive text comparison, and keyset predicates on a timestamp. Each
// is a statement SQLite would have tolerated in a spelling PostgreSQL refuses, so
// the pair of tests is what keeps "SQLite and PostgreSQL expose equivalent
// visibility and retention behavior" a checked claim rather than a hope.

// acceptForPG accepts one Job through the real Service against PostgreSQL.
func acceptForPG(t *testing.T, svc *Service, deps Deps, acceptance Acceptance) Snapshot {
	t.Helper()
	snap, err := svc.Accept(deps, acceptance)
	if err != nil {
		t.Fatalf("accept %s: %v", acceptance.Title, err)
	}
	return snap
}

// TestListSearchCursorAndAggregatesOnPostgres drives the listing, the search box,
// the preference dimension and the aggregate over one PostgreSQL database.
func TestListSearchCursorAndAggregatesOnPostgres(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	clock := time.Date(2032, 4, 1, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(title string, owner *uint, summary string) Snapshot {
		clock = clock.Add(time.Minute)
		acceptance := Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: owner, Title: title,
		}
		if summary != "" {
			acceptance.Summary = json.RawMessage(summary)
		}
		return acceptForPG(t, svc, deps, acceptance)
	}
	first := accept("the first export", uintPtr(7), `{"groups":3,"target":"Quarterly Archive"}`)
	second := accept("the second export", uintPtr(7), `{"groups":9}`)
	third := accept("the third export", uintPtr(8), `{"groups":1}`)

	admin := Access{UserID: 1, Administrator: true}

	// Keyset pagination: newest first, and the cursor continues where the page
	// ended without repeating its last row.
	page := listFor(t, svc, deps, admin, Filter{}, Cursor{}, 2)
	requireIDs(t, "first page", pageIDs(page), third.ID, second.ID)
	if page.Next == nil {
		t.Fatal("a full page must report where to continue from")
	}
	page = listFor(t, svc, deps, admin, Filter{}, *page.Next, 2)
	requireIDs(t, "second page", pageIDs(page), first.ID)
	if page.Next != nil {
		t.Fatalf("the listing reported a next page after its last row: %+v", page.Next)
	}

	// The search box reaches the sanitized summary through the JSON column, in
	// any case: the operator offering the case-insensitive comparison is what
	// makes a search for a remembered word work on PostgreSQL.
	requireIDs(t, "summary search", pageIDs(listFor(t, svc, deps, admin, Filter{Search: "quarterly archive"}, Cursor{}, 0)), first.ID)
	requireIDs(t, "case-insensitive title search", pageIDs(listFor(t, svc, deps, admin, Filter{Search: "FIRST EXPORT"}, Cursor{}, 0)), first.ID)
	requireIDs(t, "unmatchable search", pageIDs(listFor(t, svc, deps, admin, Filter{Search: "\x00"}, Cursor{}, 0)))

	// The preference dimension is a subquery over the viewer's own rows.
	viewer := Access{UserID: 7}
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: first.ID, Dismissed: boolPtr(true)}); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	requireIDs(t, "undismissed", pageIDs(listFor(t, svc, deps, viewer, Filter{Dismissed: boolPtr(false)}, Cursor{}, 0)), second.ID)
	requireIDs(t, "dismissed", pageIDs(listFor(t, svc, deps, viewer, Filter{Dismissed: boolPtr(true)}, Cursor{}, 0)), first.ID)

	// And the aggregate answers the same filtered set.
	summary, err := svc.Summary(deps, viewer, Filter{}, 0)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Total != 2 || summary.ByState[string(StateQueued)] != 2 {
		t.Fatalf("summary = %+v", summary)
	}
	summary, err = svc.Summary(deps, admin, Filter{}, 0)
	if err != nil {
		t.Fatalf("Summary as an administrator: %v", err)
	}
	if summary.Total != 3 || summary.ByKind["group-export"] != 3 {
		t.Fatalf("administrator summary = %+v", summary)
	}
}

// TestRetentionSweepPrunesExpiredWorkOnPostgres drives the retention decision —
// a qualified delete guarded by two NOT EXISTS subqueries against the table it is
// deleting from — over PostgreSQL, together with the pin that exempts a Job and
// the unresolved claim that protects one.
func TestRetentionSweepPrunesExpiredWorkOnPostgres(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	policy := RetentionPolicy{History: time.Hour, Attention: 2 * time.Hour}
	deps.Retention = &policy
	clock := time.Date(2032, 4, 2, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(title string, owner *uint) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptForPG(t, svc, deps, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: owner, Title: title,
		})
	}
	finish := func(job Snapshot) Snapshot {
		clock = clock.Add(time.Minute)
		running, err := svc.Transition(deps, Transition{JobID: job.ID, ExpectedVersion: job.Version, To: StateRunning})
		if err != nil {
			t.Fatalf("transition to running: %v", err)
		}
		clock = clock.Add(time.Minute)
		done, err := svc.Transition(deps, Transition{JobID: running.ID, ExpectedVersion: running.Version, To: StateSucceeded})
		if err != nil {
			t.Fatalf("transition to succeeded: %v", err)
		}
		return done
	}

	due := accept("due", uintPtr(7))
	if _, err := svc.PublishOutput(deps, ExecutionRef{JobID: due.ID}, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Label: "the tar",
		Reference: json.RawMessage(`{"name":"export.tar"}`),
	}); err != nil {
		t.Fatalf("publish output: %v", err)
	}
	due = finish(due)
	pinned := finish(accept("pinned", uintPtr(7)))
	protected := finish(accept("claimed", uintPtr(7)))
	if err := svc.SetPreference(deps, Access{UserID: 7}, PreferenceRequest{JobID: pinned.ID, Pinned: boolPtr(true)}); err != nil {
		t.Fatalf("pin: %v", err)
	}
	if err := deps.DB.Create(&models.JobClaim{
		JobID: protected.ID, Kind: protected.Kind, KindVersion: protected.KindVersion,
		Claimant: "host:gone", ExecutionToken: "00000000-0000-7000-8000-000000000002",
		State:     models.JobClaimStateQuarantined,
		ClaimedAt: clock, HeartbeatAt: clock, LeaseExpiresAt: clock,
	}).Error; err != nil {
		t.Fatalf("seed quarantined claim: %v", err)
	}

	clock = clock.Add(24 * time.Hour)
	result, err := svc.Sweep(deps, policy, SweepCursor{}, 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if result.Pruned != 1 {
		t.Fatalf("pruned %d jobs, want 1", result.Pruned)
	}
	if result.Skipped != 2 {
		t.Fatalf("skipped %d jobs, want the pinned and the claimed one", result.Skipped)
	}
	if jobExists(t, deps, due.ID) {
		t.Error("an expired job survived the sweep")
	}
	if rows := countRows(t, deps, &models.JobEvent{}, "job_id = ?", due.ID); rows != 0 {
		t.Errorf("events of the pruned job survived: %d", rows)
	}
	if rows := countRows(t, deps, &models.JobOutput{}, "job_id = ?", due.ID); rows != 0 {
		t.Errorf("outputs of the pruned job survived: %d", rows)
	}
	if !jobExists(t, deps, pinned.ID) {
		t.Error("a pinned job was pruned")
	}
	if !jobExists(t, deps, protected.ID) {
		t.Error("a job with an unresolved claim was pruned")
	}
}
