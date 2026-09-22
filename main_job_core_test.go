package main

import (
	"errors"
	"path/filepath"
	"testing"

	"mahresources/constants"
	"mahresources/models"
)

// migrateJobCore is the startup step that creates the durable job tables and
// seeds the writer epoch every later start preflights against. It is exercised
// here rather than only through the server, because the interesting properties
// are about the database it leaves behind.
func TestJobCoreMigrationSeedsTheWriterEpoch(t *testing.T) {
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, filepath.Join(t.TempDir(), "jobs.db"), "", 0)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := migrateJobCore(db); err != nil {
		t.Fatalf("migrateJobCore: %v", err)
	}

	for _, table := range []string{"jobs", "job_events", "job_event_sequences", "job_links", "job_outputs", models.JobReplayEnvelopeTable, "job_claims", "job_capacity_leases", "job_preferences", models.JobLegacyHandleTable, models.JobWriterEpochTable} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("the durable job core did not create %s", table)
		}
	}

	var epoch models.JobWriterEpoch
	if err := db.Where("id = ?", models.JobWriterEpochRowID).First(&epoch).Error; err != nil {
		t.Fatalf("read seeded writer epoch: %v", err)
	}
	if epoch.MinimumEpoch != models.JobWriterEpochSupported {
		t.Fatalf("seeded epoch = %d, want the supported %d", epoch.MinimumEpoch, models.JobWriterEpochSupported)
	}
	if err := models.CheckJobWriterEpoch(db); err != nil {
		t.Fatalf("a freshly migrated database must pass the preflight: %v", err)
	}

	// Idempotent: a second start must not disturb the row.
	if err := migrateJobCore(db); err != nil {
		t.Fatalf("second migrateJobCore: %v", err)
	}

	// And never lowering: an epoch a later release advanced is left exactly as
	// it is, because lowering it would let this release back into a database it
	// is not permitted to write.
	advanced := models.JobWriterEpochSupported + 1
	if err := db.Model(&models.JobWriterEpoch{}).
		Where("id = ?", models.JobWriterEpochRowID).
		Update("minimum_epoch", advanced).Error; err != nil {
		t.Fatalf("advance epoch: %v", err)
	}
	if err := migrateJobCore(db); err != nil {
		t.Fatalf("migrateJobCore against an advanced epoch: %v", err)
	}
	if err := db.Where("id = ?", models.JobWriterEpochRowID).First(&epoch).Error; err != nil {
		t.Fatalf("reread epoch: %v", err)
	}
	if epoch.MinimumEpoch != advanced {
		t.Fatalf("migration rewrote the advanced epoch to %d", epoch.MinimumEpoch)
	}
	if err := models.CheckJobWriterEpoch(db); !errors.Is(err, models.ErrJobWriterEpochTooNew) {
		t.Fatalf("preflight against an advanced epoch = %v, want ErrJobWriterEpochTooNew", err)
	}
}
