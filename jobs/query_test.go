package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
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

// TestPartialStateFilterSelectsSucceededJobsLeftUnfinished proves the filter's
// "partial" token is a subset of succeeded — the Jobs a Kind recorded as stopped
// short — and that it joins the other states as one more alternative rather
// than narrowing them. A running Job carrying the same phase spelling is not
// partial: the phase only means "unfinished" once the Job has succeeded.
func TestPartialStateFilterSelectsSucceededJobsLeftUnfinished(t *testing.T) {
	testPartialStateFilterSelectsSucceededJobsLeftUnfinished(t, newTestDeps(t))
}

func testPartialStateFilterSelectsSucceededJobsLeftUnfinished(t *testing.T, deps Deps) {
	svc := NewService()
	clock := time.Date(2031, 6, 3, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	finish := func(title string, to State, phase string) Snapshot {
		clock = clock.Add(time.Minute)
		snap := acceptFor(t, svc, deps, Acceptance{
			Kind: "plugin-action", KindVersion: 1, State: StateQueued, Origin: "ui", Title: title,
			Replay: ReplayInput{NonReplayable: true},
		})
		snap = advanceReplayJob(t, svc, deps, snap, StateRunning)
		if to == StateRunning {
			if phase == "" {
				return snap
			}
			next, err := svc.UpdateProgress(deps, ExecutionRef{JobID: snap.ID, ExecutionToken: executionTokenOf(t, deps, snap.ID)}, Progress{Phase: phase})
			if err != nil {
				t.Fatalf("UpdateProgress: %v", err)
			}
			return next
		}
		transition := Transition{
			JobID: snap.ID, ExpectedVersion: snap.Version, To: to, Phase: phase,
			ExecutionToken: executionTokenOf(t, deps, snap.ID),
		}
		if to == StateFailed {
			transition.Failure = &Failure{Code: "test-failure", Class: FailureClassInternal, Message: "gave up"}
		}
		next, err := svc.Transition(deps, transition)
		if err != nil {
			t.Fatalf("Transition -> %s: %v", to, err)
		}
		return next
	}
	complete := finish("complete", StateSucceeded, "")
	partial := finish("partial", StateSucceeded, PhasePartial)
	failed := finish("failed", StateFailed, "")
	runningPartial := finish("running with the phase", StateRunning, PhasePartial)
	if runningPartial.Phase != PhasePartial {
		t.Fatalf("setup: running job phase = %q, want %q", runningPartial.Phase, PhasePartial)
	}

	admin := Access{UserID: 1, Administrator: true}
	requireIDs(t, "partial", pageIDs(listFor(t, svc, deps, admin, Filter{States: []string{FilterStatePartial}}, Cursor{}, 0)),
		partial.ID)
	requireIDs(t, "succeeded still includes partial",
		pageIDs(listFor(t, svc, deps, admin, Filter{States: []string{string(StateSucceeded)}}, Cursor{}, 0)),
		partial.ID, complete.ID)
	requireIDs(t, "partial or failed",
		pageIDs(listFor(t, svc, deps, admin, Filter{States: []string{string(StateFailed), FilterStatePartial}}, Cursor{}, 0)),
		failed.ID, partial.ID)
	requireIDs(t, "succeeded and partial is succeeded",
		pageIDs(listFor(t, svc, deps, admin, Filter{States: []string{string(StateSucceeded), FilterStatePartial}}, Cursor{}, 0)),
		partial.ID, complete.ID)
	requireIDs(t, "partial with another dimension",
		pageIDs(listFor(t, svc, deps, admin, Filter{States: []string{FilterStatePartial}, Search: "complete"}, Cursor{}, 0)))

	counts, err := svc.CountByState(deps, admin, Filter{States: []string{FilterStatePartial}})
	if err != nil {
		t.Fatalf("CountByState: %v", err)
	}
	if counts[string(StateSucceeded)] != 1 || len(counts) != 1 {
		t.Fatalf("CountByState(partial) = %v, want succeeded: 1", counts)
	}
}

// TestListFiltersByLineageRelationship pins the relation filter to the spelling
// every other lineage read uses: the FROM endpoint of a link, which is the
// successor of a Retry or Repeat and the parent of a child stage.
func TestListFiltersByLineageRelationship(t *testing.T) {
	testListFiltersByLineageRelationship(t, newTestDeps(t))
}

func testListFiltersByLineageRelationship(t *testing.T, deps Deps) {
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

// TestPartialStateFilterUsesAnIndex pins the plan: partial Jobs are a sliver of
// the succeeded ones, and a deployment holds millions, so the list page and the
// count must find them through an index that carries the phase rather than by
// reading every succeeded row to test it.
func TestPartialStateFilterUsesAnIndex(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	admin := Access{UserID: 1, Administrator: true}
	owner := Access{UserID: 7}
	filter := Filter{States: []string{FilterStatePartial}}

	for _, tc := range []struct {
		name   string
		access Access
		index  string
	}{
		{"administrator", admin, "idx_jobs_state_phase"},
		// An owner's reads carry the visibility predicate, which leads its own
		// index; the phase has to sit in that one too.
		{"owner", owner, "idx_jobs_visible_phase"},
	} {
		for what, build := range pagePlanQueries() {
			plan := explainListQuery(t, svc, deps, tc.access, filter, build)
			if !strings.Contains(plan, tc.index) || strings.Contains(plan, "TEMP B-TREE") {
				t.Errorf("the %s's partial %s does not read %s in order:\n%s", tc.name, what, tc.index, plan)
			}
		}
	}
}

// TestRelationshipFiltersCheckEachLinkRatherThanListingTheVisibleJobs pins the
// plan of all three lineage filters. Their far endpoint must be visible, and
// answering that with "IN (every Job the asker may see)" materializes that whole
// set — every Job, for an administrator — before a 50-row page. Each link is
// checked against its own far Job instead.
func TestRelationshipFiltersCheckEachLinkRatherThanListingTheVisibleJobs(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	for _, access := range []Access{{UserID: 1, Administrator: true}, {UserID: 7}} {
		for _, tc := range []struct {
			filter Filter
			lookup string // the link index search that bounds each Job to its own links
		}{
			{Filter{Relationship: string(LinkRetryOf)}, "(type=? AND from_job_id=?)"},
			{Filter{InboundRelationship: string(LinkRetryOf)}, "idx_job_links_inbound (to_job_id=? AND type=?)"},
			{Filter{NoInboundRelationship: string(LinkRetryOf)}, "idx_job_links_inbound (to_job_id=? AND type=?)"},
		} {
			for what, build := range pagePlanQueries() {
				plan := explainListQuery(t, svc, deps, access, tc.filter, build)
				// The outer list may scan jobs (that is the listing); the link
				// and its far Job must each be a keyed search.
				if strings.Contains(plan, "LIST SUBQUERY") || strings.Contains(plan, "SCAN l") ||
					strings.Contains(plan, "SCAN far") || !strings.Contains(plan, tc.lookup) {
					t.Errorf("%+v %s for %+v does not look each link up by its own Job:\n%s", tc.filter, what, access, plan)
				}
			}
		}
	}
}

// TestMixedPartialFilterPagesAcrossBothBranches is "failed or partially
// completed" walked with a small page, forward and back. The list reads the two
// branches separately — each through its own index — and merges them by the
// keyset, so the pages must be exactly the newest-first order of the union, with
// Next and Prev that continue it, as the single-query listing's are.
func TestMixedPartialFilterPagesAcrossBothBranches(t *testing.T) {
	testMixedPartialFilterPagesAcrossBothBranches(t, newTestDeps(t))
}

func testMixedPartialFilterPagesAcrossBothBranches(t *testing.T, deps Deps) {
	svc := NewService()
	clock := time.Date(2031, 7, 1, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	var want []string // newest first
	for i := 0; i < 9; i++ {
		clock = clock.Add(time.Minute)
		snap := acceptFor(t, svc, deps, Acceptance{
			Kind: "plugin-action", KindVersion: 1, State: StateQueued, Origin: "ui", Title: fmt.Sprintf("job %d", i),
			OwnerUserID: uintPtr(7), Replay: ReplayInput{NonReplayable: true},
		})
		snap = advanceReplayJob(t, svc, deps, snap, StateRunning)
		transition := Transition{JobID: snap.ID, ExpectedVersion: snap.Version, ExecutionToken: executionTokenOf(t, deps, snap.ID)}
		switch i % 3 {
		case 0: // failed: in the filter
			transition.To = StateFailed
			transition.Failure = &Failure{Code: "x", Class: FailureClassInternal, Message: "x"}
		case 1: // partial: in the filter
			transition.To, transition.Phase = StateSucceeded, PhasePartial
		default: // complete: not in the filter
			transition.To = StateSucceeded
		}
		if _, err := svc.Transition(deps, transition); err != nil {
			t.Fatalf("finish %d: %v", i, err)
		}
		if i%3 != 2 {
			want = append([]string{snap.ID}, want...)
		}
	}

	filter := Filter{States: []string{string(StateFailed), FilterStatePartial}}
	for _, access := range []Access{{UserID: 1, Administrator: true}, {UserID: 7}} {
		var got []string
		var pages []Page
		cursor := Cursor{}
		for {
			page := listFor(t, svc, deps, access, filter, cursor, 2)
			pages = append(pages, page)
			got = append(got, pageIDs(page)...)
			if page.Next == nil {
				break
			}
			cursor = *page.Next
		}
		if !slices.Equal(got, want) {
			t.Fatalf("admin=%v walked %v, want %v", access.Administrator, got, want)
		}
		// Walking back from the last page lands on each earlier page in turn.
		for i := len(pages) - 1; i > 0; i-- {
			back, err := svc.ListBefore(deps, access, filter, *pages[i].Prev, 2)
			if err != nil {
				t.Fatalf("ListBefore: %v", err)
			}
			if !slices.Equal(pageIDs(back), pageIDs(pages[i-1])) {
				t.Fatalf("admin=%v back from page %d = %v, want %v", access.Administrator, i, pageIDs(back), pageIDs(pages[i-1]))
			}
		}
		counts, err := svc.CountByState(deps, access, filter)
		if err != nil {
			t.Fatalf("CountByState: %v", err)
		}
		if counts[string(StateFailed)] != 3 || counts[string(StateSucceeded)] != 3 || len(counts) != 2 {
			t.Fatalf("admin=%v counts = %v, want failed 3, succeeded 3", access.Administrator, counts)
		}
	}
}

// TestListBranchesSplitsOnlyAMixedPartialFilter pins when the listing reads two
// branches: the partial token beside other states that do not include it. Every
// other filter is read as itself, and the split keeps every other dimension.
func TestListBranchesSplitsOnlyAMixedPartialFilter(t *testing.T) {
	for name, tc := range map[string]struct {
		states []string
		want   [][]string
	}{
		"no partial":          {[]string{"failed", "blocked"}, [][]string{{"failed", "blocked"}}},
		"partial alone":       {[]string{FilterStatePartial}, [][]string{{FilterStatePartial}}},
		"partial + succeeded": {[]string{"succeeded", FilterStatePartial}, [][]string{{"succeeded", FilterStatePartial}}},
		"partial + failed":    {[]string{"failed", FilterStatePartial, "blocked"}, [][]string{{"failed", "blocked"}, {FilterStatePartial}}},
	} {
		branches := listBranches(Filter{States: tc.states, Kinds: []string{"k"}, Search: "s"})
		if len(branches) != len(tc.want) {
			t.Fatalf("%s: %d branches, want %d", name, len(branches), len(tc.want))
		}
		for i, branch := range branches {
			if !slices.Equal(branch.States, tc.want[i]) || !slices.Equal(branch.Kinds, []string{"k"}) || branch.Search != "s" {
				t.Errorf("%s: branch %d = %+v, want states %v with the other dimensions kept", name, i, branch, tc.want[i])
			}
		}
	}
}

// TestMixedPartialPredicateBoundsEachBranchByTheWholeFilter is the summary's
// window reaching both branches. Each branch is the whole filter, not its state
// alone, so a narrow recent window bounds the seek into a long history of
// failures instead of materializing all of them for an outer window to discard.
func TestMixedPartialPredicateBoundsEachBranchByTheWholeFilter(t *testing.T) {
	deps := newTestDeps(t)
	from := time.Date(2031, 6, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	filter := Filter{
		States: []string{string(StateFailed), FilterStatePartial}, Kinds: []string{"remote-download"},
		AcceptedAfter: &from, AcceptedBefore: &to,
	}
	for _, access := range []Access{{UserID: 1, Administrator: true}, {UserID: 7}} {
		query, err := applyFilter(jobQuery(deps.DB.Model(&models.Job{}), access), access, filter)
		if err != nil {
			t.Fatalf("applyFilter: %v", err)
		}
		statement := query.Session(&gorm.Session{DryRun: true}).Find(&[]models.Job{}).Statement
		sql := statement.SQL.String()
		arms := strings.Split(sql, "UNION ALL")
		if len(arms) != 2 {
			t.Fatalf("admin=%v: want two branches, got SQL %s", access.Administrator, sql)
		}
		for i, arm := range arms {
			for _, bound := range []string{"jobs.accepted_at >=", "jobs.accepted_at <=", "jobs.kind IN"} {
				if !strings.Contains(arm, bound) {
					t.Errorf("admin=%v branch %d lacks %q:\n%s", access.Administrator, i, bound, arm)
				}
			}
		}
	}
}

// TestMixedPartialPredicateSeeksBothBranchesOnAPopulatedTable pins the single
// predicate a mixed partial filter becomes wherever it is read as one (Summary,
// and any reader other than the listing, which reads the branches directly):
// each branch is its own indexed seek — the partial one on the phase — and the
// outer read fetches the matched Jobs by id rather than walking every Job, or
// every succeeded one, to test them. The table is populated and analyzed,
// because SQLite plans an empty table from guesses no deployment shares.
func TestMixedPartialPredicateSeeksBothBranchesOnAPopulatedTable(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	owner := uint(7)
	base := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := make([]models.Job, 0, 20000)
	for i := 0; i < 20000; i++ {
		state, phase := string(StateSucceeded), ""
		switch {
		case i%4000 == 1:
			state = string(StateFailed)
		case i%10000 == 7:
			phase = PhasePartial
		}
		rows = append(rows, models.Job{
			ID: fmt.Sprintf("00000000-0000-7000-8000-%012d", i), Kind: "remote-download", KindVersion: 1,
			State: state, Phase: phase, VisibilityClass: string(VisibilityOwner), OwnerUserID: &owner,
			Origin: "api", Title: "job", AcceptedAt: base.Add(time.Duration(i) * time.Second),
			ReplayClass: string(ReplayClassNonReplayable),
		})
	}
	if err := deps.DB.Session(&gorm.Session{SkipHooks: true}).CreateInBatches(rows, 500).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := deps.DB.Exec("ANALYZE").Error; err != nil {
		t.Fatalf("analyze: %v", err)
	}

	filter := Filter{States: []string{string(StateFailed), FilterStatePartial}}
	for _, access := range []Access{{UserID: 1, Administrator: true}, {UserID: owner}} {
		for what, build := range pagePlanQueries() {
			plan := explainListQuery(t, svc, deps, access, filter, build)
			if !strings.Contains(plan, "state=? AND phase=?)") || !strings.Contains(plan, "(id=?)") ||
				strings.Contains(plan, "SCAN jobs") {
				t.Errorf("admin=%v %s: the mixed predicate does not seek both branches and fetch by id:\n%s", access.Administrator, what, plan)
			}
		}

		// The statements List and ListBefore actually run: two ordered, limited
		// branches, the partial one seeking on the phase, and the other reading
		// no more than the same listing without the partial token does (its own
		// plan is the plain state filter's, whatever that is).
		middle := Cursor{AcceptedAt: base.Add(10000 * time.Second), ID: fmt.Sprintf("00000000-0000-7000-8000-%012d", 10000)}
		for _, direction := range []struct {
			name   string
			desc   bool
			cursor Cursor
		}{{"List", true, Cursor{}}, {"List after a cursor", true, middle}, {"ListBefore", false, middle}} {
			sql, vars := listRowsStatement(t, svc, deps, access, filter, direction.desc, direction.cursor)
			plan := explainSQLite(t, deps, sql, vars)
			plainSQL, plainVars := listRowsStatement(t, svc, deps, access, Filter{States: []string{string(StateFailed)}}, direction.desc, direction.cursor)
			plain := explainSQLite(t, deps, plainSQL, plainVars)
			if !strings.Contains(sql, "UNION ALL") || !strings.Contains(plan, "state=? AND phase=?") {
				t.Errorf("admin=%v %s: the listing statement does not seek the partial branch:\n%s\n%s", access.Administrator, direction.name, sql, plan)
			}
			for _, line := range strings.Split(plan, "\n") {
				if strings.HasPrefix(line, "SCAN jobs") && !strings.Contains(plain, line) {
					t.Errorf("admin=%v %s: the mixed listing scans where the plain one does not (%q):\n%s\nplain:\n%s", access.Administrator, direction.name, line, plan, plain)
				}
			}
		}
	}
}

// explainSQLite is SQLite's plan for one statement.
func explainSQLite(t *testing.T, deps Deps, sql string, vars []any) string {
	t.Helper()
	rows, err := deps.DB.Raw("EXPLAIN QUERY PLAN "+sql, vars...).Rows()
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("read plan: %v", err)
		}
		plan.WriteString(detail + "\n")
	}
	return plan.String()
}

// listRowsStatement is the SQL and arguments List (desc) or ListBefore (asc)
// runs for one filter from one cursor — the statement itself, not a stand-in.
func listRowsStatement(t *testing.T, svc *Service, deps Deps, access Access, filter Filter, desc bool, cursor Cursor) (string, []any) {
	t.Helper()
	position := func(q *gorm.DB) *gorm.DB { return continueAfter(q, cursor) }
	if !desc {
		position = func(q *gorm.DB) *gorm.DB { return continueBefore(q, cursor) }
	}
	query, _, union, err := svc.listRowsQuery(deps, access, filter, 0, false, desc, position)
	if err != nil {
		t.Fatalf("listRowsQuery: %v", err)
	}
	dry := query.Session(&gorm.Session{DryRun: true})
	var statement *gorm.Statement
	if union {
		statement = dry.Scan(&[]models.Job{}).Statement
	} else {
		statement = dry.Find(&[]models.Job{}).Statement
	}
	return statement.SQL.String(), statement.Vars
}

var planIndexName = regexp.MustCompile(`USING (COVERING )?INDEX \S+`)

// planShape is a plan with its index names removed.
func planShape(plan string) string {
	return planIndexName.ReplaceAllString(plan, "USING INDEX")
}

// TestJobFilterIndexesAreIdempotent is the startup step run a second time, as
// every restart does: nothing to create, nothing refused.
func TestJobFilterIndexesAreIdempotent(t *testing.T) {
	deps := newTestDeps(t)
	if err := models.EnsureJobFilterIndexes(deps.DB); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	for _, name := range []string{"idx_jobs_state_phase", "idx_jobs_visible_phase", "idx_job_links_inbound"} {
		var count int64
		if err := deps.DB.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?", name).Scan(&count).Error; err != nil || count != 1 {
			t.Errorf("%s: count %d, %v", name, count, err)
		}
	}

	// An index under the name but on other columns is replaced, not accepted.
	if err := deps.DB.Exec("DROP INDEX idx_job_links_inbound").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := deps.DB.Exec("CREATE INDEX idx_job_links_inbound ON job_links (type)").Error; err != nil {
		t.Fatalf("create the wrong shape: %v", err)
	}
	if err := models.EnsureJobFilterIndexes(deps.DB); err != nil {
		t.Fatalf("pass over a wrong shape: %v", err)
	}
	var columns []string
	if err := deps.DB.Raw("SELECT name FROM pragma_index_info('idx_job_links_inbound') ORDER BY seqno").Scan(&columns).Error; err != nil {
		t.Fatalf("index info: %v", err)
	}
	if !slices.Equal(columns, []string{"to_job_id", "type", "from_job_id"}) {
		t.Fatalf("idx_job_links_inbound columns after the pass = %v", columns)
	}

	// Or on the right columns of another table, such as a retained backup copy.
	if err := deps.DB.Exec("DROP INDEX idx_jobs_state_phase").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := deps.DB.Exec("CREATE TABLE jobs_backup AS SELECT * FROM jobs").Error; err != nil {
		t.Fatalf("backup table: %v", err)
	}
	if err := deps.DB.Exec("CREATE INDEX idx_jobs_state_phase ON jobs_backup (state, phase, accepted_at, id)").Error; err != nil {
		t.Fatalf("index the backup: %v", err)
	}
	if err := models.EnsureJobFilterIndexes(deps.DB); err != nil {
		t.Fatalf("pass over an index on another table: %v", err)
	}
	var table string
	if err := deps.DB.Raw("SELECT tbl_name FROM sqlite_master WHERE type = 'index' AND name = 'idx_jobs_state_phase'").Scan(&table).Error; err != nil || table != "jobs" {
		t.Fatalf("idx_jobs_state_phase is on %q (%v), want jobs", table, err)
	}
}

func pagePlanQueries() map[string]func(*gorm.DB) *gorm.DB {
	return map[string]func(*gorm.DB) *gorm.DB{
		"page": func(q *gorm.DB) *gorm.DB {
			return q.Order("jobs.accepted_at DESC, jobs.id DESC").Limit(51).Find(&[]models.Job{})
		},
		"count": func(q *gorm.DB) *gorm.DB {
			var n int64
			return q.Count(&n)
		},
	}
}

// explainListQuery is SQLite's plan for one listing question.
func explainListQuery(t *testing.T, svc *Service, deps Deps, access Access, filter Filter, build func(*gorm.DB) *gorm.DB) string {
	t.Helper()
	query, _, err := svc.listQuery(deps, access, filter, Cursor{}, 0)
	if err != nil {
		t.Fatalf("listQuery: %v", err)
	}
	statement := build(query.Session(&gorm.Session{DryRun: true})).Statement
	rows, err := deps.DB.Raw("EXPLAIN QUERY PLAN "+statement.SQL.String(), statement.Vars...).Rows()
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("read plan: %v", err)
		}
		plan.WriteString(detail + "\n")
	}
	return plan.String()
}

// TestListFiltersByInboundRelationship is the other end of a link: the Job a
// Retry or Repeat was made from, or a child stage. With the negation it answers
// "failed and nobody has retried it yet". A successor the viewer cannot see
// does not count, in either direction: lineage drops hidden relatives, and a
// filter that disagreed would publish that the hidden Job exists.
func TestListFiltersByInboundRelationship(t *testing.T) {
	testListFiltersByInboundRelationship(t, newTestDeps(t))
}

func testListFiltersByInboundRelationship(t *testing.T, deps Deps) {
	svc := NewService()
	clock := time.Date(2031, 6, 4, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	accept := func(title string, class VisibilityClass) Snapshot {
		clock = clock.Add(time.Minute)
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			OwnerUserID: uintPtr(7), Visibility: class, Replay: ReplayInput{NonReplayable: true},
		})
	}
	link := func(kind LinkType, from, to Snapshot) {
		if err := svc.Link(deps, LinkRequest{Type: kind, FromJobID: from.ID, ToJobID: to.ID}); err != nil {
			t.Fatalf("%s link: %v", kind, err)
		}
	}
	retried := accept("retried", VisibilityOwner)
	retry := accept("the retry", VisibilityOwner)
	repeated := accept("repeated", VisibilityOwner)
	repeat := accept("the repeat", VisibilityOwner)
	hiddenlyRetried := accept("retried by a hidden job", VisibilityOwner)
	hiddenRetry := accept("a retry the owner may not read", VisibilityAdmin)
	untouched := accept("untouched", VisibilityOwner)
	link(LinkRetryOf, retry, retried)
	link(LinkRepeatOf, repeat, repeated)
	link(LinkRetryOf, hiddenRetry, hiddenlyRetried)
	link(LinkParentChild, retry, untouched)

	admin := Access{UserID: 1, Administrator: true}
	owner := Access{UserID: 7}
	requireIDs(t, "retried, admin", pageIDs(listFor(t, svc, deps, admin, Filter{InboundRelationship: string(LinkRetryOf)}, Cursor{}, 0)),
		hiddenlyRetried.ID, retried.ID)
	requireIDs(t, "retried, owner", pageIDs(listFor(t, svc, deps, owner, Filter{InboundRelationship: string(LinkRetryOf)}, Cursor{}, 0)),
		retried.ID)
	requireIDs(t, "repeated", pageIDs(listFor(t, svc, deps, owner, Filter{InboundRelationship: string(LinkRepeatOf)}, Cursor{}, 0)),
		repeated.ID)
	requireIDs(t, "child stage", pageIDs(listFor(t, svc, deps, owner, Filter{InboundRelationship: string(LinkParentChild)}, Cursor{}, 0)),
		untouched.ID)

	requireIDs(t, "not retried, owner", pageIDs(listFor(t, svc, deps, owner, Filter{NoInboundRelationship: string(LinkRetryOf)}, Cursor{}, 0)),
		untouched.ID, hiddenlyRetried.ID, repeat.ID, repeated.ID, retry.ID)
	requireIDs(t, "not retried, admin", pageIDs(listFor(t, svc, deps, admin, Filter{NoInboundRelationship: string(LinkRetryOf)}, Cursor{}, 0)),
		untouched.ID, hiddenRetry.ID, repeat.ID, repeated.ID, retry.ID)

	summary, err := svc.Summary(deps, owner, Filter{InboundRelationship: string(LinkRetryOf)}, 0)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Total != 1 {
		t.Fatalf("the owner's aggregate counted %d retried Jobs, want 1", summary.Total)
	}

	for _, filter := range []Filter{{InboundRelationship: "retried"}, {NoInboundRelationship: "retried"}} {
		if _, err := svc.List(deps, admin, filter, Cursor{}, 0); !errors.Is(err, ErrInvalidFilter) {
			t.Fatalf("an unknown inbound relationship %+v = %v, want ErrInvalidFilter", filter, err)
		}
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
		{name: "blank command key", filter: Filter{Command: " \t"}, want: ErrInvalidFilter},
		{name: "oversized command key", filter: Filter{Command: strings.Repeat("x", MaxCommandKeyBytes+1)}, want: ErrInvalidFilter},
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
			if tc.filter.Command != "" {
				if _, err := svc.Summary(deps, admin, tc.filter, 0); !errors.Is(err, ErrInvalidFilter) {
					t.Fatalf("Summary with invalid command key = %v, want ErrInvalidFilter", err)
				}
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

// TestListCommandFilterPaginatesSparseMatches verifies that Command is evaluated
// before the page boundary: a sparse key must not produce short pages, and the
// next cursor must name the last returned match rather than a candidate the scan
// skipped over.
func TestListCommandFilterPaginatesSparseMatches(t *testing.T) {
	testListCommandFilterPaginatesSparseMatches(t, newTestDeps(t))
}

func testListCommandFilterPaginatesSparseMatches(t *testing.T, deps Deps) {
	t.Helper()
	sqlDB, err := deps.DB.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.advertise = func(_ context.Context, commandContext CommandContext) ([]Command, error) {
		if strings.HasPrefix(commandContext.Snapshot.Title, "match-") {
			return []Command{{Key: "inspect"}}, nil
		}
		return nil, nil
	}
	adapter.selectCommand = func(_ context.Context, request CommandFilterRequest) (*gorm.DB, bool, error) {
		if request.Key != "inspect" {
			return nil, false, nil
		}
		return request.Jobs.Where("jobs.title LIKE ?", "match-%").Select("jobs.id"), true, nil
	}
	owner := uint(7)
	clock := time.Date(2034, 8, 1, 0, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	const candidates = 2*MaxPageSize + 3
	matches := make(map[int]bool)
	matches[0] = true
	matches[MaxPageSize] = true
	matches[2*MaxPageSize] = true
	created := make(map[int]Snapshot, len(matches))
	for i := 0; i < candidates; i++ {
		clock = clock.Add(time.Second)
		title := fmt.Sprintf("other-%03d", i)
		if matches[i] {
			title = fmt.Sprintf("match-%03d", i)
		}
		job := acceptFor(t, svc, deps, Acceptance{
			Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: &owner, Title: title, Replay: ReplayInput{NonReplayable: true},
		})
		if matches[i] {
			created[i] = job
		}
	}

	access := Access{UserID: owner}
	first := listFor(t, svc, deps, access, Filter{Command: "inspect"}, Cursor{}, 2)
	requireIDs(t, "first command page", pageIDs(first), created[2*MaxPageSize].ID, created[MaxPageSize].ID)
	if first.Next == nil || first.Next.ID != created[MaxPageSize].ID || !first.Next.AcceptedAt.Equal(created[MaxPageSize].AcceptedAt) {
		t.Fatalf("first page cursor = %+v, want cursor at the last returned match %+v", first.Next, created[MaxPageSize])
	}

	second := listFor(t, svc, deps, access, Filter{Command: "inspect"}, *first.Next, 2)
	requireIDs(t, "second command page", pageIDs(second), created[0].ID)
	if second.Next != nil {
		t.Fatalf("last sparse command page unexpectedly has next cursor %+v", second.Next)
	}

	summary, err := svc.Summary(deps, access, Filter{Command: "inspect"}, 0)
	if err != nil {
		t.Fatalf("sparse command summary: %v", err)
	}
	if summary.Total != 3 || summary.ByState[string(StateQueued)] != 3 || summary.ByKind[testKind] != 3 {
		t.Fatalf("sparse command summary = %+v, want the three matches across candidate batches", summary)
	}
}

// TestCommandFilterUsesCurrentRoleForListAndSummary shows command availability
// is asked from the current principal for each read, never cached on a Job or
// inherited from a prior administrator request.
func TestCommandFilterUsesCurrentRoleForListAndSummary(t *testing.T) {
	testCommandFilterUsesCurrentRoleForListAndSummary(t, newTestDeps(t))
}

func testCommandFilterUsesCurrentRoleForListAndSummary(t *testing.T, deps Deps) {
	t.Helper()
	sqlDB, err := deps.DB.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.advertise = func(_ context.Context, commandContext CommandContext) ([]Command, error) {
		if commandContext.Access.Administrator || commandContext.Snapshot.Title == "viewer-inspect" {
			return []Command{{Key: "inspect"}}, nil
		}
		return nil, nil
	}
	adapter.selectCommand = func(_ context.Context, request CommandFilterRequest) (*gorm.DB, bool, error) {
		if request.Key != "inspect" {
			return nil, false, nil
		}
		if request.Access.Administrator {
			return request.Jobs.Select("jobs.id"), true, nil
		}
		return request.Jobs.Where("jobs.title = ?", "viewer-inspect").Select("jobs.id"), true, nil
	}
	owner := uint(7)
	clock := time.Date(2034, 8, 2, 0, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	for i := 0; i < 3; i++ {
		clock = clock.Add(time.Minute)
		title := fmt.Sprintf("job-%d", i)
		if i == 0 {
			title = "viewer-inspect"
		}
		acceptFor(t, svc, deps, Acceptance{
			Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: &owner, Title: title, Replay: ReplayInput{NonReplayable: true},
		})
	}
	clock = clock.Add(time.Minute)
	otherOwner := uint(8)
	acceptFor(t, svc, deps, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: &otherOwner, Title: "viewer-inspect", Replay: ReplayInput{NonReplayable: true},
	})
	filter := Filter{Command: "inspect"}
	viewer := Access{UserID: owner}
	admin := Access{UserID: owner, Administrator: true}

	if got := listFor(t, svc, deps, viewer, filter, Cursor{}, 10); len(got.Jobs) != 1 {
		t.Fatalf("viewer list matched %d jobs before promotion, want only its advertised command", len(got.Jobs))
	}
	viewerSummary, err := svc.Summary(deps, viewer, filter, 0)
	if err != nil {
		t.Fatalf("viewer summary: %v", err)
	}
	if viewerSummary.Total != 1 || viewerSummary.ByKind[testKind] != 1 {
		t.Fatalf("viewer summary = %+v, want only its advertised command", viewerSummary)
	}

	adminPage := listFor(t, svc, deps, admin, filter, Cursor{}, 10)
	if len(adminPage.Jobs) != 4 {
		t.Fatalf("administrator list matched %d jobs after promotion, want 4 including the other owner's work", len(adminPage.Jobs))
	}
	adminSummary, err := svc.Summary(deps, admin, filter, 0)
	if err != nil {
		t.Fatalf("administrator summary: %v", err)
	}
	if adminSummary.Total != 4 || adminSummary.ByState[string(StateQueued)] != 4 || adminSummary.ByKind[testKind] != 4 {
		t.Fatalf("administrator command summary = %+v, want four queued %s jobs", adminSummary, testKind)
	}

	// Demotion is observed by the next read, including when the same user id is
	// retained by the session.
	if got := listFor(t, svc, deps, viewer, filter, Cursor{}, 10); len(got.Jobs) != 1 {
		t.Fatalf("demoted viewer list matched %d jobs, want only its advertised command", len(got.Jobs))
	}
	demotedSummary, err := svc.Summary(deps, viewer, filter, 0)
	if err != nil {
		t.Fatalf("demoted summary: %v", err)
	}
	if demotedSummary.Total != 1 {
		t.Fatalf("demoted summary total = %d, want 1", demotedSummary.Total)
	}
}

// TestCommandFilterQueryCountDoesNotScaleWithCandidates is a query-count
// regression for sparse selectors. A command predicate must stay a SQL
// subquery before pagination and aggregation; one match among thousands of
// visible candidates costs the same number of database round trips as one
// match among dozens.
func TestCommandFilterQueryCountDoesNotScaleWithCandidates(t *testing.T) {
	testCommandFilterQueryCountDoesNotScaleWithCandidates(t, newTestDeps(t))
}

func testCommandFilterQueryCountDoesNotScaleWithCandidates(t *testing.T, deps Deps) {
	t.Helper()
	svc := NewService()
	adapter := registerTestAdapter(t, svc, testDefinition())
	var commandCalls atomic.Int64
	var selectorCalls atomic.Int64
	adapter.advertise = func(_ context.Context, commandContext CommandContext) ([]Command, error) {
		commandCalls.Add(1)
		if commandContext.Snapshot.Title == "sparse-match" {
			return []Command{{Key: "inspect"}}, nil
		}
		return nil, nil
	}
	adapter.selectCommand = func(_ context.Context, request CommandFilterRequest) (*gorm.DB, bool, error) {
		selectorCalls.Add(1)
		if request.Key != "inspect" {
			return nil, false, nil
		}
		return request.Jobs.Where("jobs.title = ?", "sparse-match").Select("jobs.id"), true, nil
	}

	now := time.Date(2035, 2, 4, 12, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return now }
	seedCommandFilterRows(t, deps, 0, 64, true, now)
	queryCount := atomic.Int64{}
	const hookName = "test:command-filter-query-count"
	if err := deps.DB.Callback().Query().After("gorm:query").Register(hookName, func(tx *gorm.DB) {
		if tx.Error == nil {
			queryCount.Add(1)
		}
	}); err != nil {
		t.Fatalf("register query counter: %v", err)
	}
	t.Cleanup(func() { _ = deps.DB.Callback().Query().Remove(hookName) })

	run := func() int64 {
		t.Helper()
		queryCount.Store(0)
		page, err := svc.List(deps, Access{Administrator: true}, Filter{Command: "inspect"}, Cursor{}, 10)
		if err != nil {
			t.Fatalf("List sparse command filter: %v", err)
		}
		if len(page.Jobs) != 1 || page.Jobs[0].Title != "sparse-match" {
			t.Fatalf("sparse page = %+v, want the one advertised match", page.Jobs)
		}
		summary, err := svc.Summary(deps, Access{Administrator: true}, Filter{Command: "inspect"}, 0)
		if err != nil {
			t.Fatalf("Summary sparse command filter: %v", err)
		}
		if summary.Total != 1 {
			t.Fatalf("sparse summary total = %d, want 1", summary.Total)
		}
		return queryCount.Load()
	}

	smallQueryCount := run()
	if commandCalls.Load() != 0 || selectorCalls.Load() != 2 {
		t.Fatalf("small candidate set called Commands %d times and selectors %d times; want zero per-Job calls and one selector per read", commandCalls.Load(), selectorCalls.Load())
	}
	selectorCalls.Store(0)
	runUnknown := func() int64 {
		t.Helper()
		queryCount.Store(0)
		page, err := svc.List(deps, Access{Administrator: true}, Filter{Command: "not-advertised"}, Cursor{}, 10)
		if err != nil {
			t.Fatalf("List unknown command filter: %v", err)
		}
		if len(page.Jobs) != 0 {
			t.Fatalf("unknown command page returned %d jobs", len(page.Jobs))
		}
		summary, err := svc.Summary(deps, Access{Administrator: true}, Filter{Command: "not-advertised"}, 0)
		if err != nil {
			t.Fatalf("Summary unknown command filter: %v", err)
		}
		if summary.Total != 0 {
			t.Fatalf("unknown command summary total = %d, want 0", summary.Total)
		}
		return queryCount.Load()
	}
	unknownSmallQueryCount := runUnknown()
	if commandCalls.Load() != 0 || selectorCalls.Load() != 2 {
		t.Fatalf("small unknown-key set called Commands %d times and selectors %d times; want zero per-Job calls and one selector per read", commandCalls.Load(), selectorCalls.Load())
	}
	selectorCalls.Store(0)
	seedCommandFilterRows(t, deps, 64, 16384, false, now)
	largeQueryCount := run()
	if largeQueryCount != smallQueryCount {
		t.Fatalf("query count grew with candidate rows: small=%d large=%d", smallQueryCount, largeQueryCount)
	}
	if commandCalls.Load() != 0 || selectorCalls.Load() != 2 {
		t.Fatalf("large candidate set called Commands %d times and selectors %d times; want zero per-Job calls and one selector per read", commandCalls.Load(), selectorCalls.Load())
	}
	selectorCalls.Store(0)
	unknownLargeQueryCount := runUnknown()
	if unknownLargeQueryCount != unknownSmallQueryCount {
		t.Fatalf("unknown-key query count grew with candidate rows: small=%d large=%d", unknownSmallQueryCount, unknownLargeQueryCount)
	}
	if commandCalls.Load() != 0 || selectorCalls.Load() != 2 {
		t.Fatalf("large unknown-key set called Commands %d times and selectors %d times; want zero per-Job calls and one selector per read", commandCalls.Load(), selectorCalls.Load())
	}

	commands, err := svc.AdvertisedCommands(context.Background(), deps, Access{Administrator: true}, "00000000-0000-7000-8000-000000000000")
	if err != nil {
		t.Fatalf("read selected Job's commands: %v", err)
	}
	if len(commands) != 1 || commands[0].Key != "inspect" {
		t.Fatalf("selected Job advertises %+v, want inspect", commands)
	}
}

func seedCommandFilterRows(t *testing.T, deps Deps, start, count int, includeMatch bool, acceptedAt time.Time) {
	t.Helper()
	owner := uint(7)
	rows := make([]models.Job, count)
	for i := range rows {
		index := start + i
		title := "unmatched"
		if includeMatch && i == 0 {
			title = "sparse-match"
		}
		rows[i] = models.Job{
			ID:   fmt.Sprintf("00000000-0000-7000-8000-%012x", index),
			Kind: testKind, KindVersion: 1, State: string(StateQueued), Title: title,
			OwnerUserID: &owner, Origin: "api", VisibilityClass: string(VisibilityOwner),
			ExecutionPrincipal: string(PrincipalOwner), ReplayClass: string(ReplayClassNonReplayable),
			Version: 1, AcceptedAt: acceptedAt.Add(-time.Duration(index) * time.Second),
		}
	}
	if len(rows) == 0 {
		return
	}
	if err := deps.DB.CreateInBatches(&rows, 500).Error; err != nil {
		t.Fatalf("seed %d command-filter rows: %v", count, err)
	}
}

// TestCommandHostSelectorsMatchAdvertisedCommands covers the host predicates
// composed after adapter selectors. The cases include state gates, cancel
// intent, an unresolved claim, retry lineage, replay availability and the
// caller's preference commands; each SQL-filtered set must equal the set the
// current principal sees from Commands.
func TestCommandHostSelectorsMatchAdvertisedCommands(t *testing.T) {
	testCommandHostSelectorsMatchAdvertisedCommands(t, newTestDeps(t))
}

func testCommandHostSelectorsMatchAdvertisedCommands(t *testing.T, deps Deps) {
	t.Helper()
	svc := NewService()
	registerTestCodec(t, svc)
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "command-selector-test-key")}
	clock := time.Date(2035, 3, 10, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	adapter := registerTestAdapter(t, svc, testDefinition())
	adapter.advertise = func(context.Context, CommandContext) ([]Command, error) {
		return []Command{
			{Key: CommandCancel}, {Key: CommandPause}, {Key: CommandResume},
			{Key: CommandRetry}, {Key: CommandRepeat},
		}, nil
	}
	adapter.selectCommand = func(_ context.Context, request CommandFilterRequest) (*gorm.DB, bool, error) {
		switch request.Key {
		case CommandCancel, CommandPause, CommandResume, CommandRetry, CommandRepeat:
			return request.Jobs.Select("jobs.id"), true, nil
		default:
			return nil, false, nil
		}
	}
	owner := uint(7)
	jobsByName := make(map[string]Snapshot)
	accept := func(name string, replay bool) Snapshot {
		t.Helper()
		clock = clock.Add(time.Second)
		input := ReplayInput{NonReplayable: true}
		if replay {
			input = ReplayInput{Input: json.RawMessage(`{"kind":"selector-test"}`)}
		}
		snapshot := acceptFor(t, svc, deps, Acceptance{
			Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: &owner, ActorUserID: &owner, Title: name, Replay: input,
		})
		jobsByName[name] = snapshot
		return snapshot
	}
	setState := func(name string, state State, controlIntent string) {
		t.Helper()
		updates := map[string]any{"state": string(state), "control_intent": controlIntent}
		if err := deps.DB.Model(&models.Job{}).Where("id = ?", jobsByName[name].ID).Updates(updates).Error; err != nil {
			t.Fatalf("set %s state: %v", name, err)
		}
	}

	for _, name := range []string{
		"queued", "running", "running-cancel", "paused", "paused-cancel", "paused-held",
		"blocked", "failed", "failed-linked", "cancelled", "interrupted", "succeeded",
		"queued-no-replay", "succeeded-no-replay",
	} {
		accept(name, name != "queued-no-replay" && name != "succeeded-no-replay")
	}
	setState("running", StateRunning, "")
	setState("running-cancel", StateRunning, ControlIntentCancel)
	setState("paused", StatePaused, "")
	setState("paused-cancel", StatePaused, ControlIntentCancel)
	setState("paused-held", StatePaused, "")
	setState("blocked", StateBlocked, "")
	setState("failed", StateFailed, "")
	setState("failed-linked", StateFailed, "")
	setState("cancelled", StateCancelled, "")
	setState("interrupted", StateInterrupted, "")
	setState("succeeded", StateSucceeded, "")
	setState("succeeded-no-replay", StateSucceeded, "")
	accept("retry-successor", true)

	now := clock
	held := models.JobClaim{
		JobID: jobsByName["paused-held"].ID, Kind: testKind, KindVersion: 1,
		Claimant: "selector-test", ExecutionToken: "selector-claim-token",
		State: models.JobClaimStateHeld, ClaimedAt: now, HeartbeatAt: now,
		LeaseExpiresAt: now.Add(time.Minute),
	}
	if err := deps.DB.Create(&held).Error; err != nil {
		t.Fatalf("create unresolved claim: %v", err)
	}
	link := models.JobLink{
		Type: string(LinkRetryOf), FromJobID: jobsByName["retry-successor"].ID,
		ToJobID: jobsByName["failed-linked"].ID, CreatedAt: clock,
	}
	if err := deps.DB.Create(&link).Error; err != nil {
		t.Fatalf("create retry successor link: %v", err)
	}

	access := Access{UserID: owner}
	for _, key := range []string{
		CommandCancel, CommandPause, CommandResume, CommandRetry, CommandRepeat,
		CommandDismiss, CommandForget, CommandPin, CommandUnpin, CommandPinLineage,
	} {
		want := make(map[string]bool)
		for _, snapshot := range jobsByName {
			commands, err := svc.AdvertisedCommands(context.Background(), deps, access, snapshot.ID)
			if err != nil {
				t.Fatalf("commands for %s: %v", snapshot.ID, err)
			}
			for _, command := range commands {
				if command.Key == key {
					want[snapshot.ID] = true
				}
			}
		}
		page, err := svc.List(deps, access, Filter{Command: key}, Cursor{}, MaxPageSize)
		if err != nil {
			t.Fatalf("list %s selector: %v", key, err)
		}
		got := make(map[string]bool)
		for _, snapshot := range page.Jobs {
			got[snapshot.ID] = true
		}
		if !equalStringSets(got, want) {
			t.Fatalf("%s selector IDs = %v, Commands IDs = %v", key, got, want)
		}
	}
}

func equalStringSets(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for item := range left {
		if !right[item] {
			return false
		}
	}
	return true
}

type adapterWithoutCommandSelector struct{ Adapter }

func TestCommandFilterCoverageRejectsMissingSelectorAndKind(t *testing.T) {
	t.Run("registered adapter without selector", func(t *testing.T) {
		deps := newTestDeps(t)
		svc := NewService()
		legacy := adapterWithoutCommandSelector{Adapter: newTestAdapter(testDefinition())}
		if err := svc.RegisterAdapter(legacy); err != nil {
			t.Fatalf("register legacy adapter: %v", err)
		}
		if err := svc.ValidateCommandFilterCoverage([]CommandFilterKind{{Kind: testKind, Version: 1}}); !errors.Is(err, ErrCommandFilterUnavailable) {
			t.Fatalf("coverage check = %v, want ErrCommandFilterUnavailable", err)
		}
		if _, err := svc.List(deps, Access{Administrator: true}, Filter{Command: "inspect"}, Cursor{}, 10); !errors.Is(err, ErrCommandFilterUnavailable) {
			t.Fatalf("List with an adapter lacking a selector = %v, want ErrCommandFilterUnavailable", err)
		}
	})

	t.Run("missing expected Kind", func(t *testing.T) {
		svc := NewService()
		registerTestAdapter(t, svc, testDefinition())
		expected := []CommandFilterKind{
			{Kind: testKind, Version: 1},
			{Kind: "plugin-command", Version: 1},
		}
		if err := svc.ValidateCommandFilterCoverage(expected); !errors.Is(err, ErrCommandFilterUnavailable) {
			t.Fatalf("coverage check without expected plugin-command adapter = %v, want ErrCommandFilterUnavailable", err)
		}
		if err := svc.ValidateCommandFilterCoverage(expected[:1]); err != nil {
			t.Fatalf("complete test inventory: %v", err)
		}
	})
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

func TestSummaryRangeUsesSharedVisibilityAndFiltersPastInteractiveCeiling(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	registerTestAdapter(t, svc, testDefinition())
	now := time.Date(2031, 6, 14, 9, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return now }
	visibleOwner := uint(7)
	hiddenOwner := uint(8)

	now = now.Add(-180 * 24 * time.Hour)
	acceptFor(t, svc, deps, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api", Title: "visible historical job",
		OwnerUserID: &visibleOwner, Replay: ReplayInput{NonReplayable: true},
	})
	now = now.Add(24 * time.Hour)
	_ = acceptFor(t, svc, deps, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api", Title: "hidden historical job",
		OwnerUserID: &hiddenOwner, Replay: ReplayInput{NonReplayable: true},
	})

	from := time.Date(2030, 12, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)
	viewer := Access{UserID: visibleOwner}
	summary, err := svc.SummaryRange(deps, viewer, Filter{Kinds: []string{testKind}}, from, to)
	if err != nil {
		t.Fatalf("SummaryRange: %v", err)
	}
	if summary.Total != 1 || summary.ByKind[testKind] != 1 {
		t.Fatalf("owner-visible summary = %+v, want only the owner's historical Job", summary)
	}
	if summary.Window != to.Sub(from) || !summary.From.Equal(from) || !summary.To.Equal(to) {
		t.Fatalf("range = %v..%v (%v), want %v..%v (%v)", summary.From, summary.To, summary.Window, from, to, to.Sub(from))
	}

	admin, err := svc.SummaryRange(deps, Access{Administrator: true}, Filter{Kinds: []string{testKind}}, from, to)
	if err != nil {
		t.Fatalf("SummaryRange as admin: %v", err)
	}
	if admin.Total != 2 {
		t.Fatalf("administrator summary total = %d, want 2", admin.Total)
	}
	filtered, err := svc.SummaryRange(deps, viewer, Filter{Kinds: []string{"other-kind"}}, from, to)
	if err != nil {
		t.Fatalf("filtered SummaryRange: %v", err)
	}
	if filtered.Total != 0 {
		t.Fatalf("filtered summary total = %d, want 0", filtered.Total)
	}
	if _, err := svc.SummaryRange(deps, viewer, Filter{}, to, from); !errors.Is(err, ErrInvalidWindow) {
		t.Fatalf("reversed range error = %v, want ErrInvalidWindow", err)
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
	testListFiltersByLineageRelationshipWithoutRevealingAHiddenRelative(t, newTestDeps(t))
}

func testListFiltersByLineageRelationshipWithoutRevealingAHiddenRelative(t *testing.T, deps Deps) {
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
