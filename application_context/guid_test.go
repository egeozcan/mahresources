package application_context

import (
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

// createGUIDTestContext creates a minimal in-memory context for GUID tests.
// Each call gets its own isolated database via a unique named memory cache.
func createGUIDTestContext(t *testing.T, name string) *MahresourcesContext {
	t.Helper()

	dsn := "file:" + name + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&models.Group{}, &models.Category{}, &models.ResourceCategory{}); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	config := &MahresourcesConfig{DbType: constants.DbTypeSqlite}
	fs := afero.NewMemMapFs()
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	ctx := NewMahresourcesContext(fs, db, readOnlyDB, config)

	// Ensure default resource category (required by CreateGroup internally)
	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)

	return ctx
}

// TestEnsureGUID_ConcurrentConverges verifies that when multiple goroutines call
// ensureGUID for the same entity simultaneously, they all converge on a single
// non-empty GUID value. This is the core correctness property of the lazy
// backfill design: the atomic conditional UPDATE ensures only one writer wins,
// and all others read back the winner's value.
func TestEnsureGUID_ConcurrentConverges(t *testing.T) {
	ctx := createGUIDTestContext(t, "guid_concurrent")

	// Create a group through normal channel so schema is satisfied.
	group, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Test Group"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	// Manually set the GUID to NULL to simulate a pre-GUID entity that needs backfill.
	if err := ctx.db.Model(&models.Group{}).Where("id = ?", group.ID).Update("guid", nil).Error; err != nil {
		t.Fatalf("nullify GUID: %v", err)
	}

	// Verify the GUID is actually NULL now.
	var check models.Group
	ctx.db.First(&check, group.ID)
	if check.GUID != nil {
		t.Fatalf("expected GUID to be nil after manual null, got %q", *check.GUID)
	}

	const numGoroutines = 10
	results := make([]string, numGoroutines)
	var wg sync.WaitGroup

	// Launch multiple goroutines all calling ensureGUID for the same entity.
	for i := 0; i < numGoroutines; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Pass nil as the existing GUID to force the write-or-read-back path.
			results[i] = ctx.ensureGUID("groups", group.ID, nil)
		}()
	}
	wg.Wait()

	// All goroutines must have returned the same non-empty GUID.
	first := results[0]
	if first == "" {
		t.Fatal("ensureGUID returned an empty string")
	}
	for i, got := range results {
		if got != first {
			t.Errorf("goroutine %d got GUID %q, want %q (same as goroutine 0)", i, got, first)
		}
	}

	// The value in the database must also match.
	var final models.Group
	ctx.db.First(&final, group.ID)
	if final.GUID == nil || *final.GUID != first {
		t.Errorf("DB GUID = %v, want %q", final.GUID, first)
	}
}

// TestEnsureGUID_ExistingGUIDNotOverwritten verifies that ensureGUID returns the
// existing GUID unchanged when one is already present.
func TestEnsureGUID_ExistingGUIDNotOverwritten(t *testing.T) {
	ctx := createGUIDTestContext(t, "guid_existing")

	group, err := ctx.CreateGroup(&query_models.GroupCreator{Name: "Already Has GUID"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	// CreateGroup may assign a GUID; ensure one exists.
	existing := "00000000-0000-7000-8000-000000000001"
	if err := ctx.db.Model(&models.Group{}).Where("id = ?", group.ID).Update("guid", existing).Error; err != nil {
		t.Fatalf("set GUID: %v", err)
	}

	got := ctx.ensureGUID("groups", group.ID, &existing)
	if got != existing {
		t.Errorf("ensureGUID returned %q, want original %q", got, existing)
	}

	// Verify the DB was not modified.
	var check models.Group
	ctx.db.First(&check, group.ID)
	if check.GUID == nil || *check.GUID != existing {
		t.Errorf("DB GUID = %v, want %q", check.GUID, existing)
	}
}
