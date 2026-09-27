//go:build postgres && json1 && fts5

package application_context

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"

	"mahresources/constants"
	"mahresources/models"
)

// PostgreSQL stores the due time as timestamptz, so its comparison is between
// instants already. These run the same scenarios as the SQLite tests, so a
// dialect-specific predicate cannot quietly break the engine it does not change.

func newPostgresScheduledDownloadContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(
		&models.ScheduledDownload{}, &models.User{}, &models.Group{}, &models.Note{},
		&models.RuntimeSetting{}, &models.LogEntry{}, &models.Session{}, &models.ApiToken{}, &models.JobWriterEpoch{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := models.EnsureJobWriterEpoch(db); err != nil {
		t.Fatalf("seed writer epoch: %v", err)
	}
	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open read-only handle: %v", err)
	}
	t.Cleanup(func() { readOnly.Close() })
	return NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{DbType: constants.DbTypePosgres})
}

func TestADeferredDownloadFiresAtItsDueTimeInEveryZonePG(t *testing.T) {
	for _, zone := range deferredDueTimeZones {
		t.Run(zone.String(), func(t *testing.T) {
			assertDeferredDueTimeRowsFireOnTime(t, newPostgresScheduledDownloadContext(t), zone)
		})
	}
}

func TestDeferredDownloadsListInDueOrderInEveryZonePG(t *testing.T) {
	for _, zone := range deferredDueTimeZones {
		t.Run(zone.String(), func(t *testing.T) {
			assertDeferredDueTimeRowsListInDueOrder(t, newPostgresScheduledDownloadContext(t), zone)
		})
	}
}

func TestADeferredDownloadClaimAgesByTheClockInEveryZonePG(t *testing.T) {
	for _, zone := range deferredDueTimeZones {
		t.Run(zone.String(), func(t *testing.T) {
			assertDeferredClaimAgeIsAnInstant(t, newPostgresScheduledDownloadContext(t), zone)
		})
	}
}
