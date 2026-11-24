package mrql

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The month queries [calendar] composes (shortcodes/calendar_handler.go;
// TestCalendarComposeQueries pins the same text there). Each is the author's
// filter in parentheses plus a month range, and must be served by the
// timestamp index rather than a scan of the table.
const (
	calendarListQuery  = `(type = "resource") AND created >= "2026-03-01" AND created < "2026-04-01" ORDER BY created ASC`
	calendarCountQuery = `(type = "resource") AND created >= "2026-03-01" AND created < "2026-04-01" GROUP BY created.day COUNT() ORDER BY created.day ASC`
)

func explainCalendarCount(t *testing.T, query string, build func(*Query) (ExplainStatement, error)) ExplainStatement {
	t.Helper()
	q, err := Parse(query)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got := ExtractEntityType(q); got != EntityResource {
		t.Fatalf("the parentheses hid the entity type: %v", got)
	}
	q.EntityType = EntityResource
	st, err := build(q)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return st
}

func sqlitePlanDetails(t *testing.T, st ExplainStatement) string {
	t.Helper()
	db := setupTestDB(t)
	plan, err := NativeExplain(context.Background(), db, st)
	if err != nil {
		t.Fatalf("native explain: %v", err)
	}
	var rows []SQLitePlanRow
	if err := json.Unmarshal(plan.Plan, &rows); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	var details []string
	for _, r := range rows {
		details = append(details, r.Detail)
	}
	return strings.Join(details, " | ")
}

func TestCalendarQueriesUseTheCreatedIndexSQLite(t *testing.T) {
	db := setupTestDB(t)
	list := explainFlat(t, db, calendarListQuery, EntityResource, TranslateOptions{})
	if got := sqlitePlanDetails(t, list); !strings.Contains(got, "idx_resources_created_at") {
		t.Errorf("list query does not use the created_at index: %s", got)
	}

	count := explainCalendarCount(t, calendarCountQuery, func(q *Query) (ExplainStatement, error) {
		built, err := BuildAggregatedGroupBy(q, db, TranslateOptions{})
		if err != nil {
			return ExplainStatement{}, err
		}
		return ExplainDB(built, "resources", &[]map[string]any{}), nil
	})
	if got := sqlitePlanDetails(t, count); !strings.Contains(got, "idx_resources_created_at") {
		t.Errorf("count query does not use the created_at index: %s", got)
	}
}
