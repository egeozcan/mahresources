//go:build postgres && json1 && fts5

package jobs

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// The list filters whose SQL is not a plain column predicate, on PostgreSQL:
// the partial state (a residual test beside the state IN list) and the three
// lineage filters (a correlated subquery whose far Job carries the visibility
// rule as unqualified columns, which must resolve to that Job and not to the
// outer one). Each body is the SQLite test's own, so the two engines answer one
// specification.

func TestPartialStateFilterSelectsSucceededJobsLeftUnfinishedOnPostgres(t *testing.T) {
	testPartialStateFilterSelectsSucceededJobsLeftUnfinished(t, newPGDeps(t))
}

func TestListFiltersByLineageRelationshipOnPostgres(t *testing.T) {
	testListFiltersByLineageRelationship(t, newPGDeps(t))
}

func TestListFiltersByLineageRelationshipWithoutRevealingAHiddenRelativeOnPostgres(t *testing.T) {
	testListFiltersByLineageRelationshipWithoutRevealingAHiddenRelative(t, newPGDeps(t))
}

func TestMixedPartialFilterPagesAcrossBothBranchesOnPostgres(t *testing.T) {
	testMixedPartialFilterPagesAcrossBothBranches(t, newPGDeps(t))
}

func TestListFiltersByInboundRelationshipOnPostgres(t *testing.T) {
	testListFiltersByInboundRelationship(t, newPGDeps(t))
}

// partialPhaseSeek matches an index condition on the partial phase however
// PostgreSQL spells the column: "(phase = 'partial'::text)" or
// "((phase)::text = 'partial'::text)".
var partialPhaseSeek = regexp.MustCompile(`(Index Cond|Recheck Cond): .*\(?phase\)?(::text)? = 'partial'::text`)

// TestListFilterPlansOnAPopulatedPostgresTable checks the PostgreSQL plans on a
// table with statistics. PostgreSQL plans by cost, so an empty table proves
// nothing; this one holds a few thousand Jobs of one owner, most succeeded, a
// few partial, and a retry link for every other Job. The assertions are the
// shape the SQLite plan tests pin: the partial page is found through an index
// carrying the phase, and every lineage filter looks each listed Job's links up
// by that Job's id rather than reading the link table or the visible set.
func TestListFilterPlansOnAPopulatedPostgresTable(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	owner := uint(7)
	base := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)

	const total = 4000
	rows := make([]models.Job, 0, total)
	for i := 0; i < total; i++ {
		state, phase := string(StateSucceeded), ""
		switch {
		case i%400 == 0:
			phase = PhasePartial
		case i%5 == 0:
			state = string(StateFailed)
		}
		rows = append(rows, models.Job{
			ID: fmt.Sprintf("00000000-0000-7000-8000-%012d", i), Kind: "remote-download", KindVersion: 1,
			State: state, Phase: phase, VisibilityClass: string(VisibilityOwner), OwnerUserID: &owner,
			Origin: "api", Title: "job", AcceptedAt: base.Add(time.Duration(i) * time.Second),
			ReplayClass: string(ReplayClassNonReplayable),
		})
	}
	if err := deps.DB.Session(&gorm.Session{SkipHooks: true}).CreateInBatches(rows, 500).Error; err != nil {
		t.Fatalf("seed jobs: %v", err)
	}
	links := make([]models.JobLink, 0, total/2)
	for i := 1; i < total; i += 2 {
		links = append(links, models.JobLink{Type: string(LinkRetryOf), FromJobID: rows[i].ID, ToJobID: rows[i-1].ID})
	}
	if err := deps.DB.CreateInBatches(links, 500).Error; err != nil {
		t.Fatalf("seed links: %v", err)
	}
	if err := deps.DB.Exec("ANALYZE jobs; ANALYZE job_links").Error; err != nil {
		t.Fatalf("analyze: %v", err)
	}

	explain := func(access Access, filter Filter) string {
		query, _, err := svc.listQuery(deps, access, filter, Cursor{}, 0)
		if err != nil {
			t.Fatalf("listQuery: %v", err)
		}
		statement := query.Session(&gorm.Session{DryRun: true}).
			Order("jobs.accepted_at DESC, jobs.id DESC").Limit(51).Find(&[]models.Job{}).Statement
		var lines []string
		if err := deps.DB.Raw("EXPLAIN "+statement.SQL.String(), statement.Vars...).Scan(&lines).Error; err != nil {
			t.Fatalf("explain: %v", err)
		}
		return strings.Join(lines, "\n")
	}

	for _, access := range []Access{{UserID: 1, Administrator: true}, {UserID: owner}} {
		partial := explain(access, Filter{States: []string{FilterStatePartial}})
		if !strings.Contains(partial, "idx_jobs_state_phase") && !strings.Contains(partial, "idx_jobs_visible_phase") {
			t.Errorf("admin=%v: the partial page is not found through a phase index:\n%s", access.Administrator, partial)
		}
		// The partial token beside another state, read as one predicate (as
		// Summary reads it), with few matches: both branches seek, the partial one
		// on the phase, and the matched Jobs are fetched without reading the table.
		// (A dense match — a fifth of this table is failed — may rightly be
		// answered by a scan; the sparse case is the one a scan would ruin.)
		mixed := explain(access, Filter{States: []string{string(StateCancelled), FilterStatePartial}})
		if !partialPhaseSeek.MatchString(mixed) || strings.Contains(mixed, "Seq Scan on jobs") {
			t.Errorf("admin=%v: the sparse mixed partial predicate does not seek:\n%s", access.Administrator, mixed)
		}
		// The statements List and ListBefore run for a mixed filter — the two
		// ordered, limited branches in one UNION ALL — dense (a fifth of the table
		// failed) and sparse (no cancelled Job at all): the partial branch seeks
		// the phase and nothing reads the whole table.
		middle := Cursor{AcceptedAt: base.Add(2000 * time.Second), ID: rows[2000].ID}
		for _, states := range [][]string{{string(StateFailed), FilterStatePartial}, {string(StateCancelled), FilterStatePartial}} {
			for _, direction := range []struct {
				name   string
				desc   bool
				cursor Cursor
			}{{"List", true, Cursor{}}, {"List after a cursor", true, middle}, {"ListBefore", false, middle}} {
				sql, vars := listRowsStatement(t, svc, deps, access, Filter{States: states}, direction.desc, direction.cursor)
				var lines []string
				if err := deps.DB.Raw("EXPLAIN "+sql, vars...).Scan(&lines).Error; err != nil {
					t.Fatalf("explain: %v", err)
				}
				plan := strings.Join(lines, "\n")
				if !strings.Contains(sql, "UNION ALL") || !partialPhaseSeek.MatchString(plan) || strings.Contains(plan, "Seq Scan on jobs") {
					t.Errorf("admin=%v %v %s: the listing statement does not seek both branches:\n%s", access.Administrator, states, direction.name, plan)
				}
			}
		}
		// Every link here has one type, so an index that can only seek on type
		// would walk all of them per listed Job. The lookup must be keyed on the
		// outer Job's own id, through the column that end of the link names.
		for _, tc := range []struct {
			filter Filter
			key    string
		}{
			{Filter{Relationship: string(LinkRetryOf)}, "((from_job_id)::text = (jobs.id)::text)"},
			{Filter{InboundRelationship: string(LinkRetryOf)}, "((to_job_id)::text = (jobs.id)::text)"},
			{Filter{NoInboundRelationship: string(LinkRetryOf)}, "((to_job_id)::text = (jobs.id)::text)"},
		} {
			plan := explain(access, tc.filter)
			if strings.Contains(plan, "Seq Scan on job_links") || !strings.Contains(plan, "Index Cond: "+tc.key) {
				t.Errorf("admin=%v %+v does not look each link up by the listed Job's id:\n%s", access.Administrator, tc.filter, plan)
			}
		}
	}
}

// TestJobFilterIndexesReplaceAnInvalidIndexOnPostgres is the recovery the
// concurrent build needs: a CREATE INDEX CONCURRENTLY that fails leaves an
// invalid index under its name, which IF NOT EXISTS would then skip forever and
// the planner never uses. The next pass drops it and builds the real one.
func TestJobFilterIndexesReplaceAnInvalidIndexOnPostgres(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()
	accepted := func(title string) Snapshot {
		return acceptFor(t, svc, deps, Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api", Title: title,
			Replay: ReplayInput{NonReplayable: true},
		})
	}
	a, b, c := accepted("a"), accepted("b"), accepted("c")
	for _, from := range []Snapshot{b, c} {
		if err := svc.Link(deps, LinkRequest{Type: LinkRetryOf, FromJobID: from.ID, ToJobID: a.ID}); err != nil {
			t.Fatalf("link: %v", err)
		}
	}

	// A failed concurrent build: a unique index over duplicate values.
	if err := deps.DB.Exec("DROP INDEX IF EXISTS idx_job_links_inbound").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := deps.DB.Exec("CREATE UNIQUE INDEX CONCURRENTLY idx_job_links_inbound ON job_links (type)").Error; err == nil {
		t.Fatal("setup: the unique build over duplicate types succeeded; it must fail to leave an invalid index")
	}
	indexState := func() (bool, string) {
		var rows []struct {
			Valid bool
			Def   string
		}
		if err := deps.DB.Raw(`SELECT i.indisvalid AS valid, pg_get_indexdef(i.indexrelid) AS def FROM pg_class c
			JOIN pg_index i ON i.indexrelid = c.oid WHERE c.relname = 'idx_job_links_inbound'`).Scan(&rows).Error; err != nil {
			t.Fatalf("inspect: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("idx_job_links_inbound rows = %d, want 1", len(rows))
		}
		return rows[0].Valid, rows[0].Def
	}
	if valid, _ := indexState(); valid {
		t.Fatal("setup: the failed build left a valid index")
	}

	if err := models.EnsureJobFilterIndexes(deps.DB); err != nil {
		t.Fatalf("EnsureJobFilterIndexes: %v", err)
	}
	valid, def := indexState()
	if !valid || !strings.Contains(def, "(to_job_id, type, from_job_id)") || strings.Contains(def, "UNIQUE") {
		t.Fatalf("after the pass: valid=%v %s", valid, def)
	}

	// A valid index under the name but on other columns — one an operator made
	// by hand — is not the index the filters are planned around either.
	if err := deps.DB.Exec("DROP INDEX idx_job_links_inbound").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := deps.DB.Exec("CREATE INDEX idx_job_links_inbound ON job_links (type)").Error; err != nil {
		t.Fatalf("create the wrong shape: %v", err)
	}
	if err := models.EnsureJobFilterIndexes(deps.DB); err != nil {
		t.Fatalf("EnsureJobFilterIndexes over a wrong shape: %v", err)
	}
	if valid, def := indexState(); !valid || !strings.Contains(def, "(to_job_id, type, from_job_id)") {
		t.Fatalf("a wrong-shaped index survived the pass: valid=%v %s", valid, def)
	}
}

// TestJobFilterIndexesWaitForAnotherBuilderOnPostgres is the second server at
// startup: while another holds the build lock it waits instead of skipping, so a
// build that fails over there is still checked and repaired here.
func TestJobFilterIndexesWaitForAnotherBuilderOnPostgres(t *testing.T) {
	deps := newPGDeps(t)
	if err := deps.DB.Exec("DROP INDEX IF EXISTS idx_jobs_state_phase").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	sqlDB, err := deps.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	holder, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if _, err := holder.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", int64(0x4D524A4F42494458)); err != nil {
		t.Fatalf("hold the build lock: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- models.EnsureJobFilterIndexes(deps.DB) }()
	select {
	case err := <-done:
		t.Fatalf("the pass returned (%v) while another server held the build lock; it must wait", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := holder.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", int64(0x4D524A4F42494458)); err != nil {
		t.Fatalf("release: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("EnsureJobFilterIndexes: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the pass did not finish after the lock was released")
	}
	var valid []bool
	if err := deps.DB.Raw(`SELECT i.indisvalid FROM pg_class c JOIN pg_index i ON i.indexrelid = c.oid
		WHERE c.relname = 'idx_jobs_state_phase'`).Scan(&valid).Error; err != nil || len(valid) != 1 || !valid[0] {
		t.Fatalf("idx_jobs_state_phase after the wait: %v, %v", valid, err)
	}
}

// TestJobFilterIndexPassLeavesAOneConnectionPoolFree is the pass on its own
// connection: while it waits for another server's build, an application whose
// pool is a single connection (-max-db-connections=1) still answers.
func TestJobFilterIndexPassLeavesAOneConnectionPoolFree(t *testing.T) {
	app, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := app.AutoMigrate(jobCoreTables()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	appSQL, err := app.DB()
	if err != nil {
		t.Fatal(err)
	}
	appSQL.SetMaxOpenConns(1)

	// Another server holding the build lock, on a connection of its own.
	other, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	otherSQL, _ := other.DB()
	defer otherSQL.Close()
	holder, err := otherSQL.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if _, err := holder.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", int64(0x4D524A4F42494458)); err != nil {
		t.Fatalf("hold the build lock: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- models.EnsureJobFilterIndexesOnOwnConnection(dsn) }()
	time.Sleep(200 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("the pass returned (%v) while the build lock was held", err)
	default:
	}

	answered := make(chan error, 1)
	go func() { answered <- app.Exec("SELECT 1").Error }()
	select {
	case err := <-answered:
		if err != nil {
			t.Fatalf("application query: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the application's only connection was taken by the waiting index pass")
	}

	if _, err := holder.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", int64(0x4D524A4F42494458)); err != nil {
		t.Fatalf("release: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("pass: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the pass did not finish after the lock was released")
	}
}
