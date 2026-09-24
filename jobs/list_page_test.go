package jobs

import (
	"testing"
	"time"
)

// listBeforeFor runs one backward listing and fails the test on an unexpected
// error.
func listBeforeFor(t *testing.T, svc *Service, deps Deps, access Access, filter Filter, before Cursor, limit int) Page {
	t.Helper()
	page, err := svc.ListBefore(deps, access, filter, before, limit)
	if err != nil {
		t.Fatalf("ListBefore(%+v): %v", before, err)
	}
	return page
}

// TestListBeforeWalksBackToTheFirstPage walks a listing forward and then back.
// Each Previous must return exactly the page that preceded it, and the walk back
// must end on the true first page rather than a short slice of it: a page that
// is the top of the listing reports no Previous.
func TestListBeforeWalksBackToTheFirstPage(t *testing.T) {
	testListBeforeWalksBackToTheFirstPage(t, newTestDeps(t))
}

func testListBeforeWalksBackToTheFirstPage(t *testing.T, deps Deps) {
	svc := NewService()
	clock := time.Date(2031, 7, 1, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(title string) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	one, two, three, four, five := accept("1"), accept("2"), accept("3"), accept("4"), accept("5")
	admin := Access{UserID: 1, Administrator: true}

	first := listFor(t, svc, deps, admin, Filter{}, Cursor{}, 2)
	requireIDs(t, "first page", pageIDs(first), five.ID, four.ID)
	if first.Prev != nil {
		t.Fatalf("the first page reported a previous page: %+v", first.Prev)
	}
	second := listFor(t, svc, deps, admin, Filter{}, *first.Next, 2)
	requireIDs(t, "second page", pageIDs(second), three.ID, two.ID)
	if second.Prev == nil {
		t.Fatal("a page after the first must report where Previous starts")
	}
	third := listFor(t, svc, deps, admin, Filter{}, *second.Next, 2)
	requireIDs(t, "third page", pageIDs(third), one.ID)

	back := listBeforeFor(t, svc, deps, admin, Filter{}, *third.Prev, 2)
	requireIDs(t, "back from the third page", pageIDs(back), three.ID, two.ID)
	if back.Next == nil || back.Prev == nil {
		t.Fatalf("a middle page walked back to must link both ways: next=%v prev=%v", back.Next, back.Prev)
	}
	forward := listFor(t, svc, deps, admin, Filter{}, *back.Next, 2)
	requireIDs(t, "forward again", pageIDs(forward), one.ID)

	top := listBeforeFor(t, svc, deps, admin, Filter{}, *back.Prev, 2)
	requireIDs(t, "back to the top", pageIDs(top), five.ID, four.ID)
	if top.Prev != nil {
		t.Fatalf("the top page walked back to reported a previous page: %+v", top.Prev)
	}
	if top.Next == nil {
		t.Fatal("the top page must still continue forward")
	}
}

// TestListBeforeReturnsAFullFirstPageWhenFewerNewerJobsRemain covers the case a
// short backward page would get wrong. Walked forward two at a time, the second
// page starts at Job 2; asked for the three rows before it, only Jobs 3 and 4
// remain. The answer is the full first page of three — never a two-row slice of
// the top that renders as a page shorter than its neighbours.
func TestListBeforeReturnsAFullFirstPageWhenFewerNewerJobsRemain(t *testing.T) {
	testListBeforeReturnsAFullFirstPageWhenFewerNewerJobsRemain(t, newTestDeps(t))
}

func testListBeforeReturnsAFullFirstPageWhenFewerNewerJobsRemain(t *testing.T, deps Deps) {
	svc := NewService()
	clock := time.Date(2031, 7, 2, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(title string) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	accept("1")
	two := accept("2")
	three := accept("3")
	four := accept("4")
	admin := Access{UserID: 1, Administrator: true}

	first := listFor(t, svc, deps, admin, Filter{}, Cursor{}, 2)
	second := listFor(t, svc, deps, admin, Filter{}, *first.Next, 2)
	if second.Prev == nil {
		t.Fatal("the second page must report where Previous starts")
	}
	top := listBeforeFor(t, svc, deps, admin, Filter{}, *second.Prev, 3)
	requireIDs(t, "top page", pageIDs(top), four.ID, three.ID, two.ID)
	if top.Prev != nil {
		t.Fatalf("the top page reported a previous page: %+v", top.Prev)
	}
}

// TestListBeforeKeepsTheFilterAndVisibility proves the backward read is the same
// question as the forward one: a hidden Job and a filtered-out Job stay out of
// the page Previous returns.
func TestListBeforeKeepsTheFilterAndVisibility(t *testing.T) {
	testListBeforeKeepsTheFilterAndVisibility(t, newTestDeps(t))
}

func testListBeforeKeepsTheFilterAndVisibility(t *testing.T, deps Deps) {
	svc := NewService()
	clock := time.Date(2031, 7, 3, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(kind, title string, owner uint) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: kind, KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			OwnerUserID: uintPtr(owner), Replay: ReplayInput{NonReplayable: true},
		})
	}
	a := accept("remote-download", "a", 7)
	accept("group-export", "filtered out", 7)
	b := accept("remote-download", "b", 7)
	accept("remote-download", "somebody else's", 8)
	c := accept("remote-download", "c", 7)

	viewer := Access{UserID: 7}
	filter := Filter{Kinds: []string{"remote-download"}}
	first := listFor(t, svc, deps, viewer, filter, Cursor{}, 1)
	requireIDs(t, "first", pageIDs(first), c.ID)
	second := listFor(t, svc, deps, viewer, filter, *first.Next, 1)
	requireIDs(t, "second", pageIDs(second), b.ID)
	third := listFor(t, svc, deps, viewer, filter, *second.Next, 1)
	requireIDs(t, "third", pageIDs(third), a.ID)

	back := listBeforeFor(t, svc, deps, viewer, filter, *third.Prev, 1)
	requireIDs(t, "back", pageIDs(back), b.ID)
}

// TestCountByStateHasNoWindowAndSharesTheListingsFilter is the sidebar's rule: a
// count is the number of rows its link opens. It therefore applies the same
// visibility and filter the listing does, and no summary window — a failure
// older than thirty days still needs attention and must still be counted.
func TestCountByStateHasNoWindowAndSharesTheListingsFilter(t *testing.T) {
	testCountByStateHasNoWindowAndSharesTheListingsFilter(t, newTestDeps(t))
}

func testCountByStateHasNoWindowAndSharesTheListingsFilter(t *testing.T, deps Deps) {
	svc := NewService()
	clock := time.Date(2031, 7, 4, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(kind, title string, owner uint) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: kind, KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			OwnerUserID: uintPtr(owner), Replay: ReplayInput{NonReplayable: true},
		})
	}
	old := accept("remote-download", "an old failure", 7)
	old = advanceReplayJob(t, svc, deps, old, StateRunning)
	if _, err := svc.Transition(deps, Transition{
		JobID: old.ID, ExpectedVersion: old.Version, To: StateFailed,
		Failure:        &Failure{Code: "gone", Class: FailureClassDependency, Message: "gone"},
		ExecutionToken: executionTokenOf(t, deps, old.ID),
	}); err != nil {
		t.Fatalf("fail the old job: %v", err)
	}
	clock = clock.Add(60 * 24 * time.Hour)
	accept("remote-download", "queued", 7)
	accept("group-export", "queued export", 7)
	accept("remote-download", "somebody else's", 8)

	viewer := Access{UserID: 7}
	counts, err := svc.CountByState(deps, viewer, Filter{})
	if err != nil {
		t.Fatalf("CountByState: %v", err)
	}
	if counts[string(StateFailed)] != 1 || counts[string(StateQueued)] != 2 {
		t.Fatalf("counts = %v, want failed=1 (older than any window) and queued=2 (own jobs only)", counts)
	}

	filtered, err := svc.CountByState(deps, viewer, Filter{Kinds: []string{"remote-download"}})
	if err != nil {
		t.Fatalf("filtered CountByState: %v", err)
	}
	if filtered[string(StateQueued)] != 1 || filtered[string(StateFailed)] != 1 {
		t.Fatalf("filtered counts = %v", filtered)
	}

	if _, err := svc.CountByState(deps, viewer, Filter{States: []string{"not-a-state"}}); err == nil {
		t.Fatal("an unknown state must be refused, as the listing refuses it")
	}
}

// TestAnEmptiedPageStillLeadsBack covers a reader whose page emptied under them:
// every Job on the last page was dismissed, or finished and left a State filter.
// The page has no row to take a Previous cursor from, but the earlier pages still
// hold matches, so the answer must still lead back rather than strand the reader
// on an empty page with no navigation.
func TestAnEmptiedPageStillLeadsBack(t *testing.T) { testAnEmptiedPageStillLeadsBack(t, newTestDeps(t)) }

func testAnEmptiedPageStillLeadsBack(t *testing.T, deps Deps) {
	svc := NewService()
	clock := time.Date(2031, 7, 5, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	accept := func(title string) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			OwnerUserID: uintPtr(7), Replay: ReplayInput{NonReplayable: true},
		})
	}
	oldest := accept("1")
	two, three := accept("2"), accept("3")
	viewer := Access{UserID: 7}
	undismissed := Filter{Dismissed: boolPtr(false)}

	first := listFor(t, svc, deps, viewer, undismissed, Cursor{}, 2)
	requireIDs(t, "first page", pageIDs(first), three.ID, two.ID)
	if err := svc.SetPreference(deps, viewer, PreferenceRequest{JobID: oldest.ID, Dismissed: boolPtr(true)}); err != nil {
		t.Fatalf("dismiss: %v", err)
	}

	emptied := listFor(t, svc, deps, viewer, undismissed, *first.Next, 2)
	requireIDs(t, "emptied page", pageIDs(emptied))
	if emptied.Prev == nil {
		t.Fatal("an emptied page offered no way back to the rows before it")
	}
	back := listBeforeFor(t, svc, deps, viewer, undismissed, *emptied.Prev, 2)
	requireIDs(t, "back", pageIDs(back), three.ID, two.ID)
}
