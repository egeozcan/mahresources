package database_scopes

import (
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/query_models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newResourceTestDB opens an isolated in-memory SQLite DB and migrates the
// minimal resources table schema needed for resource scope tests.
func newResourceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	if err := db.AutoMigrate(&models.Resource{}); err != nil {
		t.Fatalf("failed to migrate Resource table: %v", err)
	}
	return db
}

// seedResource inserts a minimal Resource row with the given name and contentType.
func seedResource(t *testing.T, db *gorm.DB, name, contentType string) {
	t.Helper()
	r := models.Resource{Name: name, ContentType: contentType}
	if err := db.Create(&r).Error; err != nil {
		t.Fatalf("failed to seed resource %q: %v", name, err)
	}
}

func TestResourceScope_ContentTypes_AllowsListedTypes(t *testing.T) {
	db := newResourceTestDB(t)
	seedResource(t, db, "a.png", "image/png")
	seedResource(t, db, "b.jpg", "image/jpeg")
	seedResource(t, db, "c.pdf", "application/pdf")

	var got []models.Resource
	err := db.Model(&models.Resource{}).
		Scopes(ResourceQuery(&query_models.ResourceSearchQuery{
			ContentTypes: []string{"image/png", "image/jpeg"},
		}, true, db, Visibility{})).
		Find(&got).Error
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	for _, r := range got {
		if r.ContentType == "application/pdf" {
			t.Errorf("pdf should not be in results")
		}
	}
}

// A confined ShowWithSimilar counts only a duplicate the caller can see, and
// says so without binding anything: the scope callback already binds the whole
// allow-list into a scoped statement, and mass edit's bind budget subtracts it
// exactly once.
func TestResourceScope_ShowWithSimilarConfinesTheDuplicateWithoutBinding(t *testing.T) {
	db := newResourceTestDB(t)
	statement := func(vis Visibility) *gorm.Statement {
		var n int64
		return db.Session(&gorm.Session{DryRun: true}).Model(&models.Resource{}).
			Scopes(ResourceQuery(&query_models.ResourceSearchQuery{ShowWithSimilar: true}, true, db, vis)).
			Count(&n).Statement
	}
	open := statement(Visibility{})
	confined := statement(Visibility{Confined: true, RootID: 7})
	denied := statement(Visibility{Confined: true})

	if len(confined.Vars) != len(open.Vars) || len(denied.Vars) != len(open.Vars) {
		t.Fatalf("binds: open %d, confined %d, denied %d", len(open.Vars), len(confined.Vars), len(denied.Vars))
	}
	if sql := open.SQL.String(); strings.Contains(sql, "group_subtree") {
		t.Errorf("an unconfined caller's SQL must not change: %s", sql)
	}
	sql := confined.SQL.String()
	inner := "ih.id <> i.id AND i.resource_id IN (SELECT v.id FROM resources v WHERE v.owner_id IN (WITH RECURSIVE group_subtree"
	if !strings.Contains(sql, inner) || !strings.Contains(sql, "WHERE id = 7") {
		t.Errorf("the duplicate is not confined inside the inner EXISTS: %s", sql)
	}
	if sql := denied.SQL.String(); !strings.Contains(sql, "ih.id <> i.id AND 1 = 0") {
		t.Errorf("deny-all must match no duplicate: %s", sql)
	}
}
