//go:build postgres

package mrql

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// The PostgreSQL side of TestCalendarQueriesUseTheCreatedIndexSQLite. The test
// table is tiny, so the planner would scan it whatever indexes exist; with
// sequential scans priced out, a plan that still cannot use the created_at
// index means the predicate is not sargable.
func TestCalendarQueriesUseTheCreatedIndexPG(t *testing.T) {
	db := setupPostgresTestDB(t)
	plan := func(st ExplainStatement) string {
		t.Helper()
		var out string
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SET LOCAL enable_seqscan = off").Error; err != nil {
				return err
			}
			p, err := NativeExplain(context.Background(), tx, st)
			if err != nil {
				return err
			}
			out = string(p.Plan)
			return nil
		})
		if err != nil {
			t.Fatalf("native explain: %v", err)
		}
		return out
	}

	list := explainFlatPG(t, db, calendarListQuery, EntityResource, TranslateOptions{})
	if got := plan(list); !strings.Contains(got, "idx_resources_created_at") {
		t.Errorf("list query does not use the created_at index: %s", got)
	}
	count := explainCalendarCount(t, calendarCountQuery, func(q *Query) (ExplainStatement, error) {
		built, err := BuildAggregatedGroupBy(q, db, TranslateOptions{})
		if err != nil {
			return ExplainStatement{}, err
		}
		return ExplainDB(built, "resources", &[]map[string]any{}), nil
	})
	if got := plan(count); !strings.Contains(got, "idx_resources_created_at") {
		t.Errorf("count query does not use the created_at index: %s", got)
	}
}
