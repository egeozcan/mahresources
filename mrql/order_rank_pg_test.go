//go:build postgres

package mrql

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

// addResourceSearchVector adds a generated tsvector column to resources and seeds
// content whose relevance to "kubernetes" differs, so ORDER BY RANK orders
// deterministically (the shorter matching doc ranks higher).
func addResourceSearchVector(t *testing.T, db *gorm.DB) {
	t.Helper()
	ddl := `ALTER TABLE resources ADD COLUMN search_vector tsvector
		GENERATED ALWAYS AS (to_tsvector('english', coalesce(name,'') || ' ' || coalesce(description,''))) STORED`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("add search_vector: %v", err)
	}
	db.Model(&testResource{}).Where("id = ?", 1).Update("description", "kubernetes migration")
	db.Model(&testResource{}).Where("id = ?", 2).Update("description",
		"kubernetes appears once among many unrelated filler words about databases networking storage caching")
	db.Model(&testResource{}).Where("id = ?", 3).Update("description", "unrelated gardening notes")
}

func TestRankExecutionOrderingPG(t *testing.T) {
	db := setupPostgresTestDB(t)
	addResourceSearchVector(t, db)

	result := parseAndTranslate(t, `type = "resource" AND TEXT ~ "kubernetes" ORDER BY RANK`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 matching resources, got %d (%+v)", len(resources), resources)
	}
	if resources[0].ID != 1 {
		t.Fatalf("expected id 1 (more relevant) first, got %+v", resources)
	}
}

func TestRankSQLShapePG(t *testing.T) {
	db := setupPostgresTestDB(t)
	addResourceSearchVector(t, db)
	q := mustParse(t, `type = "resource" AND TEXT ~ "kubernetes" ORDER BY RANK`)
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	result, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	sql := result.Session(&gorm.Session{DryRun: true}).Find(&[]testResource{}).Statement.SQL.String()
	low := strings.ToLower(sql)
	if !strings.Contains(low, "-ts_rank(resources.search_vector") || !strings.Contains(low, "plainto_tsquery('english'") {
		t.Fatalf("expected -ts_rank/plainto_tsquery ORDER BY, got: %s", sql)
	}
}

// A single-quote in the term is inlined with quote-doubling — no injection.
func TestRankHostileTermQuoteDoublingPG(t *testing.T) {
	db := setupPostgresTestDB(t)
	addResourceSearchVector(t, db)
	q := mustParse(t, `type = "resource" AND TEXT ~ "o'brien" ORDER BY RANK`)
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	result, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	sql := result.Session(&gorm.Session{DryRun: true}).Find(&[]testResource{}).Statement.SQL.String()
	if !strings.Contains(sql, "o''brien") {
		t.Fatalf("expected doubled quote in inlined term, got: %s", sql)
	}
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("exec: %v", err)
	}
	// Table intact (no injection).
	var count int64
	db.Model(&testResource{}).Count(&count)
	if count != 4 {
		t.Fatalf("expected resources intact (4 rows), got %d", count)
	}
}

// Rank on notes (no search_vector column) → FTS-disabled translation error.
func TestRankFTSDisabledErrorPG(t *testing.T) {
	db := setupPostgresTestDB(t)
	q := mustParse(t, `type = "note" AND TEXT ~ "meeting" ORDER BY RANK`)
	q.EntityType = EntityNote
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	_, err := Translate(q, db)
	if err == nil || !strings.Contains(err.Error(), "requires the full-text index") {
		t.Fatalf("expected FTS-disabled error, got %v", err)
	}
}
