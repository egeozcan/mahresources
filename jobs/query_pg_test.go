//go:build postgres && json1 && fts5

package jobs

import (
	"encoding/json"
	"errors"
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
			Replay: ReplayInput{NonReplayable: true},
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
	// The Kind has to be one this process can run, because pruning a Job that
	// published an artifact asks its adapter to account for it first.
	registerTestAdapter(t, svc, Definition{Kind: "group-export", KindVersion: 1, Restorable: true})

	accept := func(title string, owner *uint) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptForPG(t, svc, deps, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: owner, Title: title,
			Replay: ReplayInput{NonReplayable: true},
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

// TestPinAdmissionSerializesOnTheViewersGuardPG covers §9's per-viewer pin limit
// as the admission question it is.
//
// The limit is a count of one viewer's committed pin rows, and two admissions
// that count at the same time both see the limit unmet — no foreign key or unique
// index objects, because the rows are different Jobs. The count therefore has to
// happen under a durable per-viewer guard, which only PostgreSQL can show: SQLite
// serializes writers, so the race cannot form there.
func TestPinAdmissionSerializesOnTheViewersGuardPG(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	clock := time.Date(2032, 9, 10, 11, 12, 13, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	deps.PinLimit = 2

	const viewerID = uint(7)
	viewer := viewerID

	accept := func(title string) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptForPG(t, svc, deps, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: &viewer, Title: title,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	held := accept("already pinned")
	first := accept("one of the last two slots")
	second := accept("the other of the last two slots")

	access := Access{UserID: viewerID}
	if err := svc.SetPreference(deps, access, PreferenceRequest{JobID: held.ID, Pinned: boolPtr(true)}); err != nil {
		t.Fatalf("pin the first: %v", err)
	}

	// A transaction holds the viewer's own admission row, exactly as an in-flight
	// pin admission for that viewer does. An admission that counts without the
	// guard runs straight past it.
	var guard models.JobPinGuard
	if err := deps.DB.Where("user_id = ?", viewerID).First(&guard).Error; err != nil {
		t.Fatalf("the first pin did not open the viewer's admission row: %v", err)
	}
	holder := deps.DB.Begin()
	if holder.Error != nil {
		t.Fatalf("begin: %v", holder.Error)
	}
	if err := holder.Exec("SELECT user_id FROM job_pin_guards WHERE user_id = ? FOR UPDATE", viewerID).Error; err != nil {
		t.Fatalf("hold the admission row: %v", err)
	}

	admitted := make(chan error, 1)
	go func() {
		admitted <- svc.SetPreference(deps, access, PreferenceRequest{JobID: first.ID, Pinned: boolPtr(true)})
	}()
	select {
	case err := <-admitted:
		t.Fatalf("a pin admission did not wait for the viewer's guard: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := holder.Commit().Error; err != nil {
		t.Fatalf("commit the holder: %v", err)
	}
	select {
	case err := <-admitted:
		if err != nil {
			t.Fatalf("pin after the guard was released: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the pin never completed after the guard was released")
	}

	// And the limit itself holds under concurrent admission of the last slot:
	// with one pin held and one admitted, the second admission is refused.
	if err := svc.SetPreference(deps, access, PreferenceRequest{JobID: second.ID, Pinned: boolPtr(true)}); !errors.Is(err, ErrPinLimitReached) {
		t.Fatalf("pinning past the limit = %v, want ErrPinLimitReached", err)
	}
	var pinned int64
	if err := deps.DB.Model(&models.JobPreference{}).
		Where("user_id = ? AND pinned_at IS NOT NULL", viewerID).Count(&pinned).Error; err != nil {
		t.Fatalf("count pins: %v", err)
	}
	if pinned != 2 {
		t.Fatalf("%d pins are held under a limit of %d", pinned, deps.PinLimit)
	}
}
