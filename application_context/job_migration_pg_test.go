//go:build postgres && json1 && fts5

package application_context

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/query_models"

	"github.com/spf13/afero"
)

func TestJobMigrationPlaintextRetirementPostgresResumesAfterCrashMidScrub(t *testing.T) {
	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(
		&models.Resource{}, &models.ResourceVersion{}, &models.ResourceCategory{},
		&models.Series{}, &models.Tag{}, &models.Group{}, &models.Note{}, &models.NoteType{},
		&models.Category{}, &models.Preview{}, &models.ImageHash{}, &models.GroupRelation{},
		&models.GroupRelationType{}, &models.NoteBlock{}, &models.User{}, &models.LogEntry{},
		&models.PluginKV{}, &models.PluginState{}, &models.RuntimeSetting{},
		&models.DownloadHistoryEntry{}, &models.ScheduledDownload{}, &models.PluginCommandRun{},
		&models.PluginCommandRunOutput{}, &models.PluginCommandImport{}, &models.PluginCommandImportMap{},
		&models.ResourceReduction{}, &models.Query{}, &models.SavedMRQLQuery{}, &models.SavedSearch{},
		&models.UserSetting{}, &models.Session{}, &models.ApiToken{}, &models.TemplatePartial{},
		&models.ResourceSimilarity{}, &models.PluginSchedule{},
		&models.Job{}, &models.JobResourceReceipt{}, &models.JobEvent{}, &models.JobEventSequence{},
		&models.JobLink{}, &models.JobOutput{}, &models.JobReplayEnvelope{}, &models.JobClaim{},
		&models.JobCapacityLease{}, &models.JobPreference{}, &models.JobPinGuard{},
		&models.JobCommandRequest{}, &models.JobLegacyHandle{}, &models.JobRuntimeFence{},
	); err != nil {
		t.Fatalf("migrate PostgreSQL fixture: %v", err)
	}

	key := sharedReplayKey(t)
	ctx := newPostgresOwnershipContext(t, dsn, key, newPostgresMigrationFilesystem(), 2)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 2; i++ {
		jobID := fmt.Sprintf("pg-migration-history-%d", i)
		creator := query_models.ResourceFromRemoteCreator{URL: "https://pg-migration.example.invalid/private/" + jobID + "?token=secret"}
		payload, err := json.Marshal(creator)
		if err != nil {
			t.Fatal(err)
		}
		started, finished := now.Add(time.Duration(i)*time.Minute), now.Add(time.Duration(i+1)*time.Minute)
		entry := models.DownloadHistoryEntry{JobID: jobID, URL: creator.URL, Status: models.DownloadHistoryStatusFailed,
			CreatedAt: now.Add(time.Duration(i) * time.Minute), StartedAt: &started, CompletedAt: &finished, Payload: payload}
		if err := ctx.db.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	creator := query_models.ResourceFromRemoteCreator{URL: "https://pg-scheduled.example.invalid/private?token=secret"}
	scheduledPayload, err := json.Marshal(creator)
	if err != nil {
		t.Fatal(err)
	}
	scheduled := models.ScheduledDownload{PluginName: "worker", URL: creator.URL, Payload: scheduledPayload,
		DueAt: now.Add(time.Hour), Status: models.ScheduledDownloadStatusPending, CreatedAt: now, UpdatedAt: now}
	if err := ctx.db.Create(&scheduled).Error; err != nil {
		t.Fatal(err)
	}
	run := models.PluginCommandRun{ID: "pg-migration-command-run", PluginName: "worker", CommandName: "ingest",
		ParamsJSON: `{"token":"secret"}`, InputsJSON: `[]`, Status: models.PluginCommandRunStatusQueued, CreatedAt: now}
	if err := ctx.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	imp := models.PluginCommandImport{ID: "pg-migration-command-import", RunID: run.ID, FileName: "private.csv",
		FieldsJSON: `{"token":"secret"}`, Status: models.PluginCommandImportStatusPending, CreatedAt: now}
	if err := ctx.db.Create(&imp).Error; err != nil {
		t.Fatal(err)
	}

	options := JobMigrationOptions{BatchSize: 1, MaxBatches: 1, WritersDrained: true, Now: func() time.Time { return now.Add(time.Hour) }}
	var checkpoint models.JobMigrationCheckpoint
	for i := 0; i < 100; i++ {
		result, err := ctx.RunJobMigration(options)
		if err != nil {
			t.Fatalf("migration pass %d: %v", i, err)
		}
		if result.Complete {
			t.Fatal("fixture completed before the test interrupted scrub")
		}
		if err := ctx.db.First(&checkpoint, models.JobMigrationCheckpointRowID).Error; err != nil {
			t.Fatal(err)
		}
		if checkpoint.Phase == models.JobMigrationPhaseScrub && checkpoint.SourceKind == jobMigrationDownloadHistory && checkpoint.CursorID != "" {
			break
		}
		if i == 99 {
			t.Fatalf("migration did not reach a mid-scrub checkpoint: %+v", checkpoint)
		}
	}
	var unswept models.DownloadHistoryEntry
	if err := ctx.db.Where("job_id = ?", "pg-migration-history-1").First(&unswept).Error; err != nil {
		t.Fatal(err)
	}
	if len(unswept.Payload) == 0 {
		t.Fatal("the second source was already scrubbed before the simulated crash")
	}
	var unsweptMapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, fmt.Sprint(unswept.ID)).First(&unsweptMapping).Error; err != nil {
		t.Fatal(err)
	}
	if unsweptMapping.Status != models.JobSourceMappingVerified {
		t.Fatalf("unswept source mapping status = %q, want verified", unsweptMapping.Status)
	}
	functionSQL := fmt.Sprintf(`CREATE FUNCTION fail_scrub_marker_for_test() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.source_kind = 'download-history' AND NEW.source_id = '%d' AND NEW.status = 'scrubbed' THEN
				RAISE EXCEPTION 'injected scrub marker failure';
			END IF;
			RETURN NEW;
		END $$;`, unswept.ID)
	if err := ctx.db.Exec(functionSQL).Error; err != nil {
		t.Fatalf("install injected marker function: %v", err)
	}
	if err := ctx.db.Exec("CREATE TRIGGER fail_scrub_marker_for_test BEFORE UPDATE ON job_source_mappings FOR EACH ROW EXECUTE FUNCTION fail_scrub_marker_for_test()").Error; err != nil {
		t.Fatalf("install injected marker failure: %v", err)
	}
	if _, err := ctx.RunJobMigration(options); err == nil {
		t.Fatal("scrub unexpectedly succeeded while the mapping marker write was rejected")
	}
	var afterFault models.DownloadHistoryEntry
	if err := ctx.db.First(&afterFault, unswept.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(afterFault.Payload) == 0 || afterFault.URL != unswept.URL {
		t.Fatalf("failed marker write committed a partial source scrub: payload=%d URL=%q", len(afterFault.Payload), afterFault.URL)
	}
	var afterFaultMapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, fmt.Sprint(unswept.ID)).First(&afterFaultMapping).Error; err != nil {
		t.Fatal(err)
	}
	if afterFaultMapping.Status != models.JobSourceMappingVerified || afterFaultMapping.ScrubbedAt != nil || afterFaultMapping.PostScrubHash != "" {
		t.Fatalf("failed marker write changed mapping state: %+v", afterFaultMapping)
	}
	if err := ctx.db.Exec("DROP TRIGGER fail_scrub_marker_for_test ON job_source_mappings").Error; err != nil {
		t.Fatalf("remove injected marker trigger: %v", err)
	}
	if err := ctx.db.Exec("DROP FUNCTION fail_scrub_marker_for_test()").Error; err != nil {
		t.Fatalf("remove injected marker failure: %v", err)
	}
	// Lifecycle-only updates on unswept rows of every source kind must remain
	// writable after the PostgreSQL fence. The triggers are scoped to writes of
	// legacy replay columns.
	if err := ctx.db.Exec("UPDATE download_history_entries SET status = status WHERE id = ?", unswept.ID).Error; err != nil {
		t.Fatalf("status-only update on an unswept source was rejected: %v", err)
	}
	if err := ctx.db.Exec("UPDATE scheduled_downloads SET status = status WHERE id = ?", scheduled.ID).Error; err != nil {
		t.Fatalf("status-only update on an unswept schedule was rejected: %v", err)
	}
	if err := ctx.db.Exec("UPDATE plugin_command_runs SET status = status WHERE id = ?", run.ID).Error; err != nil {
		t.Fatalf("status-only update on an unswept command run was rejected: %v", err)
	}
	if err := ctx.db.Exec("UPDATE plugin_command_imports SET status = status WHERE id = ?", imp.ID).Error; err != nil {
		t.Fatalf("status-only update on an unswept import was rejected: %v", err)
	}

	// A new controller has only the persisted checkpoint, mapping ledger and
	// shared replay key. It resumes from the next source and finishes the scrub.
	restarted := newPostgresOwnershipContext(t, dsn, key, newPostgresMigrationFilesystem(), 2)
	options.MaxBatches = 2
	result, err := restarted.RunJobMigrationToGate(options)
	if err != nil || !result.Complete {
		t.Fatalf("restarted migration = %+v, %v", result, err)
	}
	var rows []models.DownloadHistoryEntry
	if err := restarted.db.Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if len(row.Payload) != 0 || row.URL != "https://pg-migration.example.invalid" {
			t.Fatalf("row %s was not scrubbed after resume: payload=%d URL=%q", row.JobID, len(row.Payload), row.URL)
		}
	}
	var scrubbedSchedule models.ScheduledDownload
	if err := restarted.db.First(&scrubbedSchedule, scheduled.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(scrubbedSchedule.Payload) != 0 || scrubbedSchedule.URL != "https://pg-scheduled.example.invalid" {
		t.Fatalf("scheduled row was not scrubbed after resume: payload=%d URL=%q", len(scrubbedSchedule.Payload), scrubbedSchedule.URL)
	}
	var scrubbedRun models.PluginCommandRun
	if err := restarted.db.First(&scrubbedRun, "id = ?", run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if scrubbedRun.ParamsJSON != "" || scrubbedRun.InputsJSON != "" || scrubbedRun.JobID == "" {
		t.Fatalf("command run was not scrubbed and linked after resume: %+v", scrubbedRun)
	}
	var scrubbedImport models.PluginCommandImport
	if err := restarted.db.First(&scrubbedImport, "id = ?", imp.ID).Error; err != nil {
		t.Fatal(err)
	}
	if scrubbedImport.FieldsJSON != "" || scrubbedImport.JobID == "" {
		t.Fatalf("import was not scrubbed and linked after resume: %+v", scrubbedImport)
	}
	readiness, err := restarted.GetJobMigrationReadiness()
	if err != nil || !readiness.Ready {
		t.Fatalf("completed PostgreSQL retirement readiness = %+v, %v", readiness, err)
	}
	var scheduledMapping models.JobSourceMapping
	if err := restarted.db.Where("source_kind = ? AND source_id = ?", jobMigrationScheduledDownload, fmt.Sprint(scheduled.ID)).First(&scheduledMapping).Error; err != nil {
		t.Fatal(err)
	}
	if err := restarted.db.Exec("DROP TRIGGER retired_job_source_plaintext_scheduled_downloads ON scheduled_downloads").Error; err != nil {
		t.Fatalf("temporarily remove the retired source barrier: %v", err)
	}
	readiness, err = restarted.GetJobMigrationReadiness()
	if err != nil || readiness.Ready || readiness.Blockers["source-write-barrier-missing"] == 0 {
		t.Fatalf("missing PostgreSQL source-write barrier should fail readiness: %+v, %v", readiness, err)
	}
	if err := restarted.db.Model(&models.ScheduledDownload{}).Where("id = ?", scheduled.ID).Updates(map[string]any{
		"url":     "https://restored-user:secret@pg-scheduled.example.invalid/private?token=restored",
		"payload": scheduledPayload,
	}).Error; err != nil {
		t.Fatalf("simulate a restored pre-retirement source row: %v", err)
	}
	readiness, err = restarted.GetJobMigrationReadiness()
	if err != nil || readiness.Ready || readiness.Blockers["source-retirement-hash-mismatch/scheduled-download"] == 0 {
		t.Fatalf("restored PostgreSQL source should fail readiness: %+v, %v", readiness, err)
	}
	recovered, err := restarted.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 1, MaxBatches: 3, WritersDrained: false, Now: func() time.Time { return now.Add(2 * time.Hour) }})
	if err != nil || !recovered.Complete {
		t.Fatalf("restored PostgreSQL source retirement = %+v, %v", recovered, err)
	}
	var restoredScrubbed models.ScheduledDownload
	if err := restarted.db.First(&restoredScrubbed, scheduled.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(restoredScrubbed.Payload) != 0 || restoredScrubbed.URL != "https://pg-scheduled.example.invalid" {
		t.Fatalf("restored source was not rescrubbed: payload=%d URL=%q", len(restoredScrubbed.Payload), restoredScrubbed.URL)
	}
	readiness, err = restarted.GetJobMigrationReadiness()
	if err != nil || !readiness.Ready {
		t.Fatalf("recovered PostgreSQL retirement readiness = %+v, %v", readiness, err)
	}
	if err := restarted.db.Where("job_id = ?", scheduledMapping.JobID).Delete(&models.JobReplayEnvelope{}).Error; err != nil {
		t.Fatal(err)
	}
	readiness, err = restarted.GetJobMigrationReadiness()
	if err != nil || readiness.Ready || readiness.Blockers["nonterminal-replay-unavailable/scheduled-download"] == 0 {
		t.Fatalf("missing nonterminal PostgreSQL input should block readiness: %+v, %v", readiness, err)
	}
	unsafe := models.DownloadHistoryEntry{JobID: "post-fence-unsafe", URL: "https://user:secret@pg-migration.example.invalid/path?token=private",
		Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now, Payload: []byte(`{"url":"secret"}`)}
	if err := restarted.db.Create(&unsafe).Error; err == nil {
		t.Fatal("PostgreSQL writer barrier accepted a new legacy row with replay plaintext")
	}
}

func newPostgresMigrationFilesystem() afero.Fs {
	return afero.NewMemMapFs()
}
