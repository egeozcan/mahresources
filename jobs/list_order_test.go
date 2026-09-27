package jobs

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// stateEnteredFixture accepts a long Job and then four quick ones, and finishes
// the long one last: by acceptance it is the oldest of the five, by the time it
// entered its state the newest.
func stateEnteredFixture(t *testing.T) (*Service, Deps, Snapshot, []Snapshot) {
	t.Helper()
	return stateEnteredFixtureOn(t, newTestDeps(t))
}

func stateEnteredFixtureOn(t *testing.T, deps Deps) (*Service, Deps, Snapshot, []Snapshot) {
	t.Helper()
	svc := NewService()
	clock := time.Date(2031, 7, 1, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	accept := func(title string) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: uintPtr(7), Title: title, Replay: ReplayInput{NonReplayable: true},
		})
	}
	finish := func(job Snapshot, to State) Snapshot {
		clock = clock.Add(time.Minute)
		job = advanceReplayJob(t, svc, deps, job, StateRunning)
		clock = clock.Add(time.Minute)
		return advanceReplayJob(t, svc, deps, job, to)
	}

	long := accept("long")
	long = advanceReplayJob(t, svc, deps, long, StateRunning)
	quick := make([]Snapshot, 0, 4)
	for _, title := range []string{"quick-1", "quick-2", "quick-3", "quick-4"} {
		quick = append(quick, finish(accept(title), StateSucceeded))
	}
	clock = clock.Add(time.Minute)
	long = advanceReplayJob(t, svc, deps, long, StateSucceeded)
	return svc, deps, long, quick
}

// TestStateEnteredOrderLeadsWithTheJobThatJustFinished pins the order the Jobs
// panel reads its groups in: a long Job that finishes after newer ones is the
// first finished Job, where acceptance order leaves it off the page.
func TestStateEnteredOrderLeadsWithTheJobThatJustFinished(t *testing.T) {
	requireStateEnteredOrder(t, stateEnteredFixture)
}

func requireStateEnteredOrder(t *testing.T, fixture func(*testing.T) (*Service, Deps, Snapshot, []Snapshot)) {
	t.Helper()
	svc, deps, long, quick := fixture(t)
	// Two states, so the listing reads one branch per state.
	finished := Filter{States: []string{string(StateSucceeded), string(StateCancelled)}}
	for _, access := range []Access{{UserID: 1, Administrator: true}, {UserID: 7}} {
		byAcceptance := listFor(t, svc, deps, access, finished, Cursor{}, 3)
		requireIDs(t, "acceptance order", pageIDs(byAcceptance), quick[3].ID, quick[2].ID, quick[1].ID)

		byState := listFor(t, svc, deps, access, finished, Cursor{Order: OrderStateEntered}, 3)
		requireIDs(t, "state-entered order", pageIDs(byState), long.ID, quick[3].ID, quick[2].ID)
		if byState.Jobs[0].StateEnteredAt == nil || !byState.Jobs[0].StateEnteredAt.After(*byState.Jobs[1].StateEnteredAt) {
			t.Fatalf("the page does not carry the instants it is ordered by: %+v", byState.Jobs[:2])
		}
		if byState.Next == nil || byState.Next.Order != OrderStateEntered || byState.Prev != nil {
			t.Fatalf("a state-entered page continues with %+v and goes back with %+v", byState.Next, byState.Prev)
		}

		// The next page continues the same order, and together they hold every
		// Job once.
		rest := listFor(t, svc, deps, access, finished, *byState.Next, 3)
		requireIDs(t, "the next page", pageIDs(rest), quick[1].ID, quick[0].ID)
		if rest.Next != nil {
			t.Fatalf("the last page offers a next one: %+v", rest.Next)
		}
	}
}

func TestStateEnteredOrderRefusesWhatItCannotRead(t *testing.T) {
	svc, deps, long, _ := stateEnteredFixture(t)
	admin := Access{UserID: 1, Administrator: true}
	for name, cursor := range map[string]Cursor{
		"an unknown order":           {Order: "finishedAt"},
		"a position with no instant": {Order: OrderStateEntered, ID: long.ID},
		"an instant with no job":     {Order: OrderStateEntered, StateEnteredAt: time.Now()},
	} {
		if _, err := svc.List(deps, admin, Filter{}, cursor, 3); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("%s: List = %v, want ErrInvalidCursor", name, err)
		}
	}
	if _, err := svc.ListBefore(deps, admin, Filter{}, Cursor{Order: OrderStateEntered, StateEnteredAt: time.Now(), ID: long.ID}, 3); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("ListBefore in state-entered order = %v, want ErrInvalidCursor", err)
	}
}

// TestStateEnteredOrderSeeksEachState pins the plan the Jobs panel's groups
// are read with on every refresh: each state of the group is its own seek on an
// index ordered by state_entered_at, never a scan of the table and never a sort
// of every Job in the state.
func TestStateEnteredOrderSeeksEachState(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	dismissed := false
	filter := Filter{States: []string{string(StateSucceeded), string(StateCancelled)}, Dismissed: &dismissed}
	for _, tc := range []struct {
		name   string
		access Access
		index  string
	}{
		{"administrator", Access{UserID: 1, Administrator: true}, "idx_jobs_state_entered"},
		{"owner", Access{UserID: 7}, "idx_jobs_visible_state_entered"},
	} {
		sql, vars := listRowsStatement(t, svc, deps, tc.access, filter, true, Cursor{Order: OrderStateEntered})
		plan := explainSQLite(t, deps, sql, vars)
		if strings.Count(plan, "USING INDEX "+tc.index+" (") != 2 || strings.Contains(plan, "SCAN jobs") {
			t.Errorf("the %s's state-entered page does not seek %s once per state:\n%s", tc.name, tc.index, plan)
		}
		// No state filter is every state, one seek each, rather than a sort of
		// the whole table.
		sql, vars = listRowsStatement(t, svc, deps, tc.access, Filter{}, true, Cursor{Order: OrderStateEntered})
		plan = explainSQLite(t, deps, sql, vars)
		if strings.Count(plan, "USING INDEX "+tc.index+" (") != len(AllStates) || strings.Contains(plan, "SCAN jobs") {
			t.Errorf("the %s's unfiltered state-entered page does not seek %s once per state:\n%s", tc.name, tc.index, plan)
		}
	}
}

// TestStateEnteredOrderWithoutAStateListsEveryJobOnce pages an unfiltered
// listing in state-entered order: every state is its own branch, and the pages
// hold every Job once, newest state change first.
func TestStateEnteredOrderWithoutAStateListsEveryJobOnce(t *testing.T) {
	requireEveryJobOnceWithoutAState(t, stateEnteredFixture)
}

func requireEveryJobOnceWithoutAState(t *testing.T, fixture func(*testing.T) (*Service, Deps, Snapshot, []Snapshot)) {
	t.Helper()
	svc, deps, long, quick := fixture(t)
	admin := Access{UserID: 1, Administrator: true}
	var seen []string
	cursor := Cursor{Order: OrderStateEntered}
	for page := 0; page < 5; page++ {
		listed := listFor(t, svc, deps, admin, Filter{}, cursor, 2)
		seen = append(seen, pageIDs(listed)...)
		if listed.Next == nil {
			break
		}
		cursor = *listed.Next
	}
	requireIDs(t, "every Job", seen, long.ID, quick[3].ID, quick[2].ID, quick[1].ID, quick[0].ID)
}
