package hash_worker

import (
	"testing"
	"time"

	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"mahresources/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(
		&models.Resource{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
	); err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	return db
}

func TestHashWorker_MigrateStringHashes(t *testing.T) {
	db := setupTestDB(t)

	// Create a resource first (needed for foreign key)
	resource := models.Resource{Name: "test"}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	// Create a hash with old string format
	// Use values without high bit set to avoid SQLite uint64 limitations in tests
	// (In production, PostgreSQL handles full uint64 range)
	hash := models.ImageHash{
		AHash:      "1234567890abcdef",
		DHash:      "0fedcba987654321",
		ResourceId: &resource.ID,
	}
	if err := db.Create(&hash).Error; err != nil {
		t.Fatalf("Failed to create hash: %v", err)
	}

	// Create worker and run migration
	w := New(db, afero.NewMemMapFs(), nil, Config{
		WorkerCount:         1,
		BatchSize:           100,
		PollInterval:        time.Hour,
		SimilarityThreshold: 10,
	})

	w.migrateStringHashes()

	// Verify migration
	var updated models.ImageHash
	if err := db.First(&updated, hash.ID).Error; err != nil {
		t.Fatalf("Failed to load hash: %v", err)
	}

	if updated.AHashInt == nil || updated.DHashInt == nil {
		t.Fatal("Hash not migrated to uint64")
	}

	expectedAHash := uint64(0x1234567890abcdef)
	expectedDHash := uint64(0x0fedcba987654321)

	if *updated.AHashInt != expectedAHash {
		t.Errorf("AHashInt = %x, want %x", *updated.AHashInt, expectedAHash)
	}
	if *updated.DHashInt != expectedDHash {
		t.Errorf("DHashInt = %x, want %x", *updated.DHashInt, expectedDHash)
	}
}

func TestHashWorker_FindSimilarities(t *testing.T) {
	db := setupTestDB(t)
	w := New(db, afero.NewMemMapFs(), nil, Config{
		WorkerCount:         1,
		BatchSize:           100,
		PollInterval:        time.Hour,
		SimilarityThreshold: 10,
	})

	// Seed cache with some hashes
	// Use values without high bit set to avoid SQLite uint64 limitations
	w.hashCache[1] = 0x7F00FF00FF00FF00 // Base hash
	w.hashCache[2] = 0x7F00FF00FF00FF01 // 1 bit different (similar)
	w.hashCache[3] = 0x00FF00FF00FF00FF // Many bits different (not similar)
	w.cacheLoaded = true

	// Find similarities for a new hash that's similar to #1 and #2
	newHash := uint64(0x7F00FF00FF00FF00) // Identical to #1
	w.findAndStoreSimilarities(100, newHash)

	// Verify similarities were stored
	var similarities []models.ResourceSimilarity
	if err := db.Find(&similarities).Error; err != nil {
		t.Fatalf("Failed to query similarities: %v", err)
	}

	if len(similarities) != 2 {
		t.Errorf("Expected 2 similarities, got %d", len(similarities))
	}

	// Verify ordering (ResourceID1 < ResourceID2)
	for _, sim := range similarities {
		if sim.ResourceID1 >= sim.ResourceID2 {
			t.Errorf("Similarity has incorrect ordering: %d >= %d", sim.ResourceID1, sim.ResourceID2)
		}
	}
}
