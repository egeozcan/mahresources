package mrqlbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"

	"mahresources/constants"
	"mahresources/models"
)

func TestPrepareFixtureIsDeterministicAndRefusesReuse(t *testing.T) {
	prepare := func(name string) FixtureManifest {
		path := filepath.Join(t.TempDir(), name+".db")
		db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, path, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
		manifest, err := PrepareFixture(context.Background(), db, PrepareOptions{Profile: TinyProfile(), Dialect: constants.DbTypeSqlite, BatchSize: 50})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := PrepareFixture(context.Background(), db, PrepareOptions{Profile: TinyProfile(), Dialect: constants.DbTypeSqlite, BatchSize: 50}); err == nil {
			t.Fatal("expected marked fixture reuse to be rejected")
		}
		mismatched := manifest
		mismatched.LogicalChecksum = "wrong"
		if err := ValidateFixture(db, mismatched); err == nil {
			t.Fatal("expected mismatched manifest/database pair to be rejected")
		}
		if err := ValidateFixture(db, manifest); err != nil {
			t.Fatalf("validate prepared fixture: %v", err)
		}
		if err := db.Exec("DELETE FROM resource_tags").Error; err != nil {
			t.Fatal(err)
		}
		if err := ValidateFixture(db, manifest); err == nil {
			t.Fatal("expected mutated relation fixture to be rejected")
		}
		return manifest
	}

	first := prepare("first")
	second := prepare("second")
	if first.LogicalChecksum != second.LogicalChecksum {
		t.Fatalf("logical checksums differ: %s != %s", first.LogicalChecksum, second.LogicalChecksum)
	}
	if first.Profile != second.Profile || first.Counts["resources"] != int64(TinyProfile().Resources) {
		t.Fatalf("unexpected manifests: %#v %#v", first, second)
	}
}

func TestValidateFixtureRejectsMembershipPreservingSQLiteFTSCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fts.db")
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, path, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	manifest, err := PrepareFixture(context.Background(), db, PrepareOptions{Profile: TinyProfile(), Dialect: constants.DbTypeSqlite, BatchSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	deleteRow := func(id int, description string) {
		name := fmt.Sprintf("benchmark-resource-%09d", id)
		if err := db.Exec("INSERT INTO resources_fts(resources_fts, rowid, name, description, original_name) VALUES('delete', ?, ?, ?, ?)", id, name, description, name).Error; err != nil {
			t.Fatal(err)
		}
	}
	deleteRow(4, "deterministic fixture content benchmarkneedle")
	deleteRow(6, "deterministic fixture content")
	name := "benchmark-resource-000000006"
	if err := db.Exec("INSERT INTO resources_fts(rowid, name, description, original_name) VALUES(?, ?, ?, ?)", 6, name, "deterministic fixture content benchmarkneedle", name).Error; err != nil {
		t.Fatal(err)
	}
	if err := ValidateFixture(db, manifest); err == nil {
		t.Fatal("expected corrupted FTS index to be rejected")
	}
}

func TestFixtureSeedChangesLogicalDatasetAndBatchCapacityIsBounded(t *testing.T) {
	prepare := func(seed int64) FixtureManifest {
		path := filepath.Join(t.TempDir(), "seed.db")
		db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, path, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
		profile := TinyProfile()
		profile.Seed = seed
		manifest, err := PrepareFixture(context.Background(), db, PrepareOptions{Profile: profile, Dialect: constants.DbTypeSqlite, BatchSize: 50})
		if err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	first := prepare(1)
	second := prepare(2)
	if first.LogicalChecksum == second.LogicalChecksum {
		t.Fatal("different seeds produced the same logical checksum")
	}
	if got := relationBatchCapacity(2_999_500, 3_000_000); got != 250 {
		t.Fatalf("large-offset batch capacity = %d, want 250", got)
	}
}

func TestFixtureChecksumIncludesBenchmarkVisibleContent(t *testing.T) {
	checksum := func(description string) string {
		h := sha256.New()
		checksumModel(h, "resource-category", models.ResourceCategory{ID: 1, Name: "benchmark", Description: description, CustomMRQLResult: "<p>template</p>"})
		return hex.EncodeToString(h.Sum(nil))
	}
	if checksum("first") == checksum("second") {
		t.Fatal("benchmark-visible content did not change checksum")
	}
}

func TestPrepareFixtureHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cancel.db")
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, path, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = PrepareFixture(ctx, db, PrepareOptions{Profile: TinyProfile(), Dialect: constants.DbTypeSqlite, BatchSize: 50})
	if err == nil {
		t.Fatal("expected cancellation")
	}
}
