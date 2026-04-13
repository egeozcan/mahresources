package mrql

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupGUIDTestDB creates an in-memory SQLite database with a guid column
// on the groups table and seeds a group with a known GUID.
// This is kept separate from setupTestDB to avoid modifying the shared
// testGroup struct which is used by many other tests.
func setupGUIDTestDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Migrate the base schema using the shared testGroup struct (no guid column yet).
	if err := db.AutoMigrate(&testGroup{}); err != nil {
		t.Fatalf("auto-migrate failed: %v", err)
	}

	// Add the guid column via ALTER TABLE so the groups table matches the real schema.
	if err := db.Exec(`ALTER TABLE groups ADD COLUMN guid TEXT`).Error; err != nil {
		t.Fatalf("add guid column: %v", err)
	}

	// Seed two groups: one with a known GUID, one without.
	knownGUID := "01957b00-0000-7000-8000-000000000001"
	if err := db.Exec(`INSERT INTO groups (id, name) VALUES (10, 'Alpha')`).Error; err != nil {
		t.Fatalf("insert Alpha: %v", err)
	}
	if err := db.Exec(`UPDATE groups SET guid = ? WHERE id = 10`, knownGUID).Error; err != nil {
		t.Fatalf("set guid on Alpha: %v", err)
	}
	if err := db.Exec(`INSERT INTO groups (id, name) VALUES (11, 'Beta')`).Error; err != nil {
		t.Fatalf("insert Beta: %v", err)
	}
	// Beta intentionally has no GUID (NULL).

	return db, knownGUID
}

// TestTranslateGUIDExact verifies that MRQL can filter groups by an exact GUID
// value. This exercises the FieldString path in the translator for the "guid"
// common field defined in fields.go.
func TestTranslateGUIDExact(t *testing.T) {
	db, knownGUID := setupGUIDTestDB(t)

	result := parseAndTranslate(t, `type = "group" AND guid = "`+knownGUID+`"`, EntityGroup, db)

	// Use a raw-scan struct so we don't depend on testGroup having a GUID field.
	var rows []struct {
		ID   uint   `gorm:"column:id"`
		Name string `gorm:"column:name"`
		GUID string `gorm:"column:guid"`
	}
	if err := result.Scan(&rows).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 group with the known GUID, got %d", len(rows))
	}
	if rows[0].Name != "Alpha" {
		t.Errorf("expected 'Alpha', got %q", rows[0].Name)
	}
	if rows[0].GUID != knownGUID {
		t.Errorf("GUID = %q, want %q", rows[0].GUID, knownGUID)
	}
}

// TestTranslateGUIDIsNull verifies that MRQL can find groups where guid IS NULL.
func TestTranslateGUIDIsNull(t *testing.T) {
	db, _ := setupGUIDTestDB(t)

	result := parseAndTranslate(t, `type = "group" AND guid IS NULL`, EntityGroup, db)

	var rows []struct {
		ID   uint   `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	if err := result.Scan(&rows).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 group with NULL guid, got %d", len(rows))
	}
	if rows[0].Name != "Beta" {
		t.Errorf("expected 'Beta', got %q", rows[0].Name)
	}
}

// TestTranslateGUIDIsNotNull verifies that MRQL can find groups where guid IS NOT NULL.
func TestTranslateGUIDIsNotNull(t *testing.T) {
	db, _ := setupGUIDTestDB(t)

	result := parseAndTranslate(t, `type = "group" AND guid IS NOT NULL`, EntityGroup, db)

	var rows []struct {
		ID   uint   `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	if err := result.Scan(&rows).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 group with a non-NULL guid, got %d", len(rows))
	}
	if rows[0].Name != "Alpha" {
		t.Errorf("expected 'Alpha', got %q", rows[0].Name)
	}
}
