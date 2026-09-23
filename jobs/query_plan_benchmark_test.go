//go:build json1 && fts5

package jobs

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

const millionJobQueryPlanRows = 1_000_000

type capturedJobQuery struct {
	sql  string
	vars []any
}

type jobQueryCapture struct {
	mu         sync.Mutex
	statements []capturedJobQuery
}

func (c *jobQueryCapture) record(db *gorm.DB) {
	if db.Error != nil || db.Statement == nil || db.Statement.SQL.Len() == 0 {
		return
	}
	sqlText := db.Statement.SQL.String()
	vars := append([]any(nil), db.Statement.Vars...)
	c.mu.Lock()
	c.statements = append(c.statements, capturedJobQuery{sql: sqlText, vars: vars})
	c.mu.Unlock()
}

func (c *jobQueryCapture) reset() {
	c.mu.Lock()
	c.statements = nil
	c.mu.Unlock()
}

func (c *jobQueryCapture) snapshot() []capturedJobQuery {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]capturedJobQuery(nil), c.statements...)
}

// TestJobQueryPlansMillionRowsSQLite seeds the production Job schema and then
// captures plans and timings from the public list and summary service methods,
// plus the selectors used by retention and dispatch. The matching PostgreSQL
// case lives in query_plan_postgres_test.go.
func TestJobQueryPlansMillionRowsSQLite(t *testing.T) {
	deps := newTestDeps(t)
	runMillionJobQueryPlanEvidence(t, "sqlite", deps)
}

func runMillionJobQueryPlanEvidence(t *testing.T, engine string, deps Deps) {
	t.Helper()
	if os.Getenv("MAHRESOURCES_JOB_QUERY_PLAN_MILLION") != "1" {
		t.Skip("set MAHRESOURCES_JOB_QUERY_PLAN_MILLION=1 to seed and explain the million-row fixture")
	}
	now := time.Now().UTC().Truncate(time.Second)
	deps.Now = func() time.Time { return now }
	seedMillionQueryPlanJobs(t, deps.DB, now)

	accesses := []struct {
		name   string
		access Access
	}{
		{name: "owner-user-1", access: Access{UserID: 1}},
		{name: "administrator", access: Access{Administrator: true}},
	}

	capture := &jobQueryCapture{}
	const callbackName = "job_query_plan_capture"
	if err := deps.DB.Callback().Query().After("gorm:query").Register(callbackName, capture.record); err != nil {
		t.Fatalf("register query capture: %v", err)
	}
	if err := deps.DB.Callback().Row().After("gorm:row").Register(callbackName, capture.record); err != nil {
		t.Fatalf("register row capture: %v", err)
	}
	t.Cleanup(func() {
		_ = deps.DB.Callback().Query().Remove(callbackName)
		_ = deps.DB.Callback().Row().Remove(callbackName)
	})

	svc := NewService()
	for _, tc := range accesses {
		capture.reset()
		started := time.Now()
		page, err := svc.List(deps, tc.access, Filter{}, Cursor{}, 50)
		elapsed := time.Since(started)
		if err != nil {
			t.Fatalf("%s list: %v", tc.name, err)
		}
		allStatements := capture.snapshot()
		statements := onlyJobQueries(allStatements)
		if len(statements) == 0 {
			t.Fatalf("%s list did not capture a jobs SELECT", tc.name)
		}
		if len(allStatements) != 2 {
			t.Fatalf("%s list queried %d tables/statements, want jobs and one page replay lookup: %v", tc.name, len(allStatements), queryTables(allStatements))
		}
		foundPageReplayLookup := false
		for _, statement := range allStatements {
			if queryTable(statement.sql) == "job_replay_envelopes" {
				normalized := strings.NewReplacer("\"", "", "`", "").Replace(strings.ToLower(statement.sql))
				if !strings.Contains(normalized, "job_id in ") || len(statement.vars) != len(page.Jobs) {
					t.Fatalf("%s replay lookup is not limited to its %d listed IDs: sql=%s vars=%d", tc.name, len(page.Jobs), statement.sql, len(statement.vars))
				}
				foundPageReplayLookup = true
			}
		}
		if !foundPageReplayLookup {
			t.Fatalf("%s list did not issue its page replay availability lookup", tc.name)
		}
		t.Logf("engine=%s rows=%d shape=list access=%s page=%d next=%t elapsed_ms=%.2f", engine, millionJobQueryPlanRows, tc.name, len(page.Jobs), page.Next != nil, float64(elapsed.Microseconds())/1000)
		t.Logf("query-scope shape=list access=%s statements=%d tables=%s", tc.name, len(allStatements), strings.Join(queryTables(allStatements), ","))
		logJobQueryPlans(t, deps.DB, engine, "list/"+tc.name, statements)
	}

	for _, tc := range accesses {
		capture.reset()
		started := time.Now()
		summary, err := svc.Summary(deps, tc.access, Filter{}, 0)
		elapsed := time.Since(started)
		if err != nil {
			t.Fatalf("%s summary: %v", tc.name, err)
		}
		allStatements := capture.snapshot()
		statements := onlyJobQueries(allStatements)
		if len(statements) == 0 {
			t.Fatalf("%s summary did not capture jobs queries", tc.name)
		}
		tables := queryTables(allStatements)
		if len(tables) != 1 || tables[0] != "jobs" {
			t.Fatalf("%s summary read unexpected tables: %v", tc.name, tables)
		}
		t.Logf("engine=%s rows=%d shape=summary access=%s window_days=30 total=%d elapsed_ms=%.2f", engine, millionJobQueryPlanRows, tc.name, summary.Total, float64(elapsed.Microseconds())/1000)
		t.Logf("query-scope shape=summary access=%s statements=%d tables=%s", tc.name, len(allStatements), strings.Join(tables, ","))
		logJobQueryPlans(t, deps.DB, engine, "summary/"+tc.name, statements)
	}

	// Retention is a bounded keyset walk. Capture its real boundary and candidate
	// selectors separately so the plan shows both how the cycle finds its high
	// watermark and how it reads the next page of expired terminal Jobs.
	capture.reset()
	started := time.Now()
	bound, err := sweepBound(deps.DB, SweepCursor{}, now)
	boundElapsed := time.Since(started)
	if err != nil {
		t.Fatalf("retention sweep bound: %v", err)
	}
	if bound == nil {
		t.Fatal("million-row fixture has no expired retention candidates")
	}
	boundStatements := onlyJobQueries(capture.snapshot())
	if len(boundStatements) != 1 {
		t.Fatalf("retention boundary queried %d Job statements, want one", len(boundStatements))
	}
	t.Logf("engine=%s rows=%d shape=retention-bound expired=true elapsed_ms=%.2f", engine, millionJobQueryPlanRows, float64(boundElapsed.Microseconds())/1000)
	logJobQueryPlans(t, deps.DB, engine, "retention/bound", boundStatements)

	capture.reset()
	started = time.Now()
	retainedPage, err := expiredJobs(deps.DB, SweepCursor{}, bound, 50, now)
	retentionElapsed := time.Since(started)
	if err != nil {
		t.Fatalf("retention candidate page: %v", err)
	}
	if len(retainedPage) == 0 || len(retainedPage) > 50 {
		t.Fatalf("retention candidate page size = %d, want 1..50", len(retainedPage))
	}
	retentionStatements := onlyJobQueries(capture.snapshot())
	if len(retentionStatements) != 1 {
		t.Fatalf("retention candidate page queried %d Job statements, want one", len(retentionStatements))
	}
	t.Logf("engine=%s rows=%d shape=retention-candidates batch=%d elapsed_ms=%.2f", engine, millionJobQueryPlanRows, len(retainedPage), float64(retentionElapsed.Microseconds())/1000)
	logJobQueryPlans(t, deps.DB, engine, "retention/candidates", retentionStatements)

	// Claim uses this same selector before its guarded state update and durable
	// claim transaction. This measures the per-tick dispatch read against the full
	// Job table without changing the seeded fixture's states.
	capture.reset()
	started = time.Now()
	claimCandidate, found, err := nextClaimable(deps.DB, "download", 1, "", now)
	claimElapsed := time.Since(started)
	if err != nil {
		t.Fatalf("claim candidate: %v", err)
	}
	if !found || claimCandidate.State != string(StateQueued) {
		t.Fatalf("claim candidate found=%t state=%q, want a queued Job", found, claimCandidate.State)
	}
	claimStatements := onlyJobQueries(capture.snapshot())
	if len(claimStatements) != 1 {
		t.Fatalf("claim candidate queried %d Job statements, want one", len(claimStatements))
	}
	t.Logf("engine=%s rows=%d shape=claim-candidate kind=download found=%t state=%s elapsed_ms=%.2f", engine, millionJobQueryPlanRows, found, claimCandidate.State, float64(claimElapsed.Microseconds())/1000)
	logJobQueryPlans(t, deps.DB, engine, "claim/candidate", claimStatements)
}

func onlyJobQueries(statements []capturedJobQuery) []capturedJobQuery {
	var jobsStatements []capturedJobQuery
	for _, statement := range statements {
		if queryTable(statement.sql) == "jobs" {
			jobsStatements = append(jobsStatements, statement)
		}
	}
	return jobsStatements
}

func queryTables(statements []capturedJobQuery) []string {
	seen := make(map[string]bool)
	var tables []string
	for _, statement := range statements {
		table := queryTable(statement.sql)
		if table != "" && !seen[table] {
			seen[table] = true
			tables = append(tables, table)
		}
	}
	return tables
}

func queryTable(sqlText string) string {
	normalized := strings.NewReplacer("\"", "", "`", "").Replace(strings.ToLower(sqlText))
	from := strings.Index(normalized, " from ")
	if from < 0 {
		return ""
	}
	fields := strings.Fields(normalized[from+len(" from "):])
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], ",;")
}

func seedMillionQueryPlanJobs(t *testing.T, db *gorm.DB, now time.Time) {
	t.Helper()
	states := []string{"scheduled", "queued", "running", "paused", "blocked", "succeeded", "failed", "cancelled", "interrupted"}
	kinds := []string{"download", "plugin-action", "import-parse", "import-apply", "export", "reduction", "callback"}
	const batchSize = 500
	const columns = 17
	value := "(" + strings.TrimSuffix(strings.Repeat("?,", columns), ",") + ")"
	window := 90 * 24 * time.Hour
	step := time.Duration(int64(window) / millionJobQueryPlanRows)
	startedRows := 0

	if err := db.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < millionJobQueryPlanRows; start += batchSize {
			end := min(start+batchSize, millionJobQueryPlanRows)
			values := make([]string, 0, end-start)
			args := make([]any, 0, (end-start)*columns)
			for i := start; i < end; i++ {
				state := states[i%len(states)]
				acceptedAt := now.Add(-time.Duration(i) * step)
				var startedAt any
				if i%5 != 0 {
					startedAt = acceptedAt.Add(time.Second)
					startedRows++
				}
				var finishedAt, expiresAt any
				if state == "succeeded" || state == "failed" || state == "cancelled" || state == "interrupted" {
					finished := acceptedAt.Add(2 * time.Minute)
					finishedAt = finished
					expiresAt = finished.Add(30 * 24 * time.Hour)
				}
				failureClass := ""
				if state == "failed" {
					if i%2 == 0 {
						failureClass = "network"
					} else {
						failureClass = "validation"
					}
				}
				values = append(values, value)
				args = append(args,
					fmt.Sprintf("00000000-0000-7000-8000-%012x", i+1),
					kinds[i%len(kinds)], uint(1), state, "ui", "owner", "actor", "non-replayable", uint64(1), uint((i/7)%20+1),
					acceptedAt, startedAt, finishedAt, expiresAt,
					int64((i*17)%900)*int64(time.Second), int64((i*31)%3600)*int64(time.Second), failureClass,
				)
			}
			query := "INSERT INTO jobs (id, kind, kind_version, state, origin, visibility_class, execution_principal, replay_class, version, owner_user_id, accepted_at, started_at, finished_at, expires_at, queue_duration, running_duration, failure_class) VALUES " + strings.Join(values, ",")
			if err := tx.Exec(query, args...).Error; err != nil {
				return fmt.Errorf("insert jobs %d..%d: %w", start, end, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed million Jobs: %v", err)
	}
	if db.Dialector.Name() == "postgres" {
		if err := db.Exec("ANALYZE jobs").Error; err != nil {
			t.Fatalf("analyze million Job fixture: %v", err)
		}
	}
	var storedRows, storedStartedRows int64
	if err := db.Model(&models.Job{}).Count(&storedRows).Error; err != nil {
		t.Fatalf("count million Job fixture: %v", err)
	}
	if err := db.Model(&models.Job{}).Where("started_at IS NOT NULL").Count(&storedStartedRows).Error; err != nil {
		t.Fatalf("count started Jobs: %v", err)
	}
	if storedRows != millionJobQueryPlanRows || storedStartedRows != int64(startedRows) {
		t.Fatalf("fixture rows = %d (%d started), want %d (%d started)", storedRows, storedStartedRows, millionJobQueryPlanRows, startedRows)
	}
	t.Logf("seeded rows=%d owners=20 visibility=owner started=%d accepted_span_days=90 analyze=%t", storedRows, storedStartedRows, db.Dialector.Name() == "postgres")
}

func logJobQueryPlans(t *testing.T, db *gorm.DB, engine, shape string, statements []capturedJobQuery) {
	t.Helper()
	for i, statement := range statements {
		planSQL := "EXPLAIN QUERY PLAN " + statement.sql
		if engine == "postgres" {
			planSQL = "EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) " + statement.sql
		}
		rows, err := db.Raw(planSQL, statement.vars...).Rows()
		if err != nil {
			t.Fatalf("%s query %d explain: %v\nsql=%s", shape, i+1, err, statement.sql)
		}
		columns, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			t.Fatalf("%s query %d plan columns: %v", shape, i+1, err)
		}
		lines := make([]string, 0, 8)
		for rows.Next() {
			values := make([]any, len(columns))
			destinations := make([]any, len(columns))
			for j := range values {
				destinations[j] = &values[j]
			}
			if err := rows.Scan(destinations...); err != nil {
				_ = rows.Close()
				t.Fatalf("%s query %d plan row: %v", shape, i+1, err)
			}
			parts := make([]string, len(values))
			for j, value := range values {
				parts[j] = planValue(value)
			}
			lines = append(lines, strings.Join(parts, " | "))
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("%s query %d plan rows: %v", shape, i+1, err)
		}
		_ = rows.Close()
		t.Logf("plan engine=%s shape=%s query=%d sql=%s", engine, shape, i+1, compactSQL(statement.sql, 260))
		for _, line := range lines {
			t.Logf("  %s", line)
		}
	}
}

func planValue(value any) string {
	switch value := value.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(value)
	default:
		return fmt.Sprint(value)
	}
}

func compactSQL(sqlText string, maxBytes int) string {
	compact := strings.Join(strings.Fields(sqlText), " ")
	if len(compact) <= maxBytes {
		return compact
	}
	return compact[:maxBytes] + "…"
}
