//go:build postgres

package mrql

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"mahresources/internal/testpgutil"

	"gorm.io/gorm"
)

var pgContainer *testpgutil.Container

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	pgContainer, err = testpgutil.StartContainer(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	pgContainer.Stop(ctx)
	os.Exit(code)
}

func setupPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := pgContainer.CreateTestDB(t)

	if err := db.AutoMigrate(&testTag{}, &testGroup{}, &testResource{}, &testNote{}); err != nil {
		t.Fatalf("auto-migrate failed: %v", err)
	}

	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS resource_tags (resource_id INTEGER NOT NULL, tag_id INTEGER NOT NULL, PRIMARY KEY (resource_id, tag_id))`,
		`CREATE TABLE IF NOT EXISTS note_tags (note_id INTEGER NOT NULL, tag_id INTEGER NOT NULL, PRIMARY KEY (note_id, tag_id))`,
		`CREATE TABLE IF NOT EXISTS group_tags (group_id INTEGER NOT NULL, tag_id INTEGER NOT NULL, PRIMARY KEY (group_id, tag_id))`,
		`CREATE TABLE IF NOT EXISTS groups_related_resources (resource_id INTEGER NOT NULL, group_id INTEGER NOT NULL, PRIMARY KEY (resource_id, group_id))`,
		`CREATE TABLE IF NOT EXISTS groups_related_notes (note_id INTEGER NOT NULL, group_id INTEGER NOT NULL, PRIMARY KEY (note_id, group_id))`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("create junction table failed: %v", err)
		}
	}

	seedPostgresTestData(db, t)
	return db
}

func seedPostgresTestData(db *gorm.DB, t *testing.T) {
	t.Helper()
	now := time.Now()
	parentGroupID := uint(1)
	workGroupID := uint(2)

	for _, tag := range []testTag{
		{ID: 1, Name: "photo"},
		{ID: 2, Name: "video"},
		{ID: 3, Name: "document"},
	} {
		db.Create(&tag)
	}

	for _, g := range []testGroup{
		{ID: 1, Name: "Vacation", Meta: `{"region":"europe","priority":3}`},
		{ID: 2, Name: "Work", OwnerID: &parentGroupID, Meta: `{}`},
		{ID: 3, Name: "Archive", Meta: `{}`},
		{ID: 4, Name: "Sub-Work", OwnerID: &workGroupID, Meta: `{}`},
		{ID: 5, Name: "Photos", OwnerID: &parentGroupID, Meta: `{}`},
	} {
		db.Create(&g)
	}

	for _, r := range []testResource{
		{ID: 1, Name: "sunset.jpg", OriginalName: "sunset.jpg", ContentType: "image/jpeg", FileSize: 1024000, Width: 1920, Height: 1080, CreatedAt: now, UpdatedAt: now, Meta: `{"rating":5}`},
		{ID: 2, Name: "photo_album.png", OriginalName: "photo_album.png", ContentType: "image/png", FileSize: 2048000, Width: 800, Height: 600, CreatedAt: now, UpdatedAt: now, Meta: `{"rating":3}`},
		{ID: 3, Name: "report.pdf", OriginalName: "report.pdf", ContentType: "application/pdf", FileSize: 512000, CreatedAt: now, UpdatedAt: now, Meta: `{}`},
		{ID: 4, Name: "untagged_file.txt", OriginalName: "untagged.txt", ContentType: "text/plain", FileSize: 100, CreatedAt: now.Add(-24 * 30 * time.Hour), UpdatedAt: now, Meta: `{}`},
	} {
		db.Create(&r)
	}

	for _, n := range []testNote{
		{ID: 1, Name: "Meeting notes", CreatedAt: now, UpdatedAt: now, Meta: `{"priority":"high"}`},
		{ID: 2, Name: "Todo list", CreatedAt: now, UpdatedAt: now, Meta: `{"priority":"low","count":7}`},
	} {
		db.Create(&n)
	}

	db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (1, 1)")
	db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (2, 1)")
	db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (2, 2)")
	db.Exec("INSERT INTO note_tags (note_id, tag_id) VALUES (1, 3)")
	db.Exec("INSERT INTO note_tags (note_id, tag_id) VALUES (1, 1)")
	db.Exec("INSERT INTO groups_related_resources (resource_id, group_id) VALUES (1, 1)")
	db.Exec("INSERT INTO groups_related_resources (resource_id, group_id) VALUES (3, 2)")
	db.Exec("INSERT INTO groups_related_notes (note_id, group_id) VALUES (1, 1)")
	db.Exec("INSERT INTO groups_related_notes (note_id, group_id) VALUES (2, 2)")
	db.Exec("INSERT INTO group_tags (group_id, tag_id) VALUES (1, 1)")
	db.Exec("INSERT INTO group_tags (group_id, tag_id) VALUES (2, 3)")
	db.Model(&testResource{}).Where("id = ?", 1).Update("owner_id", 1)
	db.Model(&testResource{}).Where("id = ?", 3).Update("owner_id", 2)
	db.Model(&testNote{}).Where("id = ?", 1).Update("owner_id", 1)
	db.Model(&testNote{}).Where("id = ?", 2).Update("owner_id", 2)
}
