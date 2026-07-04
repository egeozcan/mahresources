//go:build json1 && fts5

package mrql

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupRankTestDB builds an in-memory SQLite DB with an FTS5 index over resources
// and notes, seeded with content whose relevance to "kubernetes" differs so
// ORDER BY RANK produces a deterministic order (bm25 length-normalizes: the
// shorter matching doc ranks higher).
func setupRankTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&testResource{}, &testNote{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	resources := []testResource{
		{ID: 1, Name: "doc_a", Description: "kubernetes migration", CreatedAt: now, UpdatedAt: now, Meta: `{}`},
		{ID: 2, Name: "doc_b", Description: "kubernetes appears once here among many unrelated filler words about databases networking storage and caching layers", CreatedAt: now, UpdatedAt: now, Meta: `{}`},
		{ID: 3, Name: "doc_c", Description: "completely unrelated document about gardening", CreatedAt: now, UpdatedAt: now, Meta: `{}`},
	}
	for _, r := range resources {
		db.Create(&r)
	}

	// FTS5 external-content index over resources(name, description).
	for _, ddl := range []string{
		`CREATE VIRTUAL TABLE resources_fts USING fts5(name, description, content='resources', content_rowid='id')`,
		`INSERT INTO resources_fts(resources_fts) VALUES('rebuild')`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("fts setup: %v", err)
		}
	}
	return db
}

// --- Validation matrix ---

func TestRankValidationNoText(t *testing.T) {
	q := mustParse(t, `type = "resource" ORDER BY RANK`)
	q.EntityType = EntityResource
	err := Validate(q)
	if err == nil || !strings.Contains(err.Error(), "requires a TEXT ~ predicate") {
		t.Fatalf("expected no-TEXT rejection, got %v", err)
	}
}

func TestRankValidationTwoText(t *testing.T) {
	q := mustParse(t, `type = "resource" AND TEXT ~ "a" AND TEXT ~ "b" ORDER BY RANK`)
	q.EntityType = EntityResource
	err := Validate(q)
	if err == nil || !strings.Contains(err.Error(), "ambiguous with multiple TEXT") {
		t.Fatalf("expected ambiguous rejection, got %v", err)
	}
}

func TestRankValidationGroupBy(t *testing.T) {
	q := mustParse(t, `type = "resource" AND TEXT ~ "a" GROUP BY hash COUNT() ORDER BY RANK`)
	q.EntityType = EntityResource
	err := Validate(q)
	if err == nil || !strings.Contains(err.Error(), "not supported with GROUP BY") {
		t.Fatalf("expected GROUP BY rejection, got %v", err)
	}
}

func TestRankValidationCrossEntity(t *testing.T) {
	// No type filter, TEXT present → entity unspecified → rejected.
	q := mustParse(t, `TEXT ~ "a" ORDER BY RANK`)
	err := Validate(q)
	if err == nil || !strings.Contains(err.Error(), "requires a single entity type") {
		t.Fatalf("expected cross-entity rejection, got %v", err)
	}
}

func TestRankValidationAccepts(t *testing.T) {
	q := mustParse(t, `type = "resource" AND TEXT ~ "kubernetes" ORDER BY RANK`)
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

// meta.rank (two parts) is never captured as the rank key.
func TestRankMetaKeyNotCaptured(t *testing.T) {
	q := mustParse(t, `type = "resource" AND TEXT ~ "x" ORDER BY meta.rank`)
	q.EntityType = EntityResource
	// meta.rank is a sortable meta field, not the rank relevance key.
	if err := Validate(q); err != nil {
		t.Fatalf("meta.rank should validate as a normal meta sort, got %v", err)
	}
}

// --- SQL shape ---

func TestRankSQLShapeSQLite(t *testing.T) {
	db := setupRankTestDB(t)
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
	up := strings.ToUpper(sql)
	if !strings.Contains(up, "BM25(RESOURCES_FTS)") || !strings.Contains(up, "COALESCE") || !strings.Contains(up, "1E9") {
		t.Fatalf("expected bm25 COALESCE sentinel in ORDER BY, got: %s", sql)
	}
}

// Hostile terms are sanitized to a quote-free alphabet on SQLite; no injection.
// (The double-quote/backslash cases are covered on Postgres, where the raw term
// is inlined with quote-doubling; SQLite strips everything but [a-zA-Z0-9 .,].)
func TestRankHostileTermSQLite(t *testing.T) {
	db := setupRankTestDB(t)
	for _, term := range []string{`o'brien`, `foo AND bar`, `DROP TABLE resources`} {
		q := mustParse(t, `type = "resource" AND TEXT ~ "`+term+`" ORDER BY RANK`)
		q.EntityType = EntityResource
		if err := Validate(q); err != nil {
			t.Fatalf("validate %q: %v", term, err)
		}
		result, err := Translate(q, db)
		if err != nil {
			t.Fatalf("translate %q: %v", term, err)
		}
		var resources []testResource
		if err := result.Find(&resources).Error; err != nil {
			t.Fatalf("exec %q: %v", term, err)
		}
	}
	// Table still exists (no injection executed the DROP).
	var count int64
	db.Model(&testResource{}).Count(&count)
	if count != 3 {
		t.Fatalf("expected resources table intact (3 rows), got %d", count)
	}
}

// --- Execution ordering ---

func TestRankExecutionOrderingSQLite(t *testing.T) {
	db := setupRankTestDB(t)
	result := parseAndTranslate(t, `type = "resource" AND TEXT ~ "kubernetes" ORDER BY RANK`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 matching resources, got %d (%+v)", len(resources), resources)
	}
	// doc_a is the shorter matching doc → higher relevance → first.
	if resources[0].ID != 1 {
		t.Fatalf("expected doc_a (id 1) first by relevance, got %+v", resources)
	}
}

// FTS disabled (no _fts table) → translation error, not a silent LIKE fallback.
func TestRankFTSDisabledError(t *testing.T) {
	db := setupTestDB(t) // no resources_fts table
	q := mustParse(t, `type = "resource" AND TEXT ~ "kubernetes" ORDER BY RANK`)
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	_, err := Translate(q, db)
	if err == nil || !strings.Contains(err.Error(), "requires the full-text index") {
		t.Fatalf("expected FTS-disabled error, got %v", err)
	}
}

// Empty term: predicate and ordering both drop; no error, all rows returned.
func TestRankEmptyTermNoOp(t *testing.T) {
	db := setupRankTestDB(t)
	result := parseAndTranslate(t, `type = "resource" AND TEXT ~ "" ORDER BY RANK`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(resources) != 3 {
		t.Fatalf("empty-term should drop predicate+ordering, expected all 3 rows, got %d", len(resources))
	}
}
