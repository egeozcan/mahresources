//go:build json1 && fts5

package jobs

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/database_scopes"

	"gorm.io/gorm"
)

// TestJobSearchMillionRowsSQLite measures the search box over a million Jobs
// that carry titles and summaries: the summary term as the JSON text it was
// matched as before, and as its values, which is how it is matched now, for a
// term one Job holds (every row is read) and one every Job holds, and the list
// page a search reads. The PostgreSQL case is in query_search_scale_pg_test.go.
func TestJobSearchMillionRowsSQLite(t *testing.T) {
	runJobSearchScaleEvidence(t, "sqlite", newTestDeps(t))
}

func runJobSearchScaleEvidence(t *testing.T, engine string, deps Deps) {
	t.Helper()
	if os.Getenv("MAHRESOURCES_JOB_QUERY_PLAN_MILLION") != "1" {
		t.Skip("set MAHRESOURCES_JOB_QUERY_PLAN_MILLION=1 to seed and measure the million-row search fixture")
	}
	now := time.Now().UTC().Truncate(time.Second)
	deps.Now = func() time.Time { return now }
	seedMillionSearchJobs(t, deps.DB, now)

	operator := database_scopes.GetLikeOperator(deps.DB)
	// Each count runs three times and reports its fastest, so the first query
	// after seeding does not pay for the cache the others find warm.
	for _, term := range []string{"needle-424242", "example", "scheme", "needle-424242&"} {
		pattern, escape := database_scopes.LikePattern(term)
		values, valueArgs := summaryValueMatches(deps.DB, term, pattern, operator, escape)
		for _, predicate := range []struct {
			name string
			sql  string
			args []any
		}{
			{"json-text", "COALESCE(CAST(jobs.summary AS TEXT), '') " + operator + " ?" + escape, []any{pattern}},
			{"values", values, valueArgs},
		} {
			var count int64
			fastest := time.Duration(1<<63 - 1)
			for range 3 {
				started := time.Now()
				if err := deps.DB.Raw("SELECT COUNT(*) FROM jobs WHERE "+predicate.sql, predicate.args...).Scan(&count).Error; err != nil {
					t.Fatalf("%s count for %q: %v", predicate.name, term, err)
				}
				fastest = min(fastest, time.Since(started))
			}
			t.Logf("engine=%s rows=%d shape=summary-count predicate=%s term=%q matches=%d elapsed_ms=%.1f",
				engine, millionJobQueryPlanRows, predicate.name, term, count, float64(fastest.Microseconds())/1000)
			logSearchPlan(t, deps.DB, engine, predicate.name, "SELECT COUNT(*) FROM jobs WHERE "+predicate.sql, predicate.args...)
		}
	}

	// The whole search predicate, as it was (the summary as JSON text) and as it
	// is (applySearch), run alternately five times each so load on the machine
	// falls on both alike; the fastest of each is reported.
	for _, term := range []string{"needle-424242", "scheme", "424242"} {
		pattern, escape := database_scopes.LikePattern(term)
		before := "(jobs.id " + operator + " ?" + escape + " OR jobs.title " + operator + " ?" + escape +
			" OR COALESCE(CAST(jobs.summary AS TEXT), '') " + operator + " ?" + escape +
			" OR jobs.failure_message " + operator + " ?" + escape +
			" OR EXISTS (SELECT 1 FROM job_outputs o WHERE o.job_id = jobs.id AND o.label " + operator + " ?" + escape + "))"
		shapes := []struct {
			name  string
			count func() (int64, error)
		}{
			{"before", func() (int64, error) {
				var count int64
				err := deps.DB.Raw("SELECT COUNT(*) FROM jobs WHERE "+before, pattern, pattern, pattern, pattern, pattern).Scan(&count).Error
				return count, err
			}},
			{"after", func() (int64, error) {
				var count int64
				err := applySearch(deps.DB.Model(&models.Job{}), term).Count(&count).Error
				return count, err
			}},
		}
		fastest := []time.Duration{1<<63 - 1, 1<<63 - 1}
		counts := []int64{0, 0}
		for range 5 {
			for i, shape := range shapes {
				started := time.Now()
				got, err := shape.count()
				if err != nil {
					t.Fatalf("%s search count for %q: %v", shape.name, term, err)
				}
				counts[i] = got
				fastest[i] = min(fastest[i], time.Since(started))
			}
		}
		for i, shape := range shapes {
			t.Logf("engine=%s rows=%d shape=search-predicate-count version=%s term=%q matches=%d fastest_of_5_ms=%.1f",
				engine, millionJobQueryPlanRows, shape.name, term, counts[i], float64(fastest[i].Microseconds())/1000)
		}
	}

	svc := NewService()
	admin := Access{UserID: 1, Administrator: true}
	for _, term := range []string{"needle-424242", "example"} {
		started := time.Now()
		page, err := svc.List(deps, admin, Filter{Search: term}, Cursor{}, 50)
		if err != nil {
			t.Fatalf("list for %q: %v", term, err)
		}
		t.Logf("engine=%s rows=%d shape=search-list term=%q page=%d elapsed_ms=%.1f",
			engine, millionJobQueryPlanRows, term, len(page.Jobs), float64(time.Since(started).Microseconds())/1000)
		started = time.Now()
		counts, err := svc.CountByState(deps, admin, Filter{Search: term})
		if err != nil {
			t.Fatalf("count by state for %q: %v", term, err)
		}
		total := int64(0)
		for _, count := range counts {
			total += count
		}
		t.Logf("engine=%s rows=%d shape=search-count-by-state term=%q total=%d elapsed_ms=%.1f",
			engine, millionJobQueryPlanRows, term, total, float64(time.Since(started).Microseconds())/1000)
	}
}

func logSearchPlan(t *testing.T, db *gorm.DB, engine, predicate, query string, args ...any) {
	t.Helper()
	explain := "EXPLAIN QUERY PLAN "
	if engine == "postgres" {
		explain = "EXPLAIN "
	}
	rows, err := db.Raw(explain+query, args...).Rows()
	if err != nil {
		t.Fatalf("explain %s: %v", predicate, err)
	}
	defer rows.Close()
	columns, _ := rows.Columns()
	var lines []string
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		lines = append(lines, fmt.Sprint(values[len(values)-1]))
	}
	t.Logf("plan engine=%s predicate=%s: %s", engine, predicate, strings.Join(lines, " | "))
}

// seedMillionSearchJobs seeds a million download-shaped Jobs, each titled by a
// file and summarized as a download is, with one Job holding a unique host.
func seedMillionSearchJobs(t *testing.T, db *gorm.DB, now time.Time) {
	t.Helper()
	const batchSize = 500
	const columns = 14
	// PostgreSQL's summary column is jsonb, which a text parameter is not.
	summaryValue := "?"
	if db.Dialector.Name() == "postgres" {
		summaryValue = "CAST(? AS json)"
	}
	value := "(" + strings.Repeat("?,", columns-1) + summaryValue + ")"
	states := []string{"queued", "running", "succeeded", "failed", "cancelled"}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < millionJobQueryPlanRows; start += batchSize {
			end := min(start+batchSize, millionJobQueryPlanRows)
			values := make([]string, 0, end-start)
			args := make([]any, 0, (end-start)*columns)
			for i := start; i < end; i++ {
				acceptedAt := now.Add(-time.Duration(i) * time.Second)
				host := fmt.Sprintf("files-%d.example", i%97)
				if i == 424242 {
					host = "needle-424242.example"
				}
				summary := fmt.Sprintf(`{"scheme":"http","host":%q,"targets":["owner:%d","group:%d"]}`, host, i%20+1, i%1000)
				values = append(values, value)
				args = append(args,
					fmt.Sprintf("00000000-0000-7000-8000-%012x", i+1),
					"remote-download", uint(1), states[i%len(states)], "api", "owner", "actor", "non-replayable",
					uint64(1), uint(i%20+1), acceptedAt, acceptedAt, fmt.Sprintf("photo-%d.jpg", i), summary,
				)
			}
			query := "INSERT INTO jobs (id, kind, kind_version, state, origin, visibility_class, execution_principal, replay_class, version, owner_user_id, accepted_at, state_entered_at, title, summary) VALUES " + strings.Join(values, ",")
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
}
