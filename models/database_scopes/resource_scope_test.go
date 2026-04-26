package database_scopes

import (
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
		}, true, db)).
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
