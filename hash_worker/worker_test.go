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

func TestSimilarityQuery_UnionBothDirections(t *testing.T) {
	db := setupTestDB(t)

	// Create similarity records with various Hamming distances
	// Resource 5 is similar to resources 1, 2, 3 with different distances
	similarities := []models.ResourceSimilarity{
		{ResourceID1: 1, ResourceID2: 5, HammingDistance: 3}, // 5 is larger, stored as (1, 5)
		{ResourceID1: 2, ResourceID2: 5, HammingDistance: 1}, // closest match
		{ResourceID1: 3, ResourceID2: 5, HammingDistance: 5}, // furthest match
		{ResourceID1: 5, ResourceID2: 10, HammingDistance: 2}, // 5 is smaller, stored as (5, 10)
	}

	for _, s := range similarities {
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("Failed to create similarity: %v", err)
		}
	}

	// Query similar resources for resource 5 using UNION ALL query
	var similarIDs []uint
	rows, err := db.Raw(`
		SELECT resource_id2 as similar_id, hamming_distance FROM resource_similarities WHERE resource_id1 = ?
		UNION ALL
		SELECT resource_id1 as similar_id, hamming_distance FROM resource_similarities WHERE resource_id2 = ?
		ORDER BY hamming_distance ASC
	`, 5, 5).Rows()
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id uint
		var dist int
		if err := rows.Scan(&id, &dist); err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		similarIDs = append(similarIDs, id)
	}

	// Should find 4 similar resources: 10 (dist 2 from resource_id_1=5),
	// 1, 2, 3 (from resource_id_2=5)
	if len(similarIDs) != 4 {
		t.Errorf("Expected 4 similar resources, got %d: %v", len(similarIDs), similarIDs)
	}

	// Verify ordering by Hamming distance (ascending)
	expectedOrder := []uint{2, 10, 1, 3} // distances: 1, 2, 3, 5
	for i, id := range similarIDs {
		if i < len(expectedOrder) && id != expectedOrder[i] {
			t.Errorf("Position %d: got resource %d, want %d", i, id, expectedOrder[i])
		}
	}
}

func TestSimilarityQuery_NoResults(t *testing.T) {
	db := setupTestDB(t)

	// Query for a resource with no similarities
	var similarIDs []uint
	rows, err := db.Raw(`
		SELECT resource_id2 as similar_id, hamming_distance FROM resource_similarities WHERE resource_id1 = ?
		UNION ALL
		SELECT resource_id1 as similar_id, hamming_distance FROM resource_similarities WHERE resource_id2 = ?
		ORDER BY hamming_distance ASC
	`, 999, 999).Rows()
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id uint
		var dist int
		if err := rows.Scan(&id, &dist); err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		similarIDs = append(similarIDs, id)
	}

	if len(similarIDs) != 0 {
		t.Errorf("Expected 0 similar resources, got %d", len(similarIDs))
	}
}
