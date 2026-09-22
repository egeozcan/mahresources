package jobs

import (
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

// acceptFor accepts one Job through the real Service with every field a
// read-path test needs named explicitly.
func acceptFor(t *testing.T, svc *Service, deps Deps, acceptance Acceptance) Snapshot {
	t.Helper()
	snap, err := svc.Accept(deps, acceptance)
	if err != nil {
		t.Fatalf("accept %s: %v", acceptance.Title, err)
	}
	return snap
}

// listFor runs one visible listing and fails the test on an unexpected error.
func listFor(t *testing.T, svc *Service, deps Deps, access Access, filter Filter, cursor Cursor, limit int) Page {
	t.Helper()
	page, err := svc.List(deps, access, filter, cursor, limit)
	if err != nil {
		t.Fatalf("List(%+v): %v", filter, err)
	}
	return page
}

// pageIDs names the Jobs a page returned, in order.
func pageIDs(page Page) []string {
	ids := make([]string, 0, len(page.Jobs))
	for _, job := range page.Jobs {
		ids = append(ids, job.ID)
	}
	return ids
}

// requireIDs compares a listing against the exact order it must have.
func requireIDs(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s listed %d jobs %v, want %d %v", what, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s listed %v, want %v", what, got, want)
		}
	}
}

// TestVisibilityShowsAnOrdinaryUserOnlyTheirOwnOwnerClassJobs is §8's rule at the
// one predicate every read shares: an administrator sees every Job, everybody
// else sees a Job only when it carries the public visibility class and they own
// it. The class is a durable property of the row — fixed by the registered Kind,
// never by request input — which is what keeps a demoted submitter out of the
// admin-class work they once started.
func TestVisibilityShowsAnOrdinaryUserOnlyTheirOwnOwnerClassJobs(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 1, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(title string, owner *uint, class VisibilityClass) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: owner, Title: title, Visibility: class,
			Replay: ReplayInput{NonReplayable: true},
		})
	}

	owner := accept("mine", uintPtr(7), "")
	other := accept("theirs", uintPtr(8), "")
	adminClass := accept("a command run", uintPtr(7), VisibilityAdmin)
	ownerless := accept("a system job", nil, "")
	// A command run submitted while its owner was still an administrator: the
	// class is on the row, so a later demotion changes nothing about what it
	// exposes.
	formerAdmin := accept("a command run from an admin", uintPtr(9), VisibilityAdmin)

	ordinary := Access{UserID: 7}
	requireIDs(t, "an ordinary user's listing", pageIDs(listFor(t, svc, deps, ordinary, Filter{}, Cursor{}, 0)), owner.ID)

	admin := Access{UserID: 3, Administrator: true}
	requireIDs(t, "an administrator's listing",
		pageIDs(listFor(t, svc, deps, admin, Filter{}, Cursor{}, 0)),
		formerAdmin.ID, ownerless.ID, adminClass.ID, other.ID, owner.ID)

	for _, hidden := range []Snapshot{other, adminClass, ownerless, formerAdmin} {
		if _, err := svc.Get(deps, ordinary, hidden.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%s, %q) = %v, want ErrNotFound", hidden.Title, hidden.ID, err)
		}
	}
	// The demoted-owner case is the whole point of a durable class: user 9 owns
	// the command run and still cannot see it.
	if _, err := svc.Get(deps, Access{UserID: 9}, formerAdmin.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a non-admin owner saw their own admin-class job: %v", err)
	}
	for _, visible := range []Snapshot{owner, other, adminClass, ownerless, formerAdmin} {
		if _, err := svc.Get(deps, admin, visible.ID); err != nil {
			t.Errorf("an administrator could not read %s: %v", visible.Title, err)
		}
	}
}

// TestListFiltersByTheStoredDimensions proves each dimension of a listing is a
// durable predicate rather than something applied to a page afterwards: a filter
// that ran in Go would answer its page from one set and its count from another.
func TestListFiltersByTheStoredDimensions(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 3, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(kind, origin string, owner, actor *uint) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: kind, KindVersion: 1, State: StateQueued, Origin: origin,
			OwnerUserID: owner, ActorUserID: actor, Title: kind,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	alpha := accept("group-export", "ui", uintPtr(7), uintPtr(7))
	beta := accept("remote-download", "cli", uintPtr(7), uintPtr(9))
	gamma := accept("remote-download", "plugin", uintPtr(8), nil)
	delta := accept("remote-download", "api", uintPtr(7), uintPtr(7))
	clock = clock.Add(time.Minute)
	running := accept("reconciler", "system", uintPtr(7), nil)
	running = advanceReplayJob(t, svc, deps, running, StateRunning)

	admin := Access{UserID: 1, Administrator: true}
	requireIDs(t, "state", pageIDs(listFor(t, svc, deps, admin, Filter{States: []string{string(StateRunning)}}, Cursor{}, 0)), running.ID)
	requireIDs(t, "kind", pageIDs(listFor(t, svc, deps, admin, Filter{Kinds: []string{"remote-download"}}, Cursor{}, 0)),
		delta.ID, gamma.ID, beta.ID)
	requireIDs(t, "origin", pageIDs(listFor(t, svc, deps, admin, Filter{Origins: []string{"ui", "system"}}, Cursor{}, 0)),
		running.ID, alpha.ID)
	requireIDs(t, "owner", pageIDs(listFor(t, svc, deps, admin, Filter{OwnerID: uintPtr(8)}, Cursor{}, 0)), gamma.ID)
	requireIDs(t, "actor", pageIDs(listFor(t, svc, deps, admin, Filter{ActorID: uintPtr(9)}, Cursor{}, 0)), beta.ID)

	after := alpha.AcceptedAt
	requireIDs(t, "accepted after", pageIDs(listFor(t, svc, deps, admin, Filter{AcceptedAfter: &after}, Cursor{}, 0)),
		running.ID, delta.ID, gamma.ID, beta.ID, alpha.ID)
	before := delta.AcceptedAt
	requireIDs(t, "accepted before", pageIDs(listFor(t, svc, deps, admin, Filter{AcceptedBefore: &before}, Cursor{}, 0)),
		delta.ID, gamma.ID, beta.ID, alpha.ID)
	combined := listFor(t, svc, deps, admin, Filter{
		Kinds: []string{"remote-download"}, Origins: []string{"api"}, OwnerID: uintPtr(7), ActorID: uintPtr(7),
	}, Cursor{}, 0)
	requireIDs(t, "combined", pageIDs(combined), delta.ID)
}

// TestListFiltersByLineageRelationship pins the relation filter to the spelling
// every other lineage read uses: the FROM endpoint of a link, which is the
// successor of a Retry or Repeat and the parent of a child stage.
func TestListFiltersByLineageRelationship(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 4, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(title string) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	first := accept("first")
	second := accept("second")
	third := accept("third")
	fourth := accept("fourth")
	if err := svc.Link(deps, LinkRequest{Type: LinkRetryOf, FromJobID: second.ID, ToJobID: first.ID}); err != nil {
		t.Fatalf("retry link: %v", err)
	}
	if err := svc.Link(deps, LinkRequest{Type: LinkRepeatOf, FromJobID: third.ID, ToJobID: first.ID}); err != nil {
		t.Fatalf("repeat link: %v", err)
	}
	if err := svc.Link(deps, LinkRequest{Type: LinkParentChild, FromJobID: second.ID, ToJobID: fourth.ID}); err != nil {
		t.Fatalf("parent link: %v", err)
	}

	admin := Access{UserID: 1, Administrator: true}
	requireIDs(t, "retry-of", pageIDs(listFor(t, svc, deps, admin, Filter{Relationship: string(LinkRetryOf)}, Cursor{}, 0)), second.ID)
	requireIDs(t, "repeat-of", pageIDs(listFor(t, svc, deps, admin, Filter{Relationship: string(LinkRepeatOf)}, Cursor{}, 0)), third.ID)
	requireIDs(t, "parent-child", pageIDs(listFor(t, svc, deps, admin, Filter{Relationship: string(LinkParentChild)}, Cursor{}, 0)), second.ID)

	if _, err := svc.List(deps, admin, Filter{Relationship: "retried"}, Cursor{}, 0); !errors.Is(err, ErrInvalidFilter) {
		t.Fatalf("an unknown relationship = %v, want ErrInvalidFilter", err)
	}
}

// TestListSearchMatchesSanitizedTextAndExcludesSecretsAndDiagnostics is §8's
// search contract: the box reaches what a viewer may read — identity, title,
// sanitized summary, sanitized failure message and output labels — and never the
// ciphertext beside the Job or the protected diagnostic reference.
func TestListSearchMatchesSanitizedTextAndExcludesSecretsAndDiagnostics(t *testing.T) {
	deps, dsn := newReplayDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 5, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}

	admin := Access{UserID: 1, Administrator: true}

	// A Job whose sanitized summary, title and sealed input are three different
	// things.
	sealed := acceptFor(t, svc, deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		Title: "a quarterly recording", Replay: ReplayInput{Input: fixtureReplayInput()},
	})
	var envelope models.JobReplayEnvelope
	if err := deps.DB.Where("job_id = ?", sealed.ID).First(&envelope).Error; err != nil {
		t.Fatalf("load envelope: %v", err)
	}
	if len(envelope.Ciphertext) == 0 {
		t.Fatal("the fixture stored no ciphertext, so the exclusion proves nothing")
	}

	// A failed Job whose message is for a person and whose diagnostic reference
	// is for an administrator.
	failed := acceptFor(t, svc, deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api", Title: "an export",
		Replay: ReplayInput{NonReplayable: true},
	})
	failed = advanceReplayJob(t, svc, deps, failed, StateRunning)
	finished, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: failed.ID, ExecutionToken: executionTokenOf(t, deps, failed.ID)},
		ExpectedVersion: failed.Version, Outcome: StateFailed,
		Failure: &Failure{
			Code: "disk-full", Class: FailureClassDependency,
			Message: "the storage volume was full", DiagnosticRef: "/var/lib/internal/path-4f2a.log",
		},
	})
	if err != nil {
		t.Fatalf("finish failed job: %v", err)
	}
	_ = finished

	// An output label is searchable text too.
	labeled := acceptFor(t, svc, deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api", Title: "a second export",
		Replay: ReplayInput{NonReplayable: true},
	})
	if _, err := svc.PublishOutput(deps, ExecutionRef{JobID: labeled.ID}, OutputInput{
		Key: "report", Type: OutputTypeReport, Label: "quarterly summary report",
		Reference: json.RawMessage(`{"link":"/v1/report/1"}`),
	}); err != nil {
		t.Fatalf("publish output: %v", err)
	}

	search := func(term string) []string {
		t.Helper()
		return pageIDs(listFor(t, svc, deps, admin, Filter{Search: term}, Cursor{}, 0))
	}

	requireIDs(t, "uuid", search(sealed.ID), sealed.ID)
	requireIDs(t, "title", search("quarterly recording"), sealed.ID)
	// The sanitized summary the Kind produced: scheme, host and file name.
	requireIDs(t, "sanitized summary", search("files.example.test"), sealed.ID)
	requireIDs(t, "sanitized failure message", search("storage volume"), failed.ID)
	requireIDs(t, "output label", search("report"), labeled.ID)

	// The protected diagnostic reference is not searchable, and neither is the
	// ciphertext beside the Job nor anything only its plaintext holds.
	for _, term := range []string{"path-4f2a", fixtureQuerySecret, fixtureCookieSecret, fixtureAuthSecret, fixturePluginSecret} {
		if got := search(term); len(got) != 0 {
			t.Errorf("search for %q returned %v, want nothing", term, got)
		}
	}

	// A term no stored text can hold matches nothing rather than failing: a NUL
	// and an invalid UTF-8 byte are both refused by PostgreSQL inside a text
	// parameter.
	for _, term := range []string{"\x00", "\xff\xfe"} {
		if got := search(term); len(got) != 0 {
			t.Errorf("search for %q returned %v, want nothing", term, got)
		}
	}
	// And the same text is absent from the database file itself.
	requireSecretsAbsentFromDisk(t, dsn)
}

// TestListRefusesQuestionsItCannotAnswer covers the refusals: a listing answers
// what it can and says so when it cannot, rather than returning a page that
// looks like "no matches".
func TestListRefusesQuestionsItCannotAnswer(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	admin := Access{UserID: 1, Administrator: true}
	acceptFor(t, svc, deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api", Title: "one job",
		Replay: ReplayInput{NonReplayable: true},
	})

	cases := []struct {
		name   string
		filter Filter
		cursor Cursor
		limit  int
		want   error
	}{
		{name: "unknown state", filter: Filter{States: []string{"finished"}}, want: ErrInvalidFilter},
		{name: "empty kind", filter: Filter{Kinds: []string{" "}}, want: ErrInvalidFilter},
		{name: "inverted window", filter: Filter{AcceptedAfter: timePtr(time.Date(2032, 1, 1, 0, 0, 0, 0, time.UTC)), AcceptedBefore: timePtr(time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC))}, want: ErrInvalidFilter},
		{name: "command dimension", filter: Filter{Command: "cancel"}, want: ErrInvalidFilter},
		{name: "cursor without identity", cursor: Cursor{AcceptedAt: time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)}, want: ErrInvalidCursor},
		{name: "cursor without instant", cursor: Cursor{ID: "5c7b5b4a-0000-7000-8000-000000000000"}, want: ErrInvalidCursor},
		{name: "negative page", limit: -1, want: ErrInvalidPage},
		{name: "page over the ceiling", limit: MaxPageSize + 1, want: ErrInvalidPage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.List(deps, admin, tc.filter, tc.cursor, tc.limit)
			if !errors.Is(err, tc.want) {
				t.Fatalf("List = %v, want %v", err, tc.want)
			}
		})
	}

	// The default page size is the ceiling-free case, and a full page still
	// reports the end of the listing rather than a next page that does not exist.
	page := listFor(t, svc, deps, admin, Filter{}, Cursor{}, 0)
	if len(page.Jobs) != 1 || page.Next != nil {
		t.Fatalf("page = %d jobs, next = %v", len(page.Jobs), page.Next)
	}
}

func timePtr(t time.Time) *time.Time { return &t }

// TestCursorPagesNewestFirstAndSkipsNothingWhenHistoryGrows walks a listing
// while a newer Job is accepted in the middle of it. An offset would shift every
// later row down by one and skip a Job; a keyset cursor cannot.
func TestCursorPagesNewestFirstAndSkipsNothingWhenHistoryGrows(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 6, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(title string) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	oldest := accept("1")
	second := accept("2")
	third := accept("3")
	fourth := accept("4")
	fifth := accept("5")

	admin := Access{UserID: 1, Administrator: true}
	seen := make([]string, 0, 5)
	page := listFor(t, svc, deps, admin, Filter{}, Cursor{}, 2)
	seen = append(seen, pageIDs(page)...)
	requireIDs(t, "first page", seen, fifth.ID, fourth.ID)
	if page.Next == nil {
		t.Fatal("a full page must report where to continue from")
	}

	// History grows underneath the reader: the newest Job is accepted after the
	// first page was served.
	newest := accept("6")

	page = listFor(t, svc, deps, admin, Filter{}, *page.Next, 2)
	seen = append(seen, pageIDs(page)...)
	requireIDs(t, "second page", pageIDs(page), third.ID, second.ID)
	whileGrowing := page.Next
	if whileGrowing == nil {
		t.Fatal("the second page must report where to continue from")
	}

	page = listFor(t, svc, deps, admin, Filter{}, *whileGrowing, 2)
	seen = append(seen, pageIDs(page)...)
	requireIDs(t, "third page", pageIDs(page), oldest.ID)
	if page.Next != nil {
		t.Fatalf("the listing reported a next page after its last row: %+v", page.Next)
	}

	requireIDs(t, "every older job exactly once", seen, fifth.ID, fourth.ID, third.ID, second.ID, oldest.ID)
	// The Job accepted mid-walk is ahead of the cursor, so it is not spliced
	// into a page that had already been served: a reader that wants it asks
	// again from the start, and the cursor never renumbers a page.
	for _, id := range seen {
		if id == newest.ID {
			t.Fatal("a job accepted after the first page appeared inside the walk")
		}
	}
	requireIDs(t, "a fresh listing sees it", pageIDs(listFor(t, svc, deps, admin, Filter{}, Cursor{}, 0)),
		newest.ID, fifth.ID, fourth.ID, third.ID, second.ID, oldest.ID)
}

func boolPtr(v bool) *bool { return &v }

// TestDismissChangesOnlyThatViewersDefaultList proves dismissal is a view rather
// than a fact about the Job: it hides the Job from the list one viewer is
// looking at, leaves the Job — and its history — exactly as it was for everybody
// else, and is reversible.
func TestDismissChangesOnlyThatViewersDefaultList(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 7, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := acceptFor(t, svc, deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Title: "one finished job",
		Replay: ReplayInput{NonReplayable: true},
	})
	viewer := Access{UserID: 7}
	other := Access{UserID: 1, Administrator: true}

	// The default list is the one that asks to be shown undismissed Jobs.
	undismissed := Filter{Dismissed: boolPtr(false)}
	requireIDs(t, "before dismissal", pageIDs(listFor(t, svc, deps, viewer, undismissed, Cursor{}, 0)), job.ID)

	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: job.ID, Dismissed: boolPtr(true)}); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	requireIDs(t, "after dismissal", pageIDs(listFor(t, svc, deps, viewer, undismissed, Cursor{}, 0)))
	requireIDs(t, "dismissed only", pageIDs(listFor(t, svc, deps, viewer, Filter{Dismissed: boolPtr(true)}, Cursor{}, 0)), job.ID)
	requireIDs(t, "no dismissal filter", pageIDs(listFor(t, svc, deps, viewer, Filter{}, Cursor{}, 0)), job.ID)

	// Another viewer's default list is untouched, and the Job is still there.
	requireIDs(t, "another viewer", pageIDs(listFor(t, svc, deps, other, undismissed, Cursor{}, 0)), job.ID)
	if _, err := svc.Get(deps, viewer, job.ID); err != nil {
		t.Fatalf("a dismissed job must still be readable: %v", err)
	}
	if events := jobEvents(t, deps, job.ID); len(events) == 0 {
		t.Fatal("dismissal deleted history")
	}

	// Clearing it puts the Job back, and replaying the same request is the same
	// state rather than an error.
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: job.ID, Dismissed: boolPtr(true)}); err != nil {
		t.Fatalf("dismiss twice: %v", err)
	}
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: job.ID, Dismissed: boolPtr(false)}); err != nil {
		t.Fatalf("undismiss: %v", err)
	}
	requireIDs(t, "after undismissal", pageIDs(listFor(t, svc, deps, viewer, undismissed, Cursor{}, 0)), job.ID)
	requireIDs(t, "nothing is dismissed any more", pageIDs(listFor(t, svc, deps, viewer, Filter{Dismissed: boolPtr(true)}, Cursor{}, 0)))

	// A preference that names nothing to change is a caller bug, not a no-op.
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: job.ID}); !errors.Is(err, ErrInvalidPreference) {
		t.Fatalf("an empty preference request = %v, want ErrInvalidPreference", err)
	}
}

// TestPinObservesThePerUserLimitAndAnswersTheViewersOwnRows covers both halves of
// §9's pin: it is the viewer's own row (so one viewer's list is what changes),
// and it is bounded per viewer so a list cannot exempt unbounded history from
// expiry.
func TestPinObservesThePerUserLimitAndAnswersTheViewersOwnRows(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 8, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	deps.PinLimit = 2

	accept := func(owner uint, title string) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: uintPtr(owner), Title: title,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	first := accept(7, "first")
	second := accept(7, "second")
	third := accept(7, "third")
	foreign := accept(8, "another viewer's job")

	viewer := Access{UserID: 7}
	for _, job := range []Snapshot{first, second} {
		if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: job.ID, Pinned: boolPtr(true)}); err != nil {
			t.Fatalf("pin %s: %v", job.Title, err)
		}
	}
	// Re-pinning a Job the viewer already pinned takes no second slot.
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: first.ID, Pinned: boolPtr(true)}); err != nil {
		t.Fatalf("pin twice: %v", err)
	}
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: third.ID, Pinned: boolPtr(true)}); !errors.Is(err, ErrPinLimitReached) {
		t.Fatalf("the third pin = %v, want ErrPinLimitReached", err)
	}

	requireIDs(t, "pinned", pageIDs(listFor(t, svc, deps, viewer, Filter{Pinned: boolPtr(true)}, Cursor{}, 0)),
		second.ID, first.ID)
	requireIDs(t, "unpinned", pageIDs(listFor(t, svc, deps, viewer, Filter{Pinned: boolPtr(false)}, Cursor{}, 0)),
		third.ID)

	// The limit is per user, not per deployment: another viewer pins their own
	// Job while this one is full.
	elsewhere := Access{UserID: 8}
	if err := svc.SetPreference(deps, elsewhere, PreferenceRequest{JobID: foreign.ID, Pinned: boolPtr(true)}); err != nil {
		t.Fatalf("another viewer's pin: %v", err)
	}
	requireIDs(t, "the other viewer's pins", pageIDs(listFor(t, svc, deps, elsewhere, Filter{Pinned: boolPtr(true)}, Cursor{}, 0)), foreign.ID)
	// ... and a preference cannot be recorded for a Job that viewer cannot see,
	// because a preference grants no visibility.
	if err := svc.SetPreference(deps, elsewhere, PreferenceRequest{JobID: first.ID, Dismissed: boolPtr(true)}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a preference on an invisible job = %v, want ErrNotFound", err)
	}

	// Unpinning frees the slot.
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: first.ID, Pinned: boolPtr(false)}); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: third.ID, Pinned: boolPtr(true)}); err != nil {
		t.Fatalf("pin after freeing a slot: %v", err)
	}

	// A preference needs a viewer to belong to.
	if err := svc.SetPreference(deps, Access{Administrator: true}, PreferenceRequest{JobID: third.ID, Pinned: boolPtr(true)}); !errors.Is(err, ErrInvalidPreference) {
		t.Fatalf("a preference for a principal with no user = %v, want ErrInvalidPreference", err)
	}
	// And it names a Job that exists.
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{
		JobID: "5c7b5b4a-0000-7000-8000-000000000000", Pinned: boolPtr(true),
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a preference on a missing job = %v, want ErrNotFound", err)
	}
}

// TestPreferenceIsRefusedOnceTheViewerIsDeleted is the rule at the seam that
// carries it out: an admission belongs to a viewer who still exists, and the
// viewer's deletion says so durably — the tombstone it leaves on the very fence
// admission takes, written in the transaction that sweeps their preferences.
//
// The property is not "the account row is gone": an admission is a viewer-keyed
// row that grants no visibility, so a deleted viewer's id would still be a valid
// key for it, and a pin among those rows exempts the Job's metadata and events
// from retention for everybody, forever, with nobody left who could unpin it.
func TestPreferenceIsRefusedOnceTheViewerIsDeleted(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 10, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := acceptQueued(t, svc, deps, uintPtr(7))
	viewer := Access{UserID: 7}
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: job.ID, Pinned: boolPtr(true)}); err != nil {
		t.Fatalf("pin: %v", err)
	}

	// The deletion's own transaction: the fence is tombstoned and the viewer's
	// preferences go with it, atomically and before anything else about the
	// viewer is touched.
	if err := deps.DB.Transaction(func(tx *gorm.DB) error {
		return DeleteViewerPreferences(tx, viewer.UserID, clock.Add(time.Hour))
	}); err != nil {
		t.Fatalf("delete the viewer's preferences: %v", err)
	}
	if rows := countRows(t, deps, &models.JobPreference{}, "user_id = ?", viewer.UserID); rows != 0 {
		t.Fatalf("the deleted viewer kept %d preference rows", rows)
	}

	// The next admission is refused, and writes nothing: a refusal is a refusal
	// rather than a row that arrives anyway.
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: job.ID, Pinned: boolPtr(true)}); !errors.Is(err, ErrViewerDeleted) {
		t.Fatalf("an admission for a deleted viewer = %v, want ErrViewerDeleted", err)
	}
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: job.ID, Dismissed: boolPtr(true)}); !errors.Is(err, ErrViewerDeleted) {
		t.Fatalf("a dismissal for a deleted viewer = %v, want ErrViewerDeleted", err)
	}
	if rows := countRows(t, deps, &models.JobPreference{}, "user_id = ?", viewer.UserID); rows != 0 {
		t.Fatalf("the refused admissions wrote %d preference rows", rows)
	}
	// Another viewer's admission is untouched by it.
	if err := svc.SetPreference(deps, Access{UserID: 8, Administrator: true}, PreferenceRequest{JobID: job.ID, Pinned: boolPtr(true)}); err != nil {
		t.Fatalf("another viewer's pin: %v", err)
	}
}

// TestVisibilityHidesAJobFromEveryOtherReadPath is the same predicate reached
// through the readers that join onto a Job. A hidden Job's timeline, outputs,
// lineage and published events are indistinguishable from a Job that does not
// exist, and it is not counted in an aggregate either.
func TestVisibilityHidesAJobFromEveryOtherReadPath(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 9, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	hidden := acceptFor(t, svc, deps, Acceptance{
		Kind: "plugin-command", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Title: "a command run", Visibility: VisibilityAdmin,
		Replay: ReplayInput{NonReplayable: true},
	})
	relative := acceptFor(t, svc, deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Title: "its retry",
		Replay: ReplayInput{NonReplayable: true},
	})
	if err := svc.Link(deps, LinkRequest{Type: LinkRetryOf, FromJobID: relative.ID, ToJobID: hidden.ID}); err != nil {
		t.Fatalf("link: %v", err)
	}
	ordinary := Access{UserID: 7}

	if _, err := svc.Timeline(deps, ordinary, hidden.ID, 0, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("Timeline on a hidden job = %v, want ErrNotFound", err)
	}
	if _, err := svc.Outputs(deps, ordinary, hidden.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Outputs on a hidden job = %v, want ErrNotFound", err)
	}
	lineage, err := svc.Lineage(deps, ordinary, relative.ID)
	if err != nil {
		t.Fatalf("Lineage on a visible job: %v", err)
	}
	if len(lineage.Ancestors) != 0 {
		t.Errorf("a hidden ancestor was named: %+v", lineage.Ancestors)
	}
	if _, err := svc.Lineage(deps, ordinary, hidden.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Lineage on a hidden job = %v, want ErrNotFound", err)
	}
	// The command preflight is the detail read: it resolves one immutable Job
	// under the shared predicate before any adapter is asked anything.
	if _, err := svc.Get(deps, ordinary, hidden.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("command preflight on a hidden job = %v, want ErrNotFound", err)
	}

	// The resumable event scan is the same predicate on the joined Job.
	if _, err := svc.PublishPendingEvents(deps, 100); err != nil {
		t.Fatalf("publish events: %v", err)
	}
	events, err := svc.PublishedEvents(deps, ordinary, 0, 0)
	if err != nil {
		t.Fatalf("PublishedEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("the visible job published no events, so the exclusion proves nothing")
	}
	for _, event := range events {
		if event.JobID == hidden.ID {
			t.Errorf("catch-up delivered an event of a hidden job: %+v", event)
		}
	}
}

// TestTimelineReturnsTheJobsOrderedBoundedTimeline covers the timeline read: a
// Job's own events in order, continued from a sequence, and bounded.
func TestTimelineReturnsTheJobsOrderedBoundedTimeline(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 10, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := acceptFor(t, svc, deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Title: "a job",
		Replay: ReplayInput{NonReplayable: true},
	})
	viewer := Access{UserID: 7}
	job = advanceReplayJob(t, svc, deps, job, StateRunning)
	job = advanceReplayJob(t, svc, deps, job, StateSucceeded)

	events, err := svc.Timeline(deps, viewer, job.ID, 0, 0)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("timeline = %d events, want accepted, started, succeeded", len(events))
	}
	for i, event := range events {
		if event.Sequence != uint64(i+1) {
			t.Errorf("event %d has sequence %d", i, event.Sequence)
		}
		if event.JobID != job.ID || event.ID == "" {
			t.Errorf("event %d = %+v", i, event)
		}
	}
	if events[0].Type != EventAccepted || events[2].Type != EventSucceeded {
		t.Errorf("timeline types = %q..%q", events[0].Type, events[2].Type)
	}
	if !events[0].ReservedHost {
		t.Error("a lifecycle event must be marked reserved")
	}

	// Continuing from a sequence is exclusive, so a client that has consumed an
	// event never sees it twice.
	tail, err := svc.Timeline(deps, viewer, job.ID, 1, 0)
	if err != nil {
		t.Fatalf("Timeline from a sequence: %v", err)
	}
	if len(tail) != 2 || tail[0].Sequence != 2 {
		t.Fatalf("timeline after sequence 1 = %+v", tail)
	}

	bounded, err := svc.Timeline(deps, viewer, job.ID, 0, 1)
	if err != nil {
		t.Fatalf("bounded Timeline: %v", err)
	}
	if len(bounded) != 1 {
		t.Fatalf("bounded timeline = %d events, want 1", len(bounded))
	}
	page, err := svc.Timeline(deps, viewer, job.ID, 0, MaxEventPageSize+1)
	if err != nil && !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("an oversized timeline = %v, want ErrInvalidPage", err)
	}
	if err == nil && len(page) > 0 {
		t.Fatalf("an oversized timeline was served: %d events", len(page))
	}
	// A hidden Job and a missing one are the same answer, and so are a hidden
	// Job's events.
	if _, err := svc.Timeline(deps, Access{UserID: 8}, job.ID, 0, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Timeline for a foreign viewer = %v, want ErrNotFound", err)
	}
}

// TestOutputsReturnsTheJobsTypedOutputsAndRefusesAHiddenJob covers the output
// lookup the detail read and the artifact routes share.
func TestOutputsReturnsTheJobsTypedOutputsAndRefusesAHiddenJob(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	job := acceptFor(t, svc, deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Title: "an export",
		Replay: ReplayInput{NonReplayable: true},
	})
	ref := ExecutionRef{JobID: job.ID}
	if _, err := svc.PublishOutput(deps, ref, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Label: "the tar",
		Reference: json.RawMessage(`{"name":"export.tar"}`), Required: true,
	}); err != nil {
		t.Fatalf("publish required output: %v", err)
	}
	if _, err := svc.PublishOutput(deps, ref, OutputInput{
		Key: "report", Type: OutputTypeReport, Label: "the plan",
		Reference: json.RawMessage(`{"summary":"3 groups"}`),
	}); err != nil {
		t.Fatalf("publish optional output: %v", err)
	}

	outputs, err := svc.Outputs(deps, Access{UserID: 7}, job.ID)
	if err != nil {
		t.Fatalf("Outputs: %v", err)
	}
	if len(outputs) != 2 {
		t.Fatalf("outputs = %d, want 2", len(outputs))
	}
	if outputs[0].Key != "artifact" || !outputs[0].Required || outputs[0].Availability != OutputAvailable {
		t.Errorf("first output = %+v", outputs[0])
	}
	if string(outputs[0].Reference) != `{"name":"export.tar"}` {
		t.Errorf("reference = %s", outputs[0].Reference)
	}
	if _, err := svc.Outputs(deps, Access{UserID: 8}, job.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Outputs for a foreign viewer = %v, want ErrNotFound", err)
	}
}

// TestLineageNamesOnlyVisibleRelatives covers the lineage read: one hop of the
// relations, each relative authorized independently, so a hidden relative is
// neither named nor counted.
func TestLineageNamesOnlyVisibleRelatives(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 11, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(title string, owner *uint, class VisibilityClass) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: owner, Title: title, Visibility: class,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	ancestor := accept("ancestor", uintPtr(7), "")
	middle := accept("middle", uintPtr(7), "")
	leaf := accept("leaf", uintPtr(7), "")
	hiddenAncestor := accept("hidden ancestor", uintPtr(7), VisibilityAdmin)
	child := accept("child", uintPtr(7), "")
	hiddenChild := accept("hidden child", uintPtr(7), VisibilityAdmin)

	for _, link := range []LinkRequest{
		{Type: LinkRetryOf, FromJobID: middle.ID, ToJobID: ancestor.ID},
		{Type: LinkRetryOf, FromJobID: leaf.ID, ToJobID: middle.ID},
		{Type: LinkRepeatOf, FromJobID: leaf.ID, ToJobID: hiddenAncestor.ID},
		{Type: LinkParentChild, FromJobID: middle.ID, ToJobID: child.ID},
		{Type: LinkParentChild, FromJobID: middle.ID, ToJobID: hiddenChild.ID},
	} {
		if err := svc.Link(deps, link); err != nil {
			t.Fatalf("link %+v: %v", link, err)
		}
	}

	lineage, err := svc.Lineage(deps, Access{UserID: 7}, leaf.ID)
	if err != nil {
		t.Fatalf("Lineage: %v", err)
	}
	if lineage.Job.ID != leaf.ID {
		t.Fatalf("lineage named %s, want %s", lineage.Job.ID, leaf.ID)
	}
	// leaf is a one-hop ancestor of itself through two relations: middle (retry)
	// and the hidden repeat ancestor, which is not named.
	requireIDs(t, "ancestors", idsOf(lineage.Ancestors), middle.ID)
	requireIDs(t, "parents", idsOf(lineage.Parents))
	requireIDs(t, "successors", idsOf(lineage.Successors))
	requireIDs(t, "children", idsOf(lineage.Children))

	middleLineage, err := svc.Lineage(deps, Access{UserID: 7}, middle.ID)
	if err != nil {
		t.Fatalf("Lineage of the middle job: %v", err)
	}
	requireIDs(t, "successors of the middle job", idsOf(middleLineage.Successors), leaf.ID)
	requireIDs(t, "children of the middle job", idsOf(middleLineage.Children), child.ID)
	requireIDs(t, "ancestors of the middle job", idsOf(middleLineage.Ancestors), ancestor.ID)

	// An administrator sees the hidden relatives the ordinary viewer cannot.
	adminLineage, err := svc.Lineage(deps, Access{UserID: 1, Administrator: true}, leaf.ID)
	if err != nil {
		t.Fatalf("Lineage as an administrator: %v", err)
	}
	requireIDs(t, "an administrator's ancestors", idsOf(adminLineage.Ancestors), hiddenAncestor.ID, middle.ID)
	// A hidden relative is not counted either: the ordinary viewer's list has no
	// gap and no placeholder.
	if len(lineage.Ancestors) != 1 {
		t.Fatalf("a hidden relative was counted: %d ancestors", len(lineage.Ancestors))
	}
}

func idsOf(snapshots []Snapshot) []string {
	ids := make([]string, 0, len(snapshots))
	for _, snap := range snapshots {
		ids = append(ids, snap.ID)
	}
	return ids
}

// TestPublishedEventsCatchesUpFromACursorInDeliveryOrder covers the resumable
// scan the SSE handler's correctness rests on: only published events, only
// visible Jobs, delivered in delivery-sequence order and continued from a
// cursor.
func TestPublishedEventsCatchesUpFromACursorInDeliveryOrder(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 12, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	first := acceptFor(t, svc, deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Title: "first",
		Replay: ReplayInput{NonReplayable: true},
	})
	second := acceptFor(t, svc, deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Title: "second",
		Replay: ReplayInput{NonReplayable: true},
	})
	admin := Access{UserID: 1, Administrator: true}

	// Nothing has been published yet: acceptance records the event, the
	// publisher assigns its delivery sequence afterwards.
	events, err := svc.PublishedEvents(deps, admin, 0, 0)
	if err != nil {
		t.Fatalf("PublishedEvents: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("an unpublished event was delivered: %+v", events)
	}

	if _, err := svc.PublishPendingEvents(deps, 100); err != nil {
		t.Fatalf("publish: %v", err)
	}
	events, err = svc.PublishedEvents(deps, admin, 0, 0)
	if err != nil {
		t.Fatalf("PublishedEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("published events = %d, want 2", len(events))
	}
	var last uint64
	for _, event := range events {
		if event.DeliverySequence == nil {
			t.Fatalf("a delivered event has no delivery sequence: %+v", event)
		}
		if *event.DeliverySequence < last {
			t.Fatalf("delivery sequences went backwards: %d after %d", *event.DeliverySequence, last)
		}
		last = *event.DeliverySequence
	}
	// Both Jobs' accepted events are delivered, and the cursor continues from
	// exactly the one it names — which Job the publisher numbered first is not a
	// contract, but which event the cursor skips is.
	delivered := map[string]bool{}
	for _, event := range events {
		delivered[event.JobID] = true
	}
	if !delivered[first.ID] || !delivered[second.ID] {
		t.Fatalf("catch-up delivered %v, want both accepted events", delivered)
	}

	cursor := *events[0].DeliverySequence
	tail, err := svc.PublishedEvents(deps, admin, cursor, 0)
	if err != nil {
		t.Fatalf("PublishedEvents from a cursor: %v", err)
	}
	if len(tail) != 1 {
		t.Fatalf("catch-up after %d = %+v, want exactly the remaining event", cursor, tail)
	}
	if tail[0].JobID == events[0].JobID {
		t.Fatalf("the cursor re-delivered the event it named: %+v", tail)
	}
	if tail[0].JobID != events[1].JobID {
		t.Fatalf("catch-up after %d = %s, want %s", cursor, tail[0].JobID, events[1].JobID)
	}

	// A viewer who may not see the Jobs is told nothing at all.
	hidden, err := svc.PublishedEvents(deps, Access{UserID: 9}, 0, 0)
	if err != nil {
		t.Fatalf("PublishedEvents for a foreign viewer: %v", err)
	}
	if len(hidden) != 0 {
		t.Fatalf("a foreign viewer received %d events", len(hidden))
	}

	if _, err := svc.PublishedEvents(deps, admin, 0, MaxEventPageSize+1); !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("an oversized catch-up = %v, want ErrInvalidPage", err)
	}
	bounded, err := svc.PublishedEvents(deps, admin, 0, 1)
	if err != nil {
		t.Fatalf("bounded PublishedEvents: %v", err)
	}
	if len(bounded) != 1 {
		t.Fatalf("bounded catch-up = %d events, want 1", len(bounded))
	}
}

// TestSummarySharesTheListingsVisibilityPredicateAndFilters is the property that
// makes an aggregate trustworthy: it is the same predicate and the same filter,
// so a Job the listing hides is a Job the counts do not know about either.
func TestSummarySharesTheListingsVisibilityPredicateAndFilters(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 13, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(kind string, owner *uint, class VisibilityClass, title string) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: kind, KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: owner, Title: title, Visibility: class,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	visible := accept("remote-download", uintPtr(7), "", "a visible download")
	accept("group-export", uintPtr(7), "", "a visible export")
	accept("plugin-command", uintPtr(7), VisibilityAdmin, "a hidden command run")
	accept("remote-download", uintPtr(8), "", "somebody else's download")

	viewer := Access{UserID: 7}
	summary, err := svc.Summary(deps, viewer, Filter{}, 0)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Total != 2 {
		t.Fatalf("total = %d, want 2: hidden and foreign jobs were counted", summary.Total)
	}
	if summary.ByKind["remote-download"] != 1 || summary.ByKind["group-export"] != 1 || summary.ByKind["plugin-command"] != 0 {
		t.Errorf("by kind = %v", summary.ByKind)
	}

	filtered, err := svc.Summary(deps, viewer, Filter{Kinds: []string{"remote-download"}}, 0)
	if err != nil {
		t.Fatalf("filtered Summary: %v", err)
	}
	if filtered.Total != 1 || filtered.ByKind["group-export"] != 0 {
		t.Errorf("filtered counts = %v", filtered.ByKind)
	}
	searched, err := svc.Summary(deps, viewer, Filter{Search: "visible export"}, 0)
	if err != nil {
		t.Fatalf("searched Summary: %v", err)
	}
	if searched.Total != 1 || searched.ByKind["group-export"] != 1 {
		t.Errorf("searched counts = %v", searched.ByKind)
	}

	// The listing and the aggregate answer the same filtered set.
	page := listFor(t, svc, deps, viewer, Filter{Kinds: []string{"remote-download"}}, Cursor{}, 0)
	if page.Jobs[0].ID != visible.ID {
		t.Fatalf("the listing and the summary disagree: %+v", pageIDs(page))
	}
}

// TestSummaryWindowDefaultsToThirtyDaysAndRefusesALongerOne pins the analysis
// window: an interactive aggregate is bounded, and a longer question is an
// export Job rather than a request that scans a year of history.
func TestSummaryWindowDefaultsToThirtyDaysAndRefusesALongerOne(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	now := time.Date(2031, 6, 14, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return now }
	acceptFor(t, svc, deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api", Title: "recent",
		Replay: ReplayInput{NonReplayable: true},
	})

	admin := Access{UserID: 1, Administrator: true}
	for _, tc := range []struct {
		name   string
		window time.Duration
		want   time.Duration
	}{
		{"unset", 0, DefaultSummaryWindow},
		{"negative", -time.Hour, DefaultSummaryWindow},
		{"an hour", time.Hour, time.Hour},
		{"the ceiling", MaxSummaryWindow, MaxSummaryWindow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			summary, err := svc.Summary(deps, admin, Filter{}, tc.window)
			if err != nil {
				t.Fatalf("Summary(%v): %v", tc.window, err)
			}
			if summary.Window != tc.want {
				t.Fatalf("window = %v, want %v", summary.Window, tc.want)
			}
			if !summary.To.Equal(now) || !summary.From.Equal(now.Add(-tc.want)) {
				t.Fatalf("window bounds = %v..%v", summary.From, summary.To)
			}
		})
	}

	if _, err := svc.Summary(deps, admin, Filter{}, MaxSummaryWindow+time.Second); !errors.Is(err, ErrInvalidWindow) {
		t.Fatalf("a window past the ceiling = %v, want ErrInvalidWindow", err)
	}

	// The window excludes older accepted work: the same Job is outside a window
	// that ends before it was accepted.
	old := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)
	before := old
	filtered, err := svc.Summary(deps, admin, Filter{AcceptedBefore: &before}, 0)
	if err != nil {
		t.Fatalf("Summary with an older window: %v", err)
	}
	if filtered.Total != 0 {
		t.Fatalf("total = %d, want 0", filtered.Total)
	}
}

// TestSummaryCountsStatesKindsDurationsAndFailures covers what the aggregate
// reports: counts by state and Kind, the success rate over settled work, queue
// and run duration percentiles, and the sanitized failure classifications.
func TestSummaryCountsStatesKindsDurationsAndFailures(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 15, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(kind, title string) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: kind, KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	// wait is the queue time, run is the time spent running.
	settle := func(job Snapshot, wait, run time.Duration, outcome State, failure *Failure) {
		clock = clock.Add(wait)
		job = advanceReplayJob(t, svc, deps, job, StateRunning)
		clock = clock.Add(run)
		next, err := svc.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: job.Version, To: outcome, Failure: failure,
			ExecutionToken: executionTokenOf(t, deps, job.ID),
		})
		if err != nil {
			t.Fatalf("transition to %s: %v", outcome, err)
		}
		_ = next
	}

	settle(accept("group-export", "first"), 10*time.Minute, 5*time.Minute, StateSucceeded, nil)
	settle(accept("group-export", "second"), 20*time.Minute, 10*time.Minute, StateSucceeded, nil)
	settle(accept("group-export", "third"), 30*time.Minute, 15*time.Minute, StateFailed, &Failure{
		Code: "disk-full", Class: FailureClassDependency, Message: "the storage volume was full",
	})
	accept("remote-download", "never ran")

	summary, err := svc.Summary(deps, Access{UserID: 1, Administrator: true}, Filter{}, 0)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Total != 4 {
		t.Fatalf("total = %d, want 4", summary.Total)
	}
	if summary.ByState[string(StateSucceeded)] != 2 || summary.ByState[string(StateFailed)] != 1 || summary.ByState[string(StateQueued)] != 1 {
		t.Errorf("by state = %v", summary.ByState)
	}
	if summary.ByKind["group-export"] != 3 || summary.ByKind["remote-download"] != 1 {
		t.Errorf("by kind = %v", summary.ByKind)
	}
	if summary.Succeeded != 2 || summary.Failed != 1 || summary.Terminal != 3 {
		t.Errorf("outcomes = %d succeeded, %d failed, %d settled", summary.Succeeded, summary.Failed, summary.Terminal)
	}
	if rate := summary.SuccessRate; rate < 0.66 || rate > 0.67 {
		t.Errorf("success rate = %v, want two thirds", rate)
	}
	// Queue duration is measured over every Job in the window: [0, 10m, 20m, 30m].
	if summary.Queue.Median != 10*time.Minute || summary.Queue.P95 != 30*time.Minute {
		t.Errorf("queue durations = %+v", summary.Queue)
	}
	// Run duration is measured over the Jobs that started: [5m, 10m, 15m].
	if summary.Run.Median != 10*time.Minute || summary.Run.P95 != 15*time.Minute {
		t.Errorf("run durations = %+v", summary.Run)
	}
	if len(summary.Failures) != 1 || summary.Failures[0].Class != FailureClassDependency || summary.Failures[0].Count != 1 {
		t.Errorf("failure classes = %+v", summary.Failures)
	}

	// An empty window answers zeroes rather than failing.
	before := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	empty, err := svc.Summary(deps, Access{UserID: 1, Administrator: true}, Filter{AcceptedBefore: &before}, 0)
	if err != nil {
		t.Fatalf("empty Summary: %v", err)
	}
	if empty.Total != 0 || empty.SuccessRate != 0 || empty.Queue.Median != 0 || len(empty.Failures) != 0 {
		t.Errorf("empty summary = %+v", empty)
	}
}

// TestListFiltersByLineageRelationshipWithoutRevealingAHiddenRelative is §8's
// lineage rule applied to the filter dimension.
//
// Lineage does not grant transitive visibility, and a hidden relative is
// neither named nor counted — but the relationship filter tested only that a
// link row existed, so a visible Job whose only relative was hidden still
// matched "is a parent" and the hidden relationship became visible by its
// existence. The predicate asks the shared visibility question about the far
// endpoint, so a listing, an aggregate and the lineage view all answer alike.
func TestListFiltersByLineageRelationshipWithoutRevealingAHiddenRelative(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2032, 6, 7, 8, 9, 10, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(title string, class VisibilityClass) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: uintPtr(7), Title: title, Visibility: class,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	parent := accept("the visible parent", VisibilityOwner)
	hidden := accept("the child nobody may read", VisibilityAdmin)
	if err := svc.Link(deps, LinkRequest{Type: LinkParentChild, FromJobID: parent.ID, ToJobID: hidden.ID}); err != nil {
		t.Fatalf("Link: %v", err)
	}

	viewer := Access{UserID: 7}
	filter := Filter{Relationship: string(LinkParentChild)}

	page := listFor(t, svc, deps, viewer, filter, Cursor{}, 0)
	if len(page.Jobs) != 0 {
		t.Fatalf("a parent whose only child is hidden matched the relationship filter: %v", pageIDs(page))
	}

	summary, err := svc.Summary(deps, viewer, filter, 0)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Total != 0 {
		t.Fatalf("the aggregate counted %d Jobs whose only relationship is hidden", summary.Total)
	}

	// The lineage view already agreed; the filter now agrees with it.
	lineage, err := svc.Lineage(deps, viewer, parent.ID)
	if err != nil {
		t.Fatalf("Lineage: %v", err)
	}
	if len(lineage.Children) != 0 {
		t.Fatalf("lineage named %d hidden children", len(lineage.Children))
	}

	// An administrator sees the relationship, because they can see both ends.
	adminPage := listFor(t, svc, deps, Access{Administrator: true}, filter, Cursor{}, 0)
	if len(adminPage.Jobs) != 1 || adminPage.Jobs[0].ID != parent.ID {
		t.Fatalf("an administrator matched %v, want the parent", pageIDs(adminPage))
	}

	// A visible child keeps the parent matching for everybody: the predicate
	// narrows on the relative's visibility, it does not hide the relation.
	visibleChild := accept("a child the owner may read", VisibilityOwner)
	if err := svc.Link(deps, LinkRequest{Type: LinkParentChild, FromJobID: parent.ID, ToJobID: visibleChild.ID}); err != nil {
		t.Fatalf("Link: %v", err)
	}
	page = listFor(t, svc, deps, viewer, filter, Cursor{}, 0)
	if len(page.Jobs) != 1 || page.Jobs[0].ID != parent.ID {
		t.Fatalf("with a visible child the parent matched %v, want the parent", pageIDs(page))
	}
}

// TestProtectedDiagnosticsStayOutOfOrdinaryProjections is §8's split between
// what a viewer may read and what an administrator's audit keeps. A failure's
// diagnostic reference is an operator-facing pointer into the deployment's own
// infrastructure, and a snapshot is what every ordinary read returns — so a
// projection that carried it handed an internal path to every owner.
func TestProtectedDiagnosticsStayOutOfOrdinaryProjections(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2032, 7, 8, 9, 10, 11, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	const diagnostic = "/var/lib/mahresources/plugin-commands/run-9/stderr.log"
	owner := uintPtr(7)
	failed := acceptFor(t, svc, deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: owner, Title: "an export that broke",
		Replay: ReplayInput{NonReplayable: true},
	})
	failed = advanceReplayJob(t, svc, deps, failed, StateRunning)
	failed, err := svc.Transition(deps, Transition{
		JobID: failed.ID, ExpectedVersion: failed.Version, To: StateFailed,
		ExecutionToken: executionTokenOf(t, deps, failed.ID),
		Failure: &Failure{
			Code: "export-failed", Class: FailureClassInternal,
			Message: "the export could not be assembled", DiagnosticRef: diagnostic,
		},
	})
	if err != nil {
		t.Fatalf("Transition to failed: %v", err)
	}
	related := acceptFor(t, svc, deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: owner, Title: "a later export",
		Replay: ReplayInput{NonReplayable: true},
	})
	if err := svc.Link(deps, LinkRequest{Type: LinkParentChild, FromJobID: related.ID, ToJobID: failed.ID}); err != nil {
		t.Fatalf("Link: %v", err)
	}

	viewer := Access{UserID: 7}
	assertNoDiagnostic := func(what string, snap Snapshot) {
		t.Helper()
		if snap.Failure == nil {
			t.Fatalf("%s: the failure itself must survive", what)
		}
		if snap.Failure.Code != "export-failed" || snap.Failure.Message != "the export could not be assembled" {
			t.Fatalf("%s: the sanitized failure is what a viewer reads: %+v", what, snap.Failure)
		}
		if snap.Failure.DiagnosticRef != "" {
			t.Fatalf("%s: an ordinary projection carried the protected diagnostic %q", what, snap.Failure.DiagnosticRef)
		}
		encoded, err := json.Marshal(snap)
		if err != nil {
			t.Fatalf("%s: marshal snapshot: %v", what, err)
		}
		if strings.Contains(string(encoded), "plugin-commands") {
			t.Fatalf("%s: the serialized snapshot leaked the diagnostic: %s", what, encoded)
		}
	}

	got, err := svc.Get(deps, viewer, failed.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertNoDiagnostic("Get", got)

	page := listFor(t, svc, deps, viewer, Filter{}, Cursor{}, 0)
	if len(page.Jobs) != 2 {
		t.Fatalf("the owner listed %d jobs, want both", len(page.Jobs))
	}
	for _, job := range page.Jobs {
		if job.ID == failed.ID {
			assertNoDiagnostic("List", job)
		}
	}

	lineage, err := svc.Lineage(deps, viewer, related.ID)
	if err != nil {
		t.Fatalf("Lineage: %v", err)
	}
	if len(lineage.Children) != 1 {
		t.Fatalf("lineage named %d children, want the failed one", len(lineage.Children))
	}
	assertNoDiagnostic("Lineage", lineage.Children[0])

	// An administrator's read is where the pointer belongs: it is the address of
	// the detail an operator goes to look at.
	adminRead, err := svc.Get(deps, Access{Administrator: true}, failed.ID)
	if err != nil {
		t.Fatalf("administrator Get: %v", err)
	}
	if adminRead.Failure == nil || adminRead.Failure.DiagnosticRef != diagnostic {
		t.Fatalf("an administrator lost the diagnostic reference: %+v", adminRead.Failure)
	}
}

// TestPreferenceMutationRechecksTheJobItIsWrittenAgainst covers §9's other half:
// a preference is a viewer's row about a Job, so it may not outlive the Job.
//
// The visibility check ran before the transaction opened, and these tables carry
// no foreign keys, so a sweep that removed the Job in that window left the write
// with nothing to point at — and it succeeded, because inserting a row that
// references a Job nobody can read is not an error anywhere.
func TestPreferenceMutationRechecksTheJobItIsWrittenAgainst(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(time.Hour)
	deps.Retention = &policy
	clock := time.Date(2032, 8, 9, 10, 11, 12, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := acceptFor(t, svc, deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Title: "an export that finished long ago",
		Replay: ReplayInput{NonReplayable: true},
	})
	job = advanceReplayJob(t, svc, deps, job, StateRunning)
	job = advanceReplayJob(t, svc, deps, job, StateSucceeded)
	clock = clock.Add(48 * time.Hour)

	// The interleaving: retention takes the Job away between the visibility check
	// and the preference write. The flag is set before the sweep runs, because
	// the sweep's own statements are queries on this table too.
	var pruning atomic.Bool
	deps.DB.Callback().Query().After("gorm:query").Register("test:prune-in-window", func(tx *gorm.DB) {
		if tx.Statement.Table != "jobs" || !pruning.CompareAndSwap(false, true) {
			return
		}
		sweepFor(t, svc, deps, policy, SweepCursor{}, 10)
	})
	err := svc.SetPreference(deps, Access{UserID: 7}, PreferenceRequest{JobID: job.ID, Pinned: boolPtr(true)})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("pinning a Job retention removed = %v, want ErrNotFound", err)
	}
	if rows := countRows(t, deps, &models.JobPreference{}, "job_id = ?", job.ID); rows != 0 {
		t.Fatalf("an orphan pin survived the Job it was about: %d rows", rows)
	}
}

// TestPreferenceRowsGoWithTheJobTheyAreAbout is the pin-before-prune ordering:
// when the viewer's row is there first, retention takes it with the Job rather
// than leaving a row pointing at nothing.
func TestPreferenceRowsGoWithTheJobTheyAreAbout(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	policy := expiredHistory(time.Hour)
	deps.Retention = &policy
	clock := time.Date(2032, 8, 10, 10, 11, 12, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := acceptFor(t, svc, deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Title: "a dismissed export",
		Replay: ReplayInput{NonReplayable: true},
	})
	job = advanceReplayJob(t, svc, deps, job, StateRunning)
	job = advanceReplayJob(t, svc, deps, job, StateSucceeded)
	if err := svc.SetPreference(deps, Access{UserID: 7}, PreferenceRequest{JobID: job.ID, Dismissed: boolPtr(true)}); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if rows := countRows(t, deps, &models.JobPreference{}, "job_id = ?", job.ID); rows != 1 {
		t.Fatalf("the dismissal was not stored: %d rows", rows)
	}

	clock = clock.Add(48 * time.Hour)
	sweepFor(t, svc, deps, policy, SweepCursor{}, 10)

	if jobExists(t, deps, job.ID) {
		t.Fatal("the expired Job survived the sweep")
	}
	if rows := countRows(t, deps, &models.JobPreference{}, "job_id = ?", job.ID); rows != 0 {
		t.Fatalf("the viewer's row outlived the Job: %d rows", rows)
	}
}
