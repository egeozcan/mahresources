package application_context

import (
	"errors"
	"path/filepath"
	"testing"

	"mahresources/constants"
	"mahresources/models"
)

// openWriterEpochDatabase creates a SQLite database whose only table is the
// writer epoch, set to the given value. It is the shape a database written by a
// newer release has when an older binary starts against it: the epoch exists and
// nothing newer has been migrated yet.
func openWriterEpochDatabase(t *testing.T, minimum uint64) string {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "advanced.db")
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, dsn, "", 0)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.JobWriterEpoch{}); err != nil {
		t.Fatalf("migrate epoch table: %v", err)
	}
	if err := db.Create(&models.JobWriterEpoch{ID: models.JobWriterEpochRowID, MinimumEpoch: minimum}).Error; err != nil {
		t.Fatalf("seed epoch: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	return dsn
}

// TestJobWriterEpochPreflightRefusesADatabaseAdvancedByANewerRelease proves the
// refusal happens on the real startup path, and that it happens before anything
// else touches the database: the context is never built, no table is created,
// and no row is written.
func TestJobWriterEpochPreflightRefusesADatabaseAdvancedByANewerRelease(t *testing.T) {
	advanced := models.JobWriterEpochSupported + 1
	dsn := openWriterEpochDatabase(t, advanced)

	ctx, db, fs, err := OpenContextWithConfig(&MahresourcesInputConfig{
		DbType:       constants.DbTypeSqlite,
		DbDsn:        dsn,
		FileSavePath: t.TempDir(),
	})
	if !errors.Is(err, models.ErrJobWriterEpochTooNew) {
		t.Fatalf("OpenContextWithConfig error = %v, want ErrJobWriterEpochTooNew", err)
	}
	if ctx != nil || db != nil || fs != nil {
		t.Fatalf("a refused start returned partial values: ctx=%v db=%v fs=%v", ctx, db, fs)
	}

	// Nothing ran: the epoch table is still the only table, and its value is
	// untouched. A refusal that had migrated or seeded first would show either.
	reopened, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, dsn, "", 0)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	reopenedDB, err := reopened.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	defer reopenedDB.Close()

	var tables []string
	if err := reopened.Raw(
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name",
	).Scan(&tables).Error; err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) != 1 || tables[0] != models.JobWriterEpochTable {
		t.Fatalf("tables after the refusal = %v, want only %s", tables, models.JobWriterEpochTable)
	}

	var epoch models.JobWriterEpoch
	if err := reopened.Where("id = ?", models.JobWriterEpochRowID).First(&epoch).Error; err != nil {
		t.Fatalf("reread epoch: %v", err)
	}
	if epoch.MinimumEpoch != advanced {
		t.Fatalf("the refusal rewrote the epoch to %d", epoch.MinimumEpoch)
	}
}

// TestJobWriterEpochFreshDatabaseBootsAndSeedsTheSupportedEpoch is the other
// half: a database with no job tables at all is a fresh (or pre-Job) database
// and is allowed, and the supported epoch is what startup records for the next
// release to check.
func TestJobWriterEpochFreshDatabaseBootsAndSeedsTheSupportedEpoch(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "fresh.db")

	ctx, db, fs, err := OpenContextWithConfig(&MahresourcesInputConfig{
		DbType:       constants.DbTypeSqlite,
		DbDsn:        dsn,
		FileSavePath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenContextWithConfig on a fresh database: %v", err)
	}
	if ctx == nil || db == nil || fs == nil {
		t.Fatal("a successful start must return its context, handle and filesystem")
	}

	// Startup's own sequence: migrate the core (which creates the epoch table),
	// then seed the epoch.
	if err := db.AutoMigrate(&models.JobWriterEpoch{}); err != nil {
		t.Fatalf("migrate epoch table: %v", err)
	}
	if err := models.EnsureJobWriterEpoch(db); err != nil {
		t.Fatalf("EnsureJobWriterEpoch: %v", err)
	}

	var epoch models.JobWriterEpoch
	if err := db.Where("id = ?", models.JobWriterEpochRowID).First(&epoch).Error; err != nil {
		t.Fatalf("read seeded epoch: %v", err)
	}
	if epoch.MinimumEpoch != models.JobWriterEpochDualPublisher {
		t.Fatalf("seeded epoch = %d, want %d", epoch.MinimumEpoch, models.JobWriterEpochDualPublisher)
	}
	if err := models.CheckJobWriterEpoch(db); err != nil {
		t.Fatalf("the seeded epoch must pass its own preflight: %v", err)
	}
}
