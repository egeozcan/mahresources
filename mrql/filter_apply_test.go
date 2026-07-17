package mrql

import (
	"strings"
	"testing"
)

func TestApplyFilterWithOptionsPreservesOuterPagination(t *testing.T) {
	db := setupTestDB(t)
	q, err := ParseFilter(EntityResource, `created >= "2025-01-01"`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q); err != nil {
		t.Fatal(err)
	}
	outer := db.Table("resources").Where("resources.id > ?", 10).Order("resources.created_at DESC").Limit(50)
	filtered, err := ApplyFilterWithOptions(q, outer, TranslateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	st := ExplainDB(filtered, "filtered", &[]map[string]any{})
	upper := strings.ToUpper(st.SQL)
	if strings.Contains(upper, " IN (SELECT") {
		t.Fatalf("filter unexpectedly materialized a self subquery: %s", st.SQL)
	}
	for _, want := range []string{"RESOURCES.ID >", "RESOURCES.CREATED_AT >=", "ORDER BY RESOURCES.CREATED_AT DESC", "LIMIT 50"} {
		if !strings.Contains(upper, want) {
			t.Fatalf("SQL missing %q: %s", want, st.SQL)
		}
	}
}

func TestApplyFilterWithOptionsComposesRelationAndTraversalPredicates(t *testing.T) {
	db := setupTestDB(t)
	for _, expression := range []string{`tags = "photo"`, `ancestors.tags = "photo"`} {
		q, err := ParseFilter(EntityResource, expression)
		if err != nil {
			t.Fatalf("parse %q: %v", expression, err)
		}
		if err := Validate(q); err != nil {
			t.Fatalf("validate %q: %v", expression, err)
		}
		outer := db.Table("resources").Where("resources.id > ?", 10).Order("resources.id DESC").Limit(25)
		filtered, err := ApplyFilterWithOptions(q, outer, TranslateOptions{})
		if err != nil {
			t.Fatalf("apply %q: %v", expression, err)
		}
		sql := strings.ToUpper(ExplainDB(filtered, "filtered", &[]map[string]any{}).SQL)
		if strings.Contains(sql, "RESOURCES.ID IN (SELECT RESOURCES.ID FROM RESOURCES") {
			t.Fatalf("%q used a self-filter subquery: %s", expression, sql)
		}
		for _, want := range []string{"RESOURCES.ID >", "ORDER BY RESOURCES.ID DESC", "LIMIT 25"} {
			if !strings.Contains(sql, want) {
				t.Fatalf("%q SQL missing %q: %s", expression, want, sql)
			}
		}
	}
}
