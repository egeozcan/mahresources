//go:build postgres

package mrql

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

func explainFlatPG(t *testing.T, db *gorm.DB, query string, et EntityType, opts TranslateOptions) ExplainStatement {
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

func TestExplainDB_FlatShapePG(t *testing.T) {
	db := setupPostgresTestDB(t)
	st := explainFlatPG(t, db, `type = "resource" AND contentType ~ "image/*"`, EntityResource, TranslateOptions{})
	if !strings.Contains(st.SQL, "resources") {
		t.Errorf("expected table in SQL: %s", st.SQL)
	}
	// Postgres uses $N bind placeholders.
	if !strings.Contains(st.SQL, "$1") {
		t.Errorf("expected $1 bind placeholder: %s", st.SQL)
	}
	if !strings.Contains(strings.ToUpper(st.SQL), "ILIKE") {
		t.Errorf("expected ILIKE on postgres: %s", st.SQL)
	}
	if len(st.Vars) == 0 {
		t.Errorf("expected vars")
	}
	if !strings.Contains(st.Interpolated, "image/") {
		t.Errorf("expected interpolated value: %s", st.Interpolated)
	}
}

func TestExplainDB_AggregatedShapePG(t *testing.T) {
	db := setupPostgresTestDB(t)
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
	if !strings.Contains(up, "GROUP BY") || !strings.Contains(up, "HAVING") {
		t.Errorf("expected GROUP BY ... HAVING: %s", st.SQL)
	}
}

func TestExplainDB_ForcedScopeAppearsPG(t *testing.T) {
	db := setupPostgresTestDB(t)
	st := explainFlatPG(t, db, `type = "resource"`, EntityResource, TranslateOptions{ScopeGroupID: 1})
	if !strings.Contains(strings.ToUpper(st.SQL), "RECURSIVE") {
		t.Errorf("expected scope CTE in scoped explain SQL: %s", st.SQL)
	}
}
