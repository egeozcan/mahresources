//go:build postgres && json1 && fts5

package application_context

import (
	"testing"

	"mahresources/models"
)

// newPostgresJobStatesContext is one PostgreSQL process with the tables the
// reclassification and the readiness report read.
func newPostgresJobStatesContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(
		&models.Resource{}, &models.ResourceVersion{}, &models.ResourceCategory{},
		&models.Series{}, &models.Tag{}, &models.Group{}, &models.Note{}, &models.NoteType{},
		&models.Category{}, &models.Preview{}, &models.ImageHash{}, &models.GroupRelation{},
		&models.GroupRelationType{}, &models.NoteBlock{}, &models.User{}, &models.LogEntry{},
		&models.PluginKV{}, &models.PluginState{}, &models.RuntimeSetting{},
		&models.DownloadHistoryEntry{}, &models.ScheduledDownload{}, &models.PluginCommandRun{},
		&models.PluginCommandRunOutput{}, &models.PluginCommandImport{}, &models.PluginCommandImportMap{},
		&models.ResourceReduction{}, &models.PluginSchedule{},
		&models.Job{}, &models.JobResourceReceipt{}, &models.JobEvent{}, &models.JobEventSequence{},
		&models.JobLink{}, &models.JobOutput{}, &models.JobReplayEnvelope{}, &models.JobClaim{},
		&models.JobCapacityLease{}, &models.JobPreference{}, &models.JobPinGuard{},
		&models.JobCommandRequest{}, &models.JobLegacyHandle{}, &models.JobRuntimeFence{},
		&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{},
	); err != nil {
		t.Fatalf("migrate PostgreSQL fixture: %v", err)
	}
	if err := models.EnsureJobWriterEpoch(db); err != nil {
		t.Fatalf("seed the writer epoch: %v", err)
	}
	return newPostgresOwnershipContext(t, dsn, sharedReplayKey(t), newPostgresMigrationFilesystem(), 2)
}

func TestAHoldAnEarlierReleaseRecordedAsBlockedIsPausedPG(t *testing.T) {
	assertHoldsAreReclassified(t, newPostgresJobStatesContext(t))
}

func TestReadinessListsUnfinishedJobsThatMayBelongToADeletedAccountPG(t *testing.T) {
	assertReadinessListsReviewCandidates(t, newPostgresJobStatesContext(t))
}
