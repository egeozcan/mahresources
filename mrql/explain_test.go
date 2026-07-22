package mrql

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// explainFlat parses/validates/translates a flat query and returns its explain
// statement (SQLite dialect).
func explainFlat(t *testing.T, db *gorm.DB, query string, et EntityType, opts TranslateOptions) ExplainStatement {
	t.Helper()
	q, err := Parse(query)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = et
	built, err := TranslateWithOptions(q, db, opts)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	return ExplainDB(built, "resources", &[]map[string]any{})
}

func TestExplainDB_FlatShape(t *testing.T) {
	db := setupTestDB(t)
	st := explainFlat(t, db, `type = "resource" AND contentType ~ "image/*"`, EntityResource, TranslateOptions{})
	if !strings.Contains(st.SQL, "resources") {
		t.Errorf("expected table in SQL: %s", st.SQL)
	}
	if !strings.Contains(st.SQL, "?") {
		t.Errorf("expected bind placeholder: %s", st.SQL)
	}
	if len(st.Vars) == 0 {
		t.Errorf("expected vars")
	}
	if !strings.Contains(st.Interpolated, "image/") {
		t.Errorf("expected interpolated value: %s", st.Interpolated)
	}
}

func TestExplainDB_AggregatedShape(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() HAVING COUNT() > 1`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q); err != nil {
		t.Fatal(err)
	}
	q.EntityType = EntityResource
	built, err := BuildAggregatedGroupBy(q, db, TranslateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	st := ExplainDB(built, "resource", &[]map[string]any{})
	up := strings.ToUpper(st.SQL)
	if !strings.Contains(up, "GROUP BY") {
		t.Errorf("expected GROUP BY: %s", st.SQL)
	}
	if !strings.Contains(up, "HAVING") {
		t.Errorf("expected HAVING: %s", st.SQL)
	}
	if !strings.Contains(up, "COUNT") {
		t.Errorf("expected COUNT: %s", st.SQL)
	}
}

func TestExplainDB_BucketKeysShape(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q); err != nil {
		t.Fatal(err)
	}
	q.EntityType = EntityResource
	built, err := BuildGroupByKeys(q, db, TranslateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	st := ExplainDB(built, "bucket keys", &[]map[string]any{})
	if !strings.Contains(strings.ToUpper(st.SQL), "GROUP BY") {
		t.Errorf("expected GROUP BY in keys query: %s", st.SQL)
	}
}

func TestExplainDB_ForcedScopeAppears(t *testing.T) {
	db := setupTestDB(t)
	// Scope group 1 (Vacation) — the scope CTE should appear in the SQL.
	st := explainFlat(t, db, `type = "resource"`, EntityResource, TranslateOptions{ScopeGroupID: 1})
	up := strings.ToUpper(st.SQL)
	if !strings.Contains(up, "RECURSIVE") && !strings.Contains(up, "WITH ") {
		t.Errorf("expected scope CTE in scoped explain SQL: %s", st.SQL)
	}
}

func TestNativeExplainSQLiteReturnsDialectPlan(t *testing.T) {
	db := setupTestDB(t)
	st := explainFlat(t, db, `type = "resource" AND name = "Vacation.jpg"`, EntityResource, TranslateOptions{})
	plan, err := NativeExplain(context.Background(), db, st)
	if err != nil {
		t.Fatalf("native explain: %v", err)
	}
	if plan.Dialect != "sqlite" || plan.Format != "query-plan" {
		t.Fatalf("unexpected envelope: %#v", plan)
	}
	var rows []SQLitePlanRow
	if err := json.Unmarshal(plan.Plan, &rows); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if len(rows) == 0 || !strings.Contains(strings.ToLower(rows[0].Detail), "resources") {
		t.Fatalf("unexpected SQLite plan rows: %#v", rows)
	}
}

type unsupportedExplainDialector struct{ gorm.Dialector }

func (unsupportedExplainDialector) Name() string { return "unsupported" }

func TestNativeExplainRejectsUnsupportedDialect(t *testing.T) {
	db, err := gorm.Open(unsupportedExplainDialector{Dialector: sqlite.Open(":memory:")}, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NativeExplain(context.Background(), db, ExplainStatement{Label: "x", SQL: "SELECT 1"})
	if !errors.Is(err, ErrNativeExplainUnsupportedDialect) {
		t.Fatalf("expected unsupported dialect error, got %v", err)
	}
}

func TestNativeExplainHonorsCancelledContext(t *testing.T) {
	db := setupTestDB(t)
	st := explainFlat(t, db, `type = "resource"`, EntityResource, TranslateOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NativeExplain(ctx, db, st)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
