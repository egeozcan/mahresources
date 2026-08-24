//go:build postgres && json1 && fts5

package application_context

import (
	"strings"
	"testing"

	"mahresources/models"
)

// EnsureResourceHashUnique is reached from main(), so no SQLite test and no
// api_test exercises its Postgres path — and the two engines differ in exactly
// the places this function touches: the index-existence probe is a pg_class
// query rather than a sqlite_master one, and a partial unique index is a
// statement whose syntax SQLite accepting proves nothing about.
func TestEnsureResourceHashUniquePG(t *testing.T) {
	db := pgContainer.CreateTestDB(t)
	if err := db.AutoMigrate(&models.Resource{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Rows that were never hashed all carry the empty string. An unqualified
	// unique index would reject every one of them after the first, which is why
	// the index is partial.
	for i := 0; i < 3; i++ {
		if err := db.Create(&models.Resource{Name: "unhashed", Hash: ""}).Error; err != nil {
			t.Fatalf("seed unhashed row %d: %v", i, err)
		}
	}
	if err := db.Create(&models.Resource{Name: "hashed", Hash: "aaa", HashType: "SHA1"}).Error; err != nil {
		t.Fatalf("seed hashed row: %v", err)
	}

	if err := models.EnsureResourceHashUnique(db); err != nil {
		t.Fatalf("create the partial unique index on Postgres: %v", err)
	}

	// Idempotent, and the second call must take the cheap pg_class path rather
	// than re-scanning for duplicates.
	if err := models.EnsureResourceHashUnique(db); err != nil {
		t.Fatalf("second call should be a no-op: %v", err)
	}

	// Unhashed rows stay insertable...
	if err := db.Create(&models.Resource{Name: "unhashed-4", Hash: ""}).Error; err != nil {
		t.Fatalf("the partial index must not constrain unhashed rows: %v", err)
	}

	// ...and a repeated content hash is refused, which is the whole point.
	err := db.Create(&models.Resource{Name: "dupe", Hash: "aaa", HashType: "SHA1"}).Error
	if err == nil {
		t.Fatal("a second row with the same content hash must be rejected by the index")
	}
	if !isUniqueConstraintError(err) {
		t.Fatalf("expected a unique-constraint violation so AddResource can resolve it as a duplicate, got: %v", err)
	}
}

// TestEnsureResourceHashUniquePG_SkipsWhenDuplicatesExist pins the
// non-destructive half on Postgres too: an existing database that already holds
// duplicate hashes keeps every row and keeps working.
func TestEnsureResourceHashUniquePG_SkipsWhenDuplicatesExist(t *testing.T) {
	db := pgContainer.CreateTestDB(t)
	if err := db.AutoMigrate(&models.Resource{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := db.Create(&models.Resource{Name: "legacy", Hash: "collides", HashType: "SHA1"}).Error; err != nil {
			t.Fatalf("seed duplicate row %d: %v", i, err)
		}
	}

	err := models.EnsureResourceHashUnique(db)
	if err == nil {
		t.Fatal("expected a warning naming the duplicates rather than a silent skip")
	}
	if !strings.Contains(err.Error(), "collides") {
		t.Fatalf("the warning must name the colliding hash, got: %v", err)
	}

	var stored int64
	db.Model(&models.Resource{}).Where("hash = ?", "collides").Count(&stored)
	if stored != 2 {
		t.Fatalf("the fixup must not delete anything, found %d of 2 rows", stored)
	}
}
