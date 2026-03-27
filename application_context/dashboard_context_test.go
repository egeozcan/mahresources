package application_context

import (
	"fmt"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/util"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	err = db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
		&models.LogEntry{},
		&models.NoteBlock{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}
	util.AddInitialData(db)

	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")

	config := &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	}
	return NewMahresourcesContext(nil, db, readOnlyDB, config)
}

// TestGetRecentActivity_NoDuplicatesForNewEntities verifies that newly created
// entities (where created_at ~ updated_at) do NOT produce both a "created" and
// an "updated" entry. The bug was that the SQLite updatedFilter compared
// updated_at (which may include sub-second precision or a different format)
// against datetime(created_at, '+1 second') without normalizing updated_at,
// causing false positives for "updated" entries.
func TestGetRecentActivity_NoDuplicatesForNewEntities(t *testing.T) {
	ctx := setupTestContext(t)

	// Create a tag with identical created_at and updated_at
	tag := &models.Tag{Name: "Fresh Tag"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("Failed to create tag: %v", err)
	}

	// Force created_at == updated_at to be identical timestamps
	now := time.Now().Truncate(time.Second)
	ctx.db.Model(tag).Updates(map[string]interface{}{
		"created_at": now,
		"updated_at": now,
	})

	entries, err := ctx.GetRecentActivity(100)
	if err != nil {
		t.Fatalf("GetRecentActivity error: %v", err)
	}

	// Count entries for our tag
	var createdCount, updatedCount int
	for _, entry := range entries {
		if entry.EntityType == "tag" && entry.Name == "Fresh Tag" {
			if entry.Action == "created" {
				createdCount++
			}
			if entry.Action == "updated" {
				updatedCount++
			}
		}
	}

	if createdCount != 1 {
		t.Errorf("expected 1 'created' entry for tag, got %d", createdCount)
	}
	if updatedCount != 0 {
		t.Errorf("expected 0 'updated' entries for tag with identical created_at/updated_at, got %d (duplicate bug)", updatedCount)
	}
}

// TestGetRecentActivity_UpdatedEntriesAppearForRealUpdates verifies that
// entities with updated_at significantly after created_at DO appear as "updated".
func TestGetRecentActivity_UpdatedEntriesAppearForRealUpdates(t *testing.T) {
	ctx := setupTestContext(t)

	tag := &models.Tag{Name: "Updated Tag"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("Failed to create tag: %v", err)
	}

	// Set updated_at to be 10 seconds after created_at
	createdAt := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	updatedAt := createdAt.Add(10 * time.Second)
	ctx.db.Model(tag).Updates(map[string]interface{}{
		"created_at": createdAt,
		"updated_at": updatedAt,
	})

	entries, err := ctx.GetRecentActivity(100)
	if err != nil {
		t.Fatalf("GetRecentActivity error: %v", err)
	}

	var hasCreated, hasUpdated bool
	for _, entry := range entries {
		if entry.EntityType == "tag" && entry.Name == "Updated Tag" {
			if entry.Action == "created" {
				hasCreated = true
			}
			if entry.Action == "updated" {
				hasUpdated = true
			}
		}
	}

	if !hasCreated {
		t.Error("expected a 'created' entry for the tag")
	}
	if !hasUpdated {
		t.Error("expected an 'updated' entry for the tag with updated_at >> created_at")
	}
}
