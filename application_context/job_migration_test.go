package application_context

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_commands"
)

//go:embed testdata/job-migration/release-a.sql
var releaseAJobMigrationFixture string

func TestJobMigrationReadinessDoesNotCreateMissingSchema(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.Migrator().DropTable(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Ready || readiness.Blockers["migration-schema-missing"] == 0 {
		t.Fatalf("readiness without migration schema = %+v, want a safe blocked report", readiness)
	}
	for _, model := range []any{&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}} {
		if ctx.db.Migrator().HasTable(model) {
			t.Fatalf("readiness probe recreated missing table %T", model)
		}
	}
}

func TestJobMigrationReadinessBlocksUnknownSourceKindsWithoutEchoingThem(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 10, MaxBatches: 20, WritersDrained: true})
	if err != nil || !result.Complete {
		t.Fatalf("empty migration = %+v, %v", result, err)
	}
	now := time.Now().UTC()
	mapping := models.JobSourceMapping{
		SourceKind: "credential-bearing-future-source", SourceID: "header=private-token",
		SourceRevision: 1, SourceHash: "safe-test-hash", Status: models.JobSourceMappingScrubbed,
		Origin: models.JobSourceOriginBackfilled, CopiedAt: now, ScrubbedAt: &now,
		PostScrubHash: "safe-post-hash", CreatedAt: now, UpdatedAt: now,
	}
	if err := ctx.db.Create(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Ready || readiness.Blockers["unknown-source-kind"] != 1 {
		t.Fatalf("unknown source kind readiness = %+v, want a closed gate", readiness)
	}
	encoded, err := json.Marshal(readiness)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "credential-bearing-future-source") || strings.Contains(string(encoded), "header=private-token") {
		t.Fatalf("readiness leaked unknown mapping fields: %s", encoded)
	}
}

func TestJobMigrationUpgradesReleaseASourceSchema(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.Migrator().DropTable(&models.PluginCommandImport{}, &models.PluginCommandRun{}, &models.DownloadHistoryEntry{}); err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Exec(releaseAJobMigrationFixture).Error; err != nil {
		t.Fatalf("load Release A source fixture: %v", err)
	}
	if err := ctx.db.AutoMigrate(&models.DownloadHistoryEntry{}, &models.PluginCommandRun{}, &models.PluginCommandImport{}); err != nil {
		t.Fatalf("upgrade Release A source tables: %v", err)
	}
	result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 1, MaxBatches: 3, WritersDrained: true})
	if err != nil || !result.Complete {
		var checkpoint models.JobMigrationCheckpoint
		_ = ctx.db.First(&checkpoint, models.JobMigrationCheckpointRowID).Error
		var mappings []models.JobSourceMapping
		_ = ctx.db.Order("source_kind, source_id").Find(&mappings).Error
		t.Fatalf("Release A source migration = %+v, %v (checkpoint=%+v mappings=%+v)", result, err, checkpoint, mappings)
	}

	var download models.DownloadHistoryEntry
	if err := ctx.db.Where("job_id = ?", "release-a-download").First(&download).Error; err != nil {
		t.Fatal(err)
	}
	if len(download.Payload) != 0 || download.URL != "https://old.example" {
		t.Fatalf("Release A download was not safely scrubbed: payload=%d URL=%q", len(download.Payload), download.URL)
	}
	var run models.PluginCommandRun
	if err := ctx.db.First(&run, "id = ?", "release-a-run").Error; err != nil {
		t.Fatal(err)
	}
	if run.JobID == "" || run.ParamsJSON != "" || run.InputsJSON != "" {
		t.Fatalf("Release A command source was not mapped and scrubbed: %+v", run)
	}
	var imp models.PluginCommandImport
	if err := ctx.db.First(&imp, "id = ?", "release-a-import").Error; err != nil {
		t.Fatal(err)
	}
	if imp.JobID == "" || imp.FieldsJSON != "" {
		t.Fatalf("Release A import source was not mapped and scrubbed: %+v", imp)
	}
	var link models.JobLink
	if err := ctx.db.Where("type = ? AND from_job_id = ? AND to_job_id = ?", models.JobLinkParentChild, run.JobID, imp.JobID).First(&link).Error; err != nil {
		t.Fatalf("Release A run/import parent link was not preserved: %v", err)
	}
}

func TestJobMigrationBoundsDownloadTitleAndKeepsFullUTF8FilenameInReplay(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	finished := now.Add(time.Minute)
	fullName := strings.Repeat("界", 300)
	creator := query_models.ResourceFromRemoteCreator{
		FileName: fullName, URL: "https://long-name.example.invalid/archive",
	}
	payload, err := json.Marshal(creator)
	if err != nil {
		t.Fatal(err)
	}
	row := models.DownloadHistoryEntry{
		JobID: "long-utf8-download", URL: creator.URL, Status: models.DownloadHistoryStatusCompleted,
		CreatedAt: now, CompletedAt: &finished, Payload: payload,
	}
	if err := ctx.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.copyDownloadHistory(row, now); err != nil {
		t.Fatalf("copy long filename source: %v", err)
	}

	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, strconv.FormatUint(uint64(row.ID), 10)).First(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if mapping.Status != models.JobSourceMappingCopied || mapping.BlockerCode != "" || mapping.JobID == "" {
		t.Fatalf("long filename mapping = %+v, want a copied Job without a blocker", mapping)
	}
	job, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, mapping.JobID)
	if err != nil {
		t.Fatalf("read imported Job: %v", err)
	}
	wantTitle := strings.Repeat("界", jobs.MaxTitleBytes/len("界"))
	if job.Title != wantTitle || len(job.Title) > jobs.MaxTitleBytes || !utf8.ValidString(job.Title) {
		t.Fatalf("Job title = %q (%d bytes), want a valid UTF-8 prefix of %d bytes", job.Title, len(job.Title), jobs.MaxTitleBytes)
	}
	opened, err := ctx.JobService().OpenReplay(ctx.jobDeps(), jobs.Access{Administrator: true}, mapping.JobID)
	if err != nil {
		t.Fatalf("open imported download replay: %v", err)
	}
	var replay downloadJobInput
	if err := json.Unmarshal(opened.Input, &replay); err != nil {
		t.Fatalf("decode imported replay: %v", err)
	}
	if replay.Creator == nil {
		t.Fatal("imported replay has no download creator")
	}
	if replay.Creator.FileName != fullName {
		t.Fatalf("replay filename was shortened: got %d bytes, want the complete %d-byte filename", len(replay.Creator.FileName), len(fullName))
	}
}

func TestJobMigrationRetriesUnchangedDownloadImportQuarantineFromDrainFence(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	finished := now.Add(time.Minute)
	creator := query_models.ResourceFromRemoteCreator{URL: "https://retry.example.invalid/file.bin", FileName: "file.bin"}
	payload, err := json.Marshal(creator)
	if err != nil {
		t.Fatal(err)
	}
	row := models.DownloadHistoryEntry{
		JobID: "retry-download-history", URL: creator.URL, Status: models.DownloadHistoryStatusCompleted,
		CreatedAt: now, CompletedAt: &finished, Payload: payload,
	}
	if err := ctx.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	sourceID := strconv.FormatUint(uint64(row.ID), 10)
	if err := ctx.db.Create(&models.JobSourceMapping{
		SourceKind: jobMigrationDownloadHistory, SourceID: sourceID, SourceRevision: 1,
		SourceHash: hashDownloadHistory(row), Status: models.JobSourceMappingQuarantined,
		BlockerCode: jobMigrationDownloadCanonicalImportFailed, Origin: models.JobSourceOriginBackfilled,
		CopiedAt: now, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	checkpoint := models.JobMigrationCheckpoint{
		ID: models.JobMigrationCheckpointRowID, Phase: models.JobMigrationPhaseDrainFence,
		LastError: "source download-history/1 is quarantined (canonical-job-import-failed)", UpdatedAt: now,
	}
	if err := ctx.db.Create(&checkpoint).Error; err != nil {
		t.Fatal(err)
	}

	first, err := ctx.RunJobMigration(JobMigrationOptions{
		BatchSize: 1, MaxBatches: 1, WritersDrained: false, Now: func() time.Time { return now.Add(time.Second) },
	})
	if err != nil {
		t.Fatalf("retry quarantined download: %v", err)
	}
	if first.Phase != models.JobMigrationPhaseCopy || first.Batches != 1 {
		t.Fatalf("first recovery pass = %+v, want one copy batch", first)
	}
	var recovered models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, sourceID).First(&recovered).Error; err != nil {
		t.Fatal(err)
	}
	if recovered.Status != models.JobSourceMappingCopied || recovered.BlockerCode != "" || recovered.JobID == "" || recovered.SourceHash != hashDownloadHistory(row) {
		t.Fatalf("recovered mapping = %+v, want the same source hash copied to a Job", recovered)
	}
	var reset models.JobMigrationCheckpoint
	if err := ctx.db.First(&reset, models.JobMigrationCheckpointRowID).Error; err != nil {
		t.Fatal(err)
	}
	if reset.Phase != models.JobMigrationPhaseCopy || reset.SourceKind != jobMigrationDownloadHistory || reset.CursorID != sourceID || reset.LastError != "" {
		t.Fatalf("checkpoint after retry batch = %+v, want copy/download-history after the retried row", reset)
	}

	finishedResult, err := ctx.RunJobMigrationToGate(JobMigrationOptions{
		BatchSize: 10, MaxBatches: 20, WritersDrained: true, Now: func() time.Time { return now.Add(2 * time.Second) },
	})
	if err != nil || !finishedResult.Complete {
		t.Fatalf("migration after quarantine retry = %+v, %v", finishedResult, err)
	}
}

func TestJobMigrationRetriesDownloadQuarantinesAcrossTextIDOrder(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	finished := now.Add(time.Minute)
	for _, id := range []uint{9, 10} {
		creator := query_models.ResourceFromRemoteCreator{URL: fmt.Sprintf("https://retry.example.invalid/%d.bin", id), FileName: fmt.Sprintf("%d.bin", id)}
		payload, err := json.Marshal(creator)
		if err != nil {
			t.Fatal(err)
		}
		row := models.DownloadHistoryEntry{
			ID: id, JobID: fmt.Sprintf("retry-download-%d", id), URL: creator.URL,
			Status: models.DownloadHistoryStatusCompleted, CreatedAt: now, CompletedAt: &finished, Payload: payload,
		}
		if err := ctx.db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		if err := ctx.db.Create(&models.JobSourceMapping{
			SourceKind: jobMigrationDownloadHistory, SourceID: strconv.FormatUint(uint64(id), 10), SourceRevision: 1,
			SourceHash: hashDownloadHistory(row), Status: models.JobSourceMappingQuarantined,
			BlockerCode: jobMigrationDownloadCanonicalImportFailed, Origin: models.JobSourceOriginBackfilled,
			CopiedAt: now, CreatedAt: now, UpdatedAt: now,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := ctx.db.Create(&models.JobMigrationCheckpoint{
		ID: models.JobMigrationCheckpointRowID, Phase: models.JobMigrationPhaseDrainFence, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}

	result, err := ctx.RunJobMigration(JobMigrationOptions{BatchSize: 10, MaxBatches: 1, WritersDrained: false})
	if err != nil || result.Phase != models.JobMigrationPhaseCopy {
		t.Fatalf("retry mixed-digit download IDs: %+v, %v", result, err)
	}
	for _, id := range []uint{9, 10} {
		var mapping models.JobSourceMapping
		if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, strconv.FormatUint(uint64(id), 10)).First(&mapping).Error; err != nil {
			t.Fatal(err)
		}
		if mapping.Status != models.JobSourceMappingCopied || mapping.JobID == "" || mapping.BlockerCode != "" {
			t.Fatalf("download %d was skipped by the retry cursor: %+v", id, mapping)
		}
	}
}

func TestJobMigrationLeavesChangedDownloadImportQuarantineAtDrainFence(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	finished := now.Add(time.Minute)
	creator := query_models.ResourceFromRemoteCreator{URL: "https://changed.example.invalid/file.bin", FileName: "file.bin"}
	payload, err := json.Marshal(creator)
	if err != nil {
		t.Fatal(err)
	}
	row := models.DownloadHistoryEntry{
		JobID: "changed-retry-download-history", URL: creator.URL, Status: models.DownloadHistoryStatusCompleted,
		CreatedAt: now, CompletedAt: &finished, Payload: payload,
	}
	if err := ctx.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	originalHash := hashDownloadHistory(row)
	if err := ctx.db.Model(&models.DownloadHistoryEntry{}).Where("id = ?", row.ID).Update("error", "source changed after quarantine").Error; err != nil {
		t.Fatal(err)
	}
	sourceID := strconv.FormatUint(uint64(row.ID), 10)
	if err := ctx.db.Create(&models.JobSourceMapping{
		SourceKind: jobMigrationDownloadHistory, SourceID: sourceID, SourceRevision: 1,
		SourceHash: originalHash, Status: models.JobSourceMappingQuarantined,
		BlockerCode: jobMigrationDownloadCanonicalImportFailed, Origin: models.JobSourceOriginBackfilled,
		CopiedAt: now, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Create(&models.JobMigrationCheckpoint{
		ID: models.JobMigrationCheckpointRowID, Phase: models.JobMigrationPhaseDrainFence, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}

	result, err := ctx.RunJobMigration(JobMigrationOptions{BatchSize: 1, MaxBatches: 1, WritersDrained: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.Phase != models.JobMigrationPhaseDrainFence || result.BlockedSources != 1 || result.Batches != 0 {
		t.Fatalf("changed source recovery = %+v, want the existing quarantine to stay blocked", result)
	}
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, sourceID).First(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if mapping.Status != models.JobSourceMappingQuarantined || mapping.BlockerCode != jobMigrationDownloadCanonicalImportFailed || mapping.SourceHash != originalHash {
		t.Fatalf("changed source mapping was rearmed: %+v", mapping)
	}
}

func TestJobMigrationReconcilesChangedDownloadSourceAfterCopy(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	creator := &query_models.ResourceFromRemoteCreator{URL: "https://history.example.invalid/item?token=sealed"}
	input, err := remoteDownloadInputJSON(creator, "")
	if err != nil {
		t.Fatal(err)
	}
	const legacyID = "changed-history-source"
	_, err = ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion, State: jobs.StateQueued,
		Origin: "api", Title: "Download from history.example.invalid", Replay: jobs.ReplayInput{Input: input},
		LegacyRefs: []jobs.LegacyRef{{Namespace: DownloadHandleNamespace, Handle: legacyID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	finished := now.Add(time.Minute)
	payload, err := json.Marshal(creator)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.RecordTerminalDownload(download_queue.HistoryRecord{
		JobID: legacyID, URL: creator.URL, Status: models.DownloadHistoryStatusFailed,
		CreatedAt: now, CompletedAt: &finished, Payload: payload,
	}); err != nil {
		t.Fatalf("write dual-published history row: %v", err)
	}
	var source models.DownloadHistoryEntry
	if err := ctx.db.Where("job_id = ?", legacyID).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.copyDownloadHistory(source, now); err != nil {
		t.Fatalf("initial copy: %v", err)
	}
	if err := ctx.db.Model(&models.DownloadHistoryEntry{}).Where("id = ?", source.ID).Update("error", "updated by pre-fence Release A").Error; err != nil {
		t.Fatal(err)
	}
	more, _, err := ctx.verifyDownloadHistoryBatch("", 10, now)
	if err != nil || more {
		t.Fatalf("verification after a source update = more:%t err:%v", more, err)
	}
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, strconv.FormatUint(uint64(source.ID), 10)).First(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	var updated models.DownloadHistoryEntry
	if err := ctx.db.First(&updated, source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if mapping.Status != models.JobSourceMappingVerified || mapping.SourceRevision != 2 || mapping.SourceHash != hashDownloadHistory(updated) {
		t.Fatalf("changed source was not re-proven at a new revision: mapping=%+v", mapping)
	}
}

func TestJobMigrationReconcilesChangedScheduledSourceAfterCopy(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}
	owner, err := ctx.CreateUser(&UserInput{Username: "migration-scheduled", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	row, err := ctx.CreateScheduledDownload("worker", owner.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://scheduled.example.invalid/later?token=sealed"}, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create dual-published scheduled source: %v", err)
	}
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).
		Updates(map[string]any{"last_error": "retry after pre-fence tick", "updated_at": now.Add(time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.First(row, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.copyScheduledDownload(*row, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("copy after a source update: %v", err)
	}
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationScheduledDownload, strconv.FormatUint(uint64(row.ID), 10)).First(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if mapping.Status != models.JobSourceMappingCopied || mapping.SourceRevision != 2 || mapping.SourceHash != hashScheduledDownload(*row) {
		t.Fatalf("changed scheduled source was not re-proven at a new revision: mapping=%+v", mapping)
	}
}

func TestJobMigrationSourceAndMappingWritesAreAtomic(t *testing.T) {
	t.Run("download history", func(t *testing.T) {
		ctx := newJobHarnessContext(t, false)
		if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
			t.Fatal(err)
		}
		if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Truncate(time.Second)
		creator := &query_models.ResourceFromRemoteCreator{URL: "https://atomic.example.invalid/item"}
		input, err := remoteDownloadInputJSON(creator, "")
		if err != nil {
			t.Fatal(err)
		}
		const legacyID = "atomic-history-source"
		if _, err := ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
			Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion, State: jobs.StateQueued,
			Origin: "api", Title: "Atomic history source", Replay: jobs.ReplayInput{Input: input},
			LegacyRefs: []jobs.LegacyRef{{Namespace: DownloadHandleNamespace, Handle: legacyID}},
		}); err != nil {
			t.Fatal(err)
		}
		if err := ctx.db.Exec("CREATE TRIGGER fail_history_mapping BEFORE INSERT ON job_source_mappings BEGIN SELECT RAISE(ABORT, 'injected mapping failure'); END").Error; err != nil {
			t.Fatal(err)
		}
		finished := now.Add(time.Minute)
		payload, err := json.Marshal(creator)
		if err != nil {
			t.Fatal(err)
		}
		if err := ctx.RecordTerminalDownload(download_queue.HistoryRecord{JobID: legacyID, URL: creator.URL,
			Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &finished, Payload: payload}); err == nil {
			t.Fatal("history write unexpectedly succeeded while its source mapping failed")
		}
		var count int64
		if err := ctx.db.Model(&models.DownloadHistoryEntry{}).Where("job_id = ?", legacyID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("history row survived without its mapping: %d", count)
		}
	})

	t.Run("scheduled download", func(t *testing.T) {
		ctx := newJobHarnessContext(t, false)
		if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
			t.Fatal(err)
		}
		if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
			t.Fatal(err)
		}
		if err := ctx.db.Exec("CREATE TRIGGER fail_scheduled_mapping BEFORE INSERT ON job_source_mappings BEGIN SELECT RAISE(ABORT, 'injected mapping failure'); END").Error; err != nil {
			t.Fatal(err)
		}
		owner, err := ctx.CreateUser(&UserInput{Username: "migration-atomic", Password: "password1", Role: models.RoleUser})
		if err != nil {
			t.Fatal(err)
		}
		_, err = ctx.CreateScheduledDownload("worker", owner.ID,
			&query_models.ResourceFromRemoteCreator{URL: "https://atomic.example.invalid/later"}, time.Now().UTC().Add(time.Hour))
		if err == nil {
			t.Fatal("scheduled acceptance unexpectedly succeeded while its source mapping failed")
		}
		for _, model := range []any{&models.ScheduledDownload{}, &models.Job{}, &models.JobLegacyHandle{}, &models.JobReplayEnvelope{}} {
			var count int64
			if err := ctx.db.Model(model).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("scheduled source/Job transaction left %T rows after rollback: %d", model, count)
			}
		}
	})
}

func TestJobMigrationCopiesDownloadHistoryBeforeScrubbingAndIsIdempotent(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}

	created := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	started := created.Add(2 * time.Minute)
	finished := created.Add(4 * time.Minute)
	creator := query_models.ResourceFromRemoteCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "migration asset", Meta: `{"origin":"archive"}`},
		URL:               "https://person:password@example.invalid/file.bin?signature=private#fragment",
		Headers:           map[string]string{"Authorization": "Bearer legacy-secret"},
	}
	payload, err := json.Marshal(creator)
	if err != nil {
		t.Fatal(err)
	}
	owner := uint(17)
	entry := models.DownloadHistoryEntry{
		JobID: "legacy-download-history-1", URL: creator.URL, Name: creator.Name,
		Status: models.DownloadHistoryStatusFailed, Error: "legacy failure", Attempts: 3,
		CreatedAt: created, StartedAt: &started, CompletedAt: &finished,
		CreatedByUserId: &owner, Payload: payload,
	}
	if err := ctx.db.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}

	migrationNow := finished.Add(time.Hour)
	options := JobMigrationOptions{BatchSize: 10, MaxBatches: 20, WritersDrained: true, Now: func() time.Time { return migrationNow }}
	first, err := ctx.RunJobMigrationToGate(options)
	if err != nil {
		t.Fatalf("first migration pass: %v", err)
	}
	if !first.Complete {
		t.Fatalf("migration phase = %q, want complete", first.Phase)
	}
	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil || !readiness.Ready {
		t.Fatalf("completed retirement readiness = %+v, %v", readiness, err)
	}

	jobID, err := ctx.JobService().ResolveLegacyHandle(ctx.jobDeps(), DownloadHandleNamespace, entry.JobID)
	if err != nil {
		t.Fatalf("resolve migrated legacy handle: %v", err)
	}
	job, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		t.Fatalf("read migrated Job: %v", err)
	}
	if job.Kind != JobKindRemoteDownload || job.State != jobs.StateFailed {
		t.Fatalf("migrated Job kind/state = %q/%q, want %q/failed", job.Kind, job.State, JobKindRemoteDownload)
	}
	if job.OwnerUserID == nil || *job.OwnerUserID != owner {
		t.Fatalf("migrated Job owner = %v, want %d", job.OwnerUserID, owner)
	}
	if !job.AcceptedAt.Equal(created) || job.StartedAt == nil || !job.StartedAt.Equal(started) || job.FinishedAt == nil || !job.FinishedAt.Equal(finished) {
		t.Fatalf("migrated Job timestamps = accepted %v, started %v, finished %v", job.AcceptedAt, job.StartedAt, job.FinishedAt)
	}
	opened, err := ctx.JobService().OpenReplay(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		t.Fatalf("open migrated execution input: %v", err)
	}
	var input downloadJobInput
	if err := json.Unmarshal(opened.Input, &input); err != nil {
		t.Fatalf("decode migrated execution input: %v", err)
	}
	if input.Creator == nil || input.Creator.URL != creator.URL || input.Creator.Headers["Authorization"] != creator.Headers["Authorization"] || input.Creator.Meta != creator.Meta {
		t.Fatalf("migrated execution input lost source fields: %+v", input)
	}

	events, err := ctx.JobService().Timeline(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID, 0, 100)
	if err != nil {
		t.Fatalf("read migrated Job events: %v", err)
	}
	var migrationNotes int
	for _, event := range events {
		if event.Type == "migration-note" {
			migrationNotes++
		}
	}
	if migrationNotes != 1 {
		t.Fatalf("migration notes = %d, want one summary for 3 legacy attempts", migrationNotes)
	}

	var scrubbed models.DownloadHistoryEntry
	if err := ctx.db.First(&scrubbed, entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(scrubbed.Payload) != 0 || scrubbed.URL != "https://example.invalid" {
		t.Fatalf("legacy replay fields were not scrubbed safely: payload=%d bytes url=%q", len(scrubbed.Payload), scrubbed.URL)
	}
	stale := scrubbed
	stale.Payload = []byte(`not canonical JSON`)
	stale.URL = "https://attacker:secret@stale.example.invalid/replayed?token=wrong"
	retried, err := ctx.DownloadHistoryPayload(&stale)
	if err != nil {
		t.Fatalf("retired history reader consulted stale legacy fields: %v", err)
	}
	if retried.URL != creator.URL || retried.Headers["Authorization"] != creator.Headers["Authorization"] {
		t.Fatalf("retired history reader did not use canonical replay: %+v", retried)
	}

	var jobsBefore, mappingsBefore int64
	if err := ctx.db.Model(&models.Job{}).Count(&jobsBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Model(&models.JobSourceMapping{}).Count(&mappingsBefore).Error; err != nil {
		t.Fatal(err)
	}
	second, err := ctx.RunJobMigrationToGate(options)
	if err != nil {
		t.Fatalf("idempotent migration pass: %v", err)
	}
	var jobsAfter, mappingsAfter int64
	if err := ctx.db.Model(&models.Job{}).Count(&jobsAfter).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Model(&models.JobSourceMapping{}).Count(&mappingsAfter).Error; err != nil {
		t.Fatal(err)
	}
	if jobsAfter != jobsBefore || mappingsAfter != mappingsBefore || !second.Complete {
		t.Fatalf("rerun changed Jobs/mappings or regressed phase: jobs %d→%d, mappings %d→%d, phase %q", jobsBefore, jobsAfter, mappingsBefore, mappingsAfter, second.Phase)
	}
}

func TestPlaintextRetirementScheduledDownloadReaderUsesCanonicalReplay(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	creator := query_models.ResourceFromRemoteCreator{URL: "https://person:password@example.invalid/deferred?signature=private#fragment",
		Headers: map[string]string{"Authorization": "Bearer scheduled-secret"}}
	payload, err := json.Marshal(creator)
	if err != nil {
		t.Fatal(err)
	}
	row := models.ScheduledDownload{PluginName: "worker", URL: creator.URL, Payload: payload,
		DueAt: created.Add(time.Hour), Status: models.ScheduledDownloadStatusPending, CreatedAt: created, UpdatedAt: created}
	if err := ctx.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 1, MaxBatches: 20, WritersDrained: true, Now: func() time.Time { return created.Add(2 * time.Hour) }})
	if err != nil || !result.Complete {
		t.Fatalf("scheduled migration = %+v, %v", result, err)
	}
	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil || !readiness.Ready {
		t.Fatalf("scheduled retirement readiness = %+v, %v", readiness, err)
	}
	var scrubbed models.ScheduledDownload
	if err := ctx.db.First(&scrubbed, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(scrubbed.Payload) != 0 || scrubbed.URL != "https://example.invalid" {
		t.Fatalf("scheduled replay projection was not retired: payload=%d URL=%q", len(scrubbed.Payload), scrubbed.URL)
	}
	stale := scrubbed
	stale.Payload = []byte(`not canonical JSON`)
	stale.URL = "https://attacker:secret@stale.example.invalid/replayed?token=wrong"
	got, err := ctx.ScheduledDownloadPayload(&stale)
	if err != nil {
		t.Fatalf("retired scheduled reader consulted stale legacy fields: %v", err)
	}
	if got.URL != creator.URL || got.Headers["Authorization"] != creator.Headers["Authorization"] {
		t.Fatalf("retired scheduled reader did not use canonical replay: %+v", got)
	}

	for _, trigger := range []string{"job_barrier_scheduled_download_insert", "job_barrier_scheduled_download_update"} {
		if err := ctx.db.Exec("DROP TRIGGER IF EXISTS " + trigger).Error; err != nil {
			t.Fatal(err)
		}
	}
	readiness, err = ctx.GetJobMigrationReadiness()
	if err != nil || readiness.Ready || readiness.Blockers["source-write-barrier-missing"] == 0 {
		t.Fatalf("missing SQLite source-write barrier should fail readiness: %+v, %v", readiness, err)
	}
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).
		Updates(map[string]any{"url": row.URL, "payload": row.Payload}).Error; err != nil {
		t.Fatalf("restore pre-retirement scheduled source: %v", err)
	}
	readiness, err = ctx.GetJobMigrationReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Ready || readiness.Blockers["source-retirement-hash-mismatch/"+jobMigrationScheduledDownload] == 0 {
		t.Fatalf("restored source was trusted from the completion marker: %+v", readiness)
	}
	recovered, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 1, MaxBatches: 20, WritersDrained: false, Now: func() time.Time { return created.Add(3 * time.Hour) }})
	if err != nil || !recovered.Complete {
		t.Fatalf("restored source retirement recovery = %+v, %v", recovered, err)
	}
	readiness, err = ctx.GetJobMigrationReadiness()
	if err != nil || !readiness.Ready {
		t.Fatalf("recovered retirement readiness = %+v, %v", readiness, err)
	}
	var recoveredRow models.ScheduledDownload
	if err := ctx.db.First(&recoveredRow, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(recoveredRow.Payload) != 0 || recoveredRow.URL != "https://example.invalid" {
		t.Fatalf("restored scheduled source survived re-scrub: payload=%d URL=%q", len(recoveredRow.Payload), recoveredRow.URL)
	}
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationScheduledDownload, strconv.FormatUint(uint64(row.ID), 10)).First(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Where("job_id = ?", mapping.JobID).Delete(&models.JobReplayEnvelope{}).Error; err != nil {
		t.Fatal(err)
	}
	readiness, err = ctx.GetJobMigrationReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Ready || readiness.Blockers["nonterminal-replay-unavailable/"+jobMigrationScheduledDownload] == 0 {
		t.Fatalf("nonterminal scheduled input loss did not block readiness: %+v", readiness)
	}
}

func TestJobMigrationStartupDriverContinuesAcrossBoundedPasses(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}

	created := time.Date(2025, 5, 6, 7, 8, 9, 0, time.UTC)
	for i := 0; i < 24; i++ {
		jobID := "bounded-history-" + strconv.Itoa(i)
		creator := query_models.ResourceFromRemoteCreator{URL: "https://bounded.example.invalid/" + jobID}
		payload, err := json.Marshal(creator)
		if err != nil {
			t.Fatal(err)
		}
		started, finished := created.Add(time.Duration(i)*time.Minute), created.Add(time.Duration(i+1)*time.Minute)
		entry := models.DownloadHistoryEntry{
			JobID: jobID, URL: creator.URL, Status: models.DownloadHistoryStatusFailed,
			CreatedAt: created.Add(time.Duration(i) * time.Minute), StartedAt: &started, CompletedAt: &finished,
			Payload: payload,
		}
		if err := ctx.db.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}

	result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 2, MaxBatches: 2, WritersDrained: true})
	if err != nil {
		t.Fatalf("bounded startup migration: %v", err)
	}
	if !result.Complete || result.Phase != models.JobMigrationPhaseComplete || result.Batches <= 2 {
		t.Fatalf("startup migration = %+v, want complete after multiple bounded passes", result)
	}
	var mappings int64
	if err := ctx.db.Model(&models.JobSourceMapping{}).Where("source_kind = ?", jobMigrationDownloadHistory).Count(&mappings).Error; err != nil {
		t.Fatal(err)
	}
	if mappings != 24 {
		t.Fatalf("migrated source mappings = %d, want 24", mappings)
	}
}

func TestJobMigrationImportsLegacyCommandRunAndImportWithProvenParent(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	created := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	started, runFinished, importFinished := created.Add(time.Minute), created.Add(2*time.Minute), created.Add(3*time.Minute)
	owner := uint(27)
	run := models.PluginCommandRun{
		ID: "legacy-command-run", PluginName: "worker", CommandName: "ingest", ParamsJSON: `{"mode":"private"}`,
		InputsJSON: `[{"name":"private.csv","bytes":19}]`, Status: plugin_commands.RunStatusSucceeded,
		CreatedByUserId: &owner, CreatedAt: created, StartedAt: &started, FinishedAt: &runFinished,
	}
	imp := models.PluginCommandImport{
		ID: "legacy-command-import", RunID: run.ID, FileName: "private.csv", FieldsJSON: `{"title":"secret"}`,
		PluginGeneration: 8, CreatedByUserId: &owner, Status: plugin_commands.ImportStatusSucceeded,
		CreatedAt: runFinished, StartedAt: &runFinished, FinishedAt: &importFinished,
	}
	if err := ctx.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Create(&imp).Error; err != nil {
		t.Fatal(err)
	}

	migrationNow := importFinished.Add(time.Hour)
	options := JobMigrationOptions{BatchSize: 1, MaxBatches: 3, WritersDrained: true, Now: func() time.Time { return migrationNow }}
	result, err := ctx.RunJobMigrationToGate(options)
	if err != nil || !result.Complete {
		var checkpoint models.JobMigrationCheckpoint
		_ = ctx.db.First(&checkpoint, models.JobMigrationCheckpointRowID).Error
		var mapping models.JobSourceMapping
		_ = ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandRun, run.ID).First(&mapping).Error
		verifyErr := ctx.verifyPluginCommandRunReplay(ctx.db, mapping.JobID, run)
		t.Fatalf("command migration = %+v, %v, checkpoint=%+v mapping=%+v verify=%v", result, err, checkpoint, mapping, verifyErr)
	}
	var migratedRun models.PluginCommandRun
	var migratedImport models.PluginCommandImport
	if err := ctx.db.First(&migratedRun, "id = ?", run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.First(&migratedImport, "id = ?", imp.ID).Error; err != nil {
		t.Fatal(err)
	}
	if migratedRun.JobID == "" || migratedImport.JobID == "" || migratedRun.ParamsJSON != "" || migratedImport.FieldsJSON != "" {
		t.Fatalf("command source rows were not mapped and scrubbed: run=%+v import=%+v", migratedRun, migratedImport)
	}
	var parentChild models.JobLink
	if err := ctx.db.Where("type = ? AND from_job_id = ? AND to_job_id = ?", models.JobLinkParentChild, migratedRun.JobID, migratedImport.JobID).First(&parentChild).Error; err != nil {
		t.Fatalf("legacy import parent link was not recovered from the run mapping: %v", err)
	}
	var runMapping, importMapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandRun, run.ID).First(&runMapping).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandImport, imp.ID).First(&importMapping).Error; err != nil {
		t.Fatal(err)
	}
	if runMapping.Status != models.JobSourceMappingScrubbed || importMapping.Status != models.JobSourceMappingScrubbed {
		t.Fatalf("command mappings not scrubbed: run=%q import=%q", runMapping.Status, importMapping.Status)
	}

	// A restored copy can have the source rows already scrubbed while the phase
	// checkpoint still points to copy. The durable scrub markers must win before
	// plaintext validation so rerunning cannot quarantine valid dual-published data.
	var checkpoint models.JobMigrationCheckpoint
	if err := ctx.db.First(&checkpoint, models.JobMigrationCheckpointRowID).Error; err != nil {
		t.Fatal(err)
	}
	checkpoint.Phase, checkpoint.SourceKind, checkpoint.CursorID = models.JobMigrationPhaseCopy, jobMigrationPluginCommandRun, ""
	if err := ctx.db.Save(&checkpoint).Error; err != nil {
		t.Fatal(err)
	}
	result, err = ctx.RunJobMigrationToGate(options)
	if err != nil || !result.Complete {
		t.Fatalf("post-scrub command rerun = %+v, %v", result, err)
	}
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandRun, run.ID).First(&runMapping).Error; err != nil {
		t.Fatal(err)
	}
	if runMapping.Status != models.JobSourceMappingScrubbed {
		t.Fatalf("post-scrub rerun changed mapping to %q", runMapping.Status)
	}
}

func TestJobMigrationDoesNotInventACommandImportParent(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	owner := uint(15)
	imp := models.PluginCommandImport{ID: "orphan-command-import", RunID: "missing-command-run", FileName: "orphan.csv",
		FieldsJSON: `{"title":"orphan"}`, CreatedByUserId: &owner, Status: plugin_commands.ImportStatusPending,
		CreatedAt: time.Now().UTC().Add(-time.Minute)}
	if err := ctx.db.Create(&imp).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 1, MaxBatches: 2, WritersDrained: true})
	if err != nil {
		t.Fatalf("orphan import migration: %v", err)
	}
	if result.Complete || result.Phase != models.JobMigrationPhaseDrainFence || result.BlockedSources != 1 {
		t.Fatalf("orphan import migration = %+v, want one quarantine blocker", result)
	}
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandImport, imp.ID).First(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if mapping.Status != models.JobSourceMappingQuarantined || mapping.BlockerCode != "parent-run-missing" || mapping.JobID != "" {
		t.Fatalf("orphan import mapping = %+v", mapping)
	}
	var jobs int64
	if err := ctx.db.Model(&models.Job{}).Count(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("orphan import fabricated %d canonical Jobs", jobs)
	}
}

func TestJobMigrationRecognizesAlreadyDualPublishedCommandRun(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.PluginCommandRunOutput{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	owner := uint(19)
	record := plugin_commands.RunRecord{ID: "dual-published-command", PluginName: "worker", CommandName: "ingest",
		ParamsJSON: `{"page":2}`, Status: plugin_commands.RunStatusQueued, CreatedByUserID: &owner, CreatedAt: created}
	if err := ctx.CreateRun(record, plugin_commands.RunOutput{RunID: record.ID, ArgvJSON: `[]`, CreatedAt: created}); err != nil {
		t.Fatalf("publish canonical command run: %v", err)
	}
	var source models.PluginCommandRun
	if err := ctx.db.First(&source, "id = ?", record.ID).Error; err != nil {
		t.Fatal(err)
	}
	canonicalJobID := source.JobID
	var before int64
	if err := ctx.db.Model(&models.Job{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 1, MaxBatches: 3, WritersDrained: true})
	if err != nil || !result.Complete {
		t.Fatalf("dual-published migration = %+v, %v", result, err)
	}
	var after int64
	if err := ctx.db.Model(&models.Job{}).Count(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("dual-published source duplicated canonical Jobs: %d→%d", before, after)
	}
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandRun, record.ID).First(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if mapping.JobID != canonicalJobID || mapping.Origin != models.JobSourceOriginDualPublished || mapping.Status != models.JobSourceMappingScrubbed {
		t.Fatalf("dual-published mapping = %+v", mapping)
	}
}

func TestJobMigrationPurgeMarkerWinsOverLegacyDownloadInput(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	created := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	started, finished := created.Add(time.Minute), created.Add(2*time.Minute)
	handle := "forgotten-legacy-download"
	creator := query_models.ResourceFromRemoteCreator{URL: "https://forget.example.invalid/private?token=legacy-secret",
		Headers: map[string]string{"Authorization": "Bearer legacy-secret"}}
	input, err := remoteDownloadInputJSON(&creator, "")
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := ctx.JobService().ImportLegacy(ctx.jobDeps(), jobs.LegacyImport{
		Acceptance: jobs.Acceptance{Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion, State: jobs.StateQueued,
			Origin: "api", Title: "legacy download", Replay: jobs.ReplayInput{Input: input},
			LegacyRefs: []jobs.LegacyRef{{Namespace: DownloadHandleNamespace, Handle: handle}}},
		State: jobs.StateSucceeded, AcceptedAt: created, StartedAt: &started, FinishedAt: &finished,
	})
	if err != nil {
		t.Fatalf("seed canonical terminal download: %v", err)
	}
	if _, err := ctx.JobService().ForgetReplay(ctx.jobDeps(), jobs.Access{Administrator: true}, canonical.ID); err != nil {
		t.Fatalf("forget canonical replay input: %v", err)
	}
	payload, err := json.Marshal(creator)
	if err != nil {
		t.Fatal(err)
	}
	entry := models.DownloadHistoryEntry{JobID: handle, URL: creator.URL, Name: "legacy download",
		Status: models.DownloadHistoryStatusCompleted, CreatedAt: created, StartedAt: &started, CompletedAt: &finished, Payload: payload}
	if err := ctx.db.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 1, MaxBatches: 3, WritersDrained: true})
	if err != nil || !result.Complete {
		t.Fatalf("purge-marker migration = %+v, %v", result, err)
	}
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, strconv.FormatUint(uint64(entry.ID), 10)).First(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if mapping.JobID != canonical.ID || mapping.Status != models.JobSourceMappingPurged || mapping.PurgeReason != models.JobReplayPurgeForgotten || mapping.PurgedAt == nil || mapping.ScrubbedAt == nil {
		t.Fatalf("forgotten replay marker was not preserved through scrub: %+v", mapping)
	}
	var envelope models.JobReplayEnvelope
	if err := ctx.db.Where("job_id = ?", canonical.ID).First(&envelope).Error; err != nil {
		t.Fatal(err)
	}
	if envelope.PurgedAt == nil || envelope.PurgeReason != models.JobReplayPurgeForgotten || len(envelope.Nonce) != 0 || len(envelope.Ciphertext) != 0 {
		t.Fatalf("migration recreated or changed forgotten replay: %+v", envelope)
	}
	var scrubbed models.DownloadHistoryEntry
	if err := ctx.db.First(&scrubbed, entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(scrubbed.Payload) != 0 || scrubbed.URL != "https://forget.example.invalid" {
		t.Fatalf("legacy replay data survived scrub: payload=%d URL=%q", len(scrubbed.Payload), scrubbed.URL)
	}
}

func TestJobMigrationImportsProvableReductionOutcomeOnly(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	created := time.Now().UTC().Add(-7 * 24 * time.Hour).Truncate(time.Second)
	accepted := created.Add(6 * 24 * time.Hour)
	started := accepted.Add(time.Minute)
	computed := accepted.Add(20 * time.Minute)
	owner := uint(44)
	ready := models.ResourceReduction{
		Name: "historic reduction", Status: models.ReductionStatusReady, MatchingMode: models.MatchingModeIdenticalOnly,
		CreatedAt: created, UpdatedAt: computed, CreatedByUserId: &owner, ComputeJobID: "historic-compute-id", ComputedAt: &computed,
	}
	if err := ctx.db.Create(&ready).Error; err != nil {
		t.Fatal(err)
	}
	acceptance := jobs.Acceptance{Kind: JobKindReductionCompute, KindVersion: jobReductionKindVersion,
		State: jobs.StateSucceeded, ActorUserID: &owner, Origin: "api", Title: "Cluster a Resource Reduction",
		Replay:     jobs.ReplayInput{NonReplayable: true},
		LegacyRefs: []jobs.LegacyRef{{Namespace: ReductionComputeHandleNamespace, Handle: ready.ComputeJobID}}}
	if _, err := ctx.JobService().ImportLegacy(ctx.jobDeps(), jobs.LegacyImport{Acceptance: acceptance,
		State: jobs.StateSucceeded, AcceptedAt: accepted, StartedAt: &started, FinishedAt: &computed}); err != nil {
		t.Fatalf("seed the canonical Reduction execution proof: %v", err)
	}
	result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 1, MaxBatches: 3, WritersDrained: true})
	if err != nil || !result.Complete {
		t.Fatalf("Reduction migration = %+v, %v", result, err)
	}
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationReduction, strconv.FormatUint(uint64(ready.ID), 10)).First(&mapping).Error; err != nil {
		t.Fatalf("provable Reduction was not mapped: %v", err)
	}
	if mapping.Status != models.JobSourceMappingScrubbed || mapping.JobID == "" {
		t.Fatalf("Reduction mapping = %+v", mapping)
	}
	var job models.Job
	if err := ctx.db.Where("id = ?", mapping.JobID).First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if !job.AcceptedAt.Equal(accepted) || job.AcceptedAt.Equal(created) {
		t.Fatalf("Reduction Job acceptance time = %v, want canonical proof %v rather than domain creation %v", job.AcceptedAt, accepted, created)
	}
	if err := ctx.recordDualPublishedReduction(ready, computed.Add(time.Hour)); err != nil {
		t.Fatalf("record post-fence Reduction publication: %v", err)
	}
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationReduction, strconv.FormatUint(uint64(ready.ID), 10)).First(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if mapping.Status != models.JobSourceMappingScrubbed || mapping.PostScrubHash != hashReductionExecution(ready) {
		t.Fatalf("post-fence Reduction write regressed scrub marker: %+v", mapping)
	}
	if err := ctx.db.Migrator().DropTable(&models.JobWriterEpoch{}); err != nil {
		t.Fatalf("drop writer epoch to inject query failure: %v", err)
	}
	if err := ctx.recordDualPublishedReduction(ready, computed.Add(2*time.Hour)); err == nil {
		t.Fatal("dual-published Reduction write succeeded when the writer epoch was unavailable")
	}

	// A ready historical Reduction without a canonical Job handle has a proven
	// result but no proven acceptance time. It remains ordinary domain data and
	// must not receive a synthetic Job using the domain row's older CreatedAt.
	blocked := newJobHarnessContext(t, false)
	createdEarlier := created.Add(-time.Hour)
	orphan := models.ResourceReduction{Name: "unproven acceptance", Status: models.ReductionStatusReady,
		MatchingMode: models.MatchingModeIdenticalOnly, CreatedAt: createdEarlier, UpdatedAt: computed,
		ComputeJobID: "missing-canonical-compute", ComputedAt: &computed}
	if err := blocked.db.Create(&orphan).Error; err != nil {
		t.Fatal(err)
	}
	var jobsBefore int64
	if err := blocked.db.Model(&models.Job{}).Count(&jobsBefore).Error; err != nil {
		t.Fatal(err)
	}
	blockedResult, err := blocked.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 1, MaxBatches: 3, WritersDrained: true})
	if err != nil {
		t.Fatalf("unproven Reduction migration: %v", err)
	}
	if !blockedResult.Complete {
		t.Fatalf("unproven Reduction prevented ordinary startup readiness: %+v", blockedResult)
	}
	var orphanMappings int64
	if err := blocked.db.Model(&models.JobSourceMapping{}).Where("source_kind = ? AND source_id = ?", jobMigrationReduction, strconv.FormatUint(uint64(orphan.ID), 10)).Count(&orphanMappings).Error; err != nil {
		t.Fatal(err)
	}
	var jobsAfter int64
	if err := blocked.db.Model(&models.Job{}).Count(&jobsAfter).Error; err != nil {
		t.Fatal(err)
	}
	if orphanMappings != 0 || jobsAfter != jobsBefore {
		t.Fatalf("unproven Reduction created migration state: mappings=%d jobs=%d→%d", orphanMappings, jobsBefore, jobsAfter)
	}
}

func TestJobMigrationWaitsForExplicitWriterDrainBeforeScrub(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2025, 4, 5, 6, 7, 8, 0, time.UTC)
	finished := created.Add(time.Minute)
	creator := query_models.ResourceFromRemoteCreator{URL: "https://drain.example.invalid/private?token=keep-me"}
	payload, err := json.Marshal(creator)
	if err != nil {
		t.Fatal(err)
	}
	entry := models.DownloadHistoryEntry{JobID: "drain-source", URL: creator.URL, Status: models.DownloadHistoryStatusCancelled,
		CreatedAt: created, CompletedAt: &finished, Payload: payload}
	if err := ctx.db.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}

	result, err := ctx.RunJobMigration(JobMigrationOptions{BatchSize: 1, MaxBatches: 20, WritersDrained: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || result.Phase != models.JobMigrationPhaseDrainFence {
		t.Fatalf("unattested phase = %q complete=%v, want drain-fence/incomplete", result.Phase, result.Complete)
	}
	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Ready || readiness.WriterEpoch != models.JobWriterEpochDualPublisher || readiness.Blockers["writer-epoch-not-retired"] == 0 {
		t.Fatalf("readiness passed before the writer fence: %+v", readiness)
	}
	var epoch models.JobWriterEpoch
	if err := ctx.db.First(&epoch, models.JobWriterEpochRowID).Error; err != nil {
		t.Fatal(err)
	}
	if epoch.MinimumEpoch != models.JobWriterEpochDualPublisher {
		t.Fatalf("unattested migration advanced epoch to %d", epoch.MinimumEpoch)
	}
	var before models.DownloadHistoryEntry
	if err := ctx.db.First(&before, entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(before.Payload) == 0 || before.URL != creator.URL {
		t.Fatalf("source was scrubbed before drain attestation: payload=%d URL=%q", len(before.Payload), before.URL)
	}

	result, err = ctx.RunJobMigration(JobMigrationOptions{BatchSize: 1, MaxBatches: 20, WritersDrained: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Phase != models.JobMigrationPhaseComplete {
		t.Fatalf("attested phase = %q complete=%v, want complete", result.Phase, result.Complete)
	}
	if err := ctx.db.First(&epoch, models.JobWriterEpochRowID).Error; err != nil {
		t.Fatal(err)
	}
	if epoch.MinimumEpoch != models.JobWriterEpochRetiredPlaintext {
		t.Fatalf("drained migration epoch = %d, want %d", epoch.MinimumEpoch, models.JobWriterEpochRetiredPlaintext)
	}
	readiness, err = ctx.GetJobMigrationReadiness()
	if err != nil || !readiness.Ready {
		t.Fatalf("readiness after writer fence = %+v, %v", readiness, err)
	}
}

func TestJobMigrationQuarantinesMalformedSourceWithoutPersistingSecrets(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}
	secret := "https://person:secret@blocked.example.invalid/file?token=secret-token"
	entry := models.DownloadHistoryEntry{JobID: "malformed-source", URL: secret, Status: models.DownloadHistoryStatusFailed,
		CreatedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC), Payload: []byte(`{"broken": "` + secret + `"`)}
	if err := ctx.db.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	result, err := ctx.RunJobMigration(JobMigrationOptions{BatchSize: 1, MaxBatches: 20, WritersDrained: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || result.BlockedSources != 1 {
		t.Fatalf("malformed source result = %+v, want one blocker and no completion", result)
	}
	var checkpoint models.JobMigrationCheckpoint
	if err := ctx.db.First(&checkpoint, models.JobMigrationCheckpointRowID).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(checkpoint.LastError, "secret") || strings.Contains(checkpoint.LastError, "blocked.example") {
		t.Fatalf("migration checkpoint leaked replay fields: %q", checkpoint.LastError)
	}
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, strconv.FormatUint(uint64(entry.ID), 10)).First(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if mapping.Status != models.JobSourceMappingQuarantined || mapping.BlockerCode != "payload-unreadable" || mapping.JobID != "" {
		t.Fatalf("source mapping = status %q blocker %q job %q", mapping.Status, mapping.BlockerCode, mapping.JobID)
	}
	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil {
		t.Fatal(err)
	}
	reportJSON, err := json.Marshal(readiness)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(reportJSON), "secret") || strings.Contains(string(reportJSON), "blocked.example.invalid") {
		t.Fatalf("readiness report leaked source input: %s", reportJSON)
	}
	var epoch models.JobWriterEpoch
	if err := ctx.db.First(&epoch, models.JobWriterEpochRowID).Error; err != nil {
		t.Fatal(err)
	}
	if epoch.MinimumEpoch != models.JobWriterEpochDualPublisher {
		t.Fatalf("quarantined migration advanced epoch to %d", epoch.MinimumEpoch)
	}
}

func TestJobMigrationRestartsFromEveryDurablePhaseOnSQLite(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	key := sharedReplayKey(t)
	holdJobReplayKey(t, ctx, key)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 2; i++ {
		jobID := "restart-history-" + strconv.Itoa(i)
		creator := query_models.ResourceFromRemoteCreator{URL: "https://restart.example.invalid/private/" + jobID + "?token=sealed"}
		payload, err := json.Marshal(creator)
		if err != nil {
			t.Fatal(err)
		}
		finished := now.Add(time.Duration(i+1) * time.Minute)
		row := models.DownloadHistoryEntry{JobID: jobID, URL: creator.URL, Status: models.DownloadHistoryStatusFailed,
			CreatedAt: now.Add(time.Duration(i) * time.Minute), CompletedAt: &finished, Payload: payload}
		if err := ctx.db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	options := JobMigrationOptions{BatchSize: 1, MaxBatches: 1, WritersDrained: false, Now: func() time.Time { return now.Add(time.Hour) }}
	readCheckpoint := func(current *MahresourcesContext) models.JobMigrationCheckpoint {
		t.Helper()
		var checkpoint models.JobMigrationCheckpoint
		if err := current.db.First(&checkpoint, models.JobMigrationCheckpointRowID).Error; err != nil {
			t.Fatal(err)
		}
		return checkpoint
	}
	driveTo := func(current *MahresourcesContext, description string, reached func(models.JobMigrationCheckpoint) bool) models.JobMigrationCheckpoint {
		t.Helper()
		for pass := 0; pass < 200; pass++ {
			if checkpoint := readCheckpoint(current); reached(checkpoint) {
				return checkpoint
			}
			if _, err := current.RunJobMigration(options); err != nil {
				var mappings []models.JobSourceMapping
				_ = current.db.Order("source_kind, source_id").Find(&mappings).Error
				t.Fatalf("migration while reaching %s: %v (checkpoint=%+v mappings=%+v)", description, err, readCheckpoint(current), mappings)
			}
		}
		t.Fatalf("migration did not reach %s; checkpoint=%+v", description, readCheckpoint(current))
		return models.JobMigrationCheckpoint{}
	}
	restart := func(current *MahresourcesContext) *MahresourcesContext {
		t.Helper()
		// Closing the SQLite handle models the process that owned this context dying;
		// the next controller reopens the same file and sees only durable rows.
		sqlDB, err := current.db.DB()
		if err != nil {
			t.Fatal(err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatalf("close migration process database: %v", err)
		}
		other, _ := newSecondProcessJobContext(t, current, key)
		return other
	}
	assertCheckpoint := func(current *MahresourcesContext, want models.JobMigrationCheckpoint) {
		t.Helper()
		got := readCheckpoint(current)
		if got.Phase != want.Phase || got.SourceKind != want.SourceKind || got.CursorID != want.CursorID {
			t.Fatalf("restarted checkpoint = {phase:%q kind:%q cursor:%q}, want {phase:%q kind:%q cursor:%q}",
				got.Phase, got.SourceKind, got.CursorID, want.Phase, want.SourceKind, want.CursorID)
		}
	}
	assertRow := func(current *MahresourcesContext, id uint, wantScrubbed bool) {
		t.Helper()
		var row models.DownloadHistoryEntry
		if err := current.db.First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		scrubbed := len(row.Payload) == 0 && row.URL == "https://restart.example.invalid"
		if scrubbed != wantScrubbed {
			t.Fatalf("source row %d scrubbed=%v, want %v (payload=%d URL=%q)", id, scrubbed, wantScrubbed, len(row.Payload), row.URL)
		}
	}

	var sourceRows []models.DownloadHistoryEntry
	if err := ctx.db.Order("id ASC").Find(&sourceRows).Error; err != nil {
		t.Fatal(err)
	}
	if len(sourceRows) != 2 {
		t.Fatalf("source fixture rows = %d, want 2", len(sourceRows))
	}
	if _, err := ctx.RunJobMigration(options); err != nil {
		t.Fatalf("initial copy batch: %v", err)
	}

	// Copy commits each mapping before the cursor is persisted. Restarting from the
	// first durable cursor must preserve the first mapping and copy the next row.
	copyCheckpoint := driveTo(ctx, "a mid-copy cursor", func(c models.JobMigrationCheckpoint) bool {
		return c.Phase == models.JobMigrationPhaseCopy && c.SourceKind == jobMigrationDownloadHistory && c.CursorID != ""
	})
	ctx = restart(ctx)
	assertCheckpoint(ctx, copyCheckpoint)
	var copied models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, strconv.FormatUint(uint64(sourceRows[0].ID), 10)).First(&copied).Error; err != nil {
		t.Fatalf("first copy batch was not durable across restart: %v", err)
	}
	if err := ctx.verifyDownloadReplay(ctx.db, copied.JobID, sourceRows[0]); err != nil {
		t.Fatalf("copied replay could not be opened by the restarted process: %v", err)
	}
	verifyCheckpoint := driveTo(ctx, "a mid-verify cursor", func(c models.JobMigrationCheckpoint) bool {
		return c.Phase == models.JobMigrationPhaseVerify && c.SourceKind == jobMigrationDownloadHistory && c.CursorID != ""
	})
	ctx = restart(ctx)
	assertCheckpoint(ctx, verifyCheckpoint)
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, verifyCheckpoint.CursorID).First(&copied).Error; err != nil {
		t.Fatalf("verified batch marker was not durable across restart: %v", err)
	}
	if copied.Status != models.JobSourceMappingVerified {
		t.Fatalf("restarted verification marker status = %q, want verified", copied.Status)
	}

	// The drain-fence phase is a durable stop while older writers are still
	// attested as live. A fresh process can honor the gate and later resume with
	// the operator's drain attestation.
	fenceCheckpoint := driveTo(ctx, "the drain-fence phase", func(c models.JobMigrationCheckpoint) bool {
		return c.Phase == models.JobMigrationPhaseDrainFence
	})
	ctx = restart(ctx)
	assertCheckpoint(ctx, fenceCheckpoint)
	fenced, err := ctx.RunJobMigration(options)
	if err != nil || fenced.Phase != models.JobMigrationPhaseDrainFence || fenced.Complete {
		t.Fatalf("unattested restart at fence = %+v, %v", fenced, err)
	}
	assertRow(ctx, sourceRows[0].ID, false)
	options.WritersDrained = true
	if err := ctx.db.Exec(`CREATE TRIGGER fail_scrub_phase_checkpoint BEFORE UPDATE ON job_migration_checkpoints
		WHEN NEW.phase = 'scrub' BEGIN SELECT RAISE(ABORT, 'injected fence checkpoint failure'); END`).Error; err != nil {
		t.Fatalf("install fence transition fault: %v", err)
	}
	var transitionErr error
	for pass := 0; pass < 100; pass++ {
		_, transitionErr = ctx.RunJobMigration(options)
		if transitionErr != nil {
			break
		}
	}
	if transitionErr == nil {
		t.Fatal("fence transition unexpectedly persisted its scrub checkpoint through the injected failure")
	}
	if got := readCheckpoint(ctx); got.Phase != models.JobMigrationPhaseDrainFence {
		t.Fatalf("checkpoint after interrupted fence transition = %+v, want drain-fence", got)
	}
	epoch, err := models.JobWriterEpochMinimum(ctx.db)
	if err != nil || epoch != models.JobWriterEpochRetiredPlaintext {
		t.Fatalf("writer epoch after interrupted fence transition = %d, %v; want retired", epoch, err)
	}
	if err := ctx.db.Exec("DROP TRIGGER fail_scrub_phase_checkpoint").Error; err != nil {
		t.Fatalf("remove fence transition fault: %v", err)
	}
	fenceCheckpoint = readCheckpoint(ctx)
	ctx = restart(ctx)
	assertCheckpoint(ctx, fenceCheckpoint)
	// The completed writer barrier is durable even though the phase checkpoint was
	// not. Recovery can resume from the old fence checkpoint without another drain
	// attestation because the database epoch has already advanced.
	options.WritersDrained = false
	scrubCheckpoint := driveTo(ctx, "a mid-scrub cursor", func(c models.JobMigrationCheckpoint) bool {
		return c.Phase == models.JobMigrationPhaseScrub && c.SourceKind == jobMigrationDownloadHistory && c.CursorID != ""
	})
	assertRow(ctx, sourceRows[0].ID, true)
	assertRow(ctx, sourceRows[1].ID, false)
	ctx = restart(ctx)
	assertCheckpoint(ctx, scrubCheckpoint)

	completeCheckpoint := driveTo(ctx, "the persisted complete phase", func(c models.JobMigrationCheckpoint) bool {
		return c.Phase == models.JobMigrationPhaseComplete
	})
	if completeCheckpoint.CompletedAt == nil {
		t.Fatalf("complete checkpoint has no completion time: %+v", completeCheckpoint)
	}
	ctx = restart(ctx)
	assertCheckpoint(ctx, completeCheckpoint)
	result, err := ctx.RunJobMigration(options)
	if err != nil || !result.Complete || result.Phase != models.JobMigrationPhaseComplete {
		t.Fatalf("fresh process finalizes complete checkpoint = %+v, %v", result, err)
	}
	for _, row := range sourceRows {
		assertRow(ctx, row.ID, true)
	}
	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil || !readiness.Ready {
		t.Fatalf("readiness after restarted completion = %+v, %v", readiness, err)
	}
}

func TestJobMigrationScrubAndMarkerAreAtomic(t *testing.T) {
	tests := []struct {
		name string
		kind string
		seed func(*testing.T, *MahresourcesContext) (string, string, func() bool)
	}{
		{
			name: "download history",
			kind: jobMigrationDownloadHistory,
			seed: func(t *testing.T, ctx *MahresourcesContext) (string, string, func() bool) {
				t.Helper()
				row := models.DownloadHistoryEntry{JobID: "atomic-scrub-history", URL: "https://user:secret@download.example.invalid/private?token=secret",
					Payload: []byte(`{"url":"https://user:secret@download.example.invalid/private?token=secret"}`),
					Status:  models.DownloadHistoryStatusFailed, CreatedAt: time.Now().UTC()}
				if err := ctx.db.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
				id := strconv.FormatUint(uint64(row.ID), 10)
				return id, hashDownloadHistory(row), func() bool {
					var got models.DownloadHistoryEntry
					if err := ctx.db.First(&got, row.ID).Error; err != nil {
						t.Fatal(err)
					}
					return string(got.Payload) == string(row.Payload) && got.URL == row.URL
				}
			},
		},
		{
			name: "scheduled download",
			kind: jobMigrationScheduledDownload,
			seed: func(t *testing.T, ctx *MahresourcesContext) (string, string, func() bool) {
				t.Helper()
				row := models.ScheduledDownload{PluginName: "worker", URL: "https://user:secret@scheduled.example.invalid/private?token=secret",
					Payload: []byte(`{"url":"https://user:secret@scheduled.example.invalid/private?token=secret"}`),
					DueAt:   time.Now().UTC(), Status: models.ScheduledDownloadStatusPending, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
				if err := ctx.db.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
				id := strconv.FormatUint(uint64(row.ID), 10)
				return id, hashScheduledDownload(row), func() bool {
					var got models.ScheduledDownload
					if err := ctx.db.First(&got, row.ID).Error; err != nil {
						t.Fatal(err)
					}
					return string(got.Payload) == string(row.Payload) && got.URL == row.URL
				}
			},
		},
		{
			name: "plugin command run",
			kind: jobMigrationPluginCommandRun,
			seed: func(t *testing.T, ctx *MahresourcesContext) (string, string, func() bool) {
				t.Helper()
				row := models.PluginCommandRun{ID: "atomic-scrub-run", PluginName: "worker", CommandName: "ingest",
					ParamsJSON: `{"token":"secret"}`, InputsJSON: `[{"name":"private.csv"}]`,
					Status: models.PluginCommandRunStatusQueued, CreatedAt: time.Now().UTC()}
				if err := ctx.db.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
				return row.ID, hashPluginCommandRun(row), func() bool {
					var got models.PluginCommandRun
					if err := ctx.db.First(&got, "id = ?", row.ID).Error; err != nil {
						t.Fatal(err)
					}
					return got.ParamsJSON == row.ParamsJSON && got.InputsJSON == row.InputsJSON
				}
			},
		},
		{
			name: "plugin command import",
			kind: jobMigrationPluginCommandImport,
			seed: func(t *testing.T, ctx *MahresourcesContext) (string, string, func() bool) {
				t.Helper()
				row := models.PluginCommandImport{ID: "atomic-scrub-import", RunID: "atomic-scrub-run", FileName: "private.csv",
					FieldsJSON: `{"column":"secret"}`, Status: models.PluginCommandImportStatusPending, CreatedAt: time.Now().UTC()}
				if err := ctx.db.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
				return row.ID, hashPluginCommandImport(row), func() bool {
					var got models.PluginCommandImport
					if err := ctx.db.First(&got, "id = ?", row.ID).Error; err != nil {
						t.Fatal(err)
					}
					return got.FieldsJSON == row.FieldsJSON
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newJobHarnessContext(t, false)
			if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
				t.Fatal(err)
			}
			if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Truncate(time.Second)
			sourceID, sourceHash, sourceIntact := tt.seed(t, ctx)
			mapping := models.JobSourceMapping{SourceKind: tt.kind, SourceID: sourceID, JobID: "canonical-job",
				SourceRevision: 1, SourceHash: sourceHash, Status: models.JobSourceMappingVerified,
				Origin: models.JobSourceOriginBackfilled, CopiedAt: now, VerifiedAt: &now, CreatedAt: now, UpdatedAt: now}
			if err := ctx.db.Create(&mapping).Error; err != nil {
				t.Fatal(err)
			}
			checkpoint := models.JobMigrationCheckpoint{ID: models.JobMigrationCheckpointRowID, Phase: models.JobMigrationPhaseScrub,
				SourceKind: tt.kind, UpdatedAt: now}
			if err := ctx.db.Create(&checkpoint).Error; err != nil {
				t.Fatal(err)
			}
			trigger := "CREATE TRIGGER fail_scrub_marker BEFORE UPDATE ON job_source_mappings WHEN OLD.source_kind = '" + tt.kind +
				"' AND OLD.source_id = '" + sourceID + "' AND NEW.status = 'scrubbed' BEGIN SELECT RAISE(ABORT, 'injected scrub marker failure'); END"
			if err := ctx.db.Exec(trigger).Error; err != nil {
				t.Fatal(err)
			}
			_, err := ctx.RunJobMigration(JobMigrationOptions{BatchSize: 1, MaxBatches: 1})
			if err == nil {
				t.Fatal("migration unexpectedly saved scrubbed source without writing its marker")
			}
			if !sourceIntact() {
				t.Fatal("source plaintext was scrubbed even though its mapping marker failed")
			}
			var afterFailure models.JobSourceMapping
			if err := ctx.db.Where("source_kind = ? AND source_id = ?", tt.kind, sourceID).First(&afterFailure).Error; err != nil {
				t.Fatal(err)
			}
			if afterFailure.Status != models.JobSourceMappingVerified || afterFailure.ScrubbedAt != nil || afterFailure.PostScrubHash != "" {
				t.Fatalf("failed marker write changed mapping state: %+v", afterFailure)
			}
			if err := ctx.db.Exec("DROP TRIGGER fail_scrub_marker").Error; err != nil {
				t.Fatal(err)
			}
			if _, err := ctx.RunJobMigration(JobMigrationOptions{BatchSize: 1, MaxBatches: 1}); err != nil {
				t.Fatalf("resumed scrub failed: %v", err)
			}
			var afterResume models.JobSourceMapping
			if err := ctx.db.Where("source_kind = ? AND source_id = ?", tt.kind, sourceID).First(&afterResume).Error; err != nil {
				t.Fatal(err)
			}
			if afterResume.Status != models.JobSourceMappingScrubbed || afterResume.ScrubbedAt == nil || afterResume.PostScrubHash == "" {
				t.Fatalf("resumed source did not acquire scrub marker: %+v", afterResume)
			}
			if sourceIntact() {
				t.Fatal("resumed scrub retained replay plaintext")
			}
		})
	}
}

func TestJobMigrationReadinessChecksCanonicalOnlyQueuedReplay(t *testing.T) {
	tests := []struct {
		name        string
		breakReplay func(t *testing.T, ctx *MahresourcesContext, jobID string)
	}{
		{
			name: "missing key",
			breakReplay: func(t *testing.T, ctx *MahresourcesContext, _ string) {
				holdJobReplayKey(t, ctx, sharedReplayKey(t))
			},
		},
		{
			name: "corrupt envelope",
			breakReplay: func(t *testing.T, ctx *MahresourcesContext, jobID string) {
				if err := ctx.db.Model(&models.JobReplayEnvelope{}).Where("job_id = ?", jobID).
					Update("ciphertext", []byte("corrupt ciphertext")).Error; err != nil {
					t.Fatalf("corrupt canonical replay envelope: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newJobHarnessContext(t, false)
			migrated, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 10, MaxBatches: 20, WritersDrained: true})
			if err != nil || !migrated.Complete {
				t.Fatalf("empty source migration = %+v, %v", migrated, err)
			}
			jobID := acceptCanonicalOnlyQueuedExportForReadiness(t, ctx)

			readiness, err := ctx.GetJobMigrationReadiness()
			if err != nil || !readiness.Ready {
				t.Fatalf("valid canonical-only queued export should be ready: %+v, %v", readiness, err)
			}

			tt.breakReplay(t, ctx, jobID)
			readiness, err = ctx.GetJobMigrationReadiness()
			if err != nil {
				t.Fatal(err)
			}
			if readiness.Ready || readiness.Blockers["nonterminal-replay-unavailable/canonical"] != 1 {
				t.Fatalf("unreadable canonical-only queued export should block readiness: %+v", readiness)
			}
			encoded, err := json.Marshal(readiness)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "corrupt ciphertext") || strings.Contains(string(encoded), jobID) {
				t.Fatalf("readiness exposed replay data or a Job id: %s", encoded)
			}
		})
	}
}

func acceptCanonicalOnlyQueuedExportForReadiness(t *testing.T, ctx *MahresourcesContext) string {
	t.Helper()
	ctx.Config.MaxJobConcurrency = 1
	holdTheDeploymentBudgetIn(t, ctx)
	groupID := createExportGroupForTest(t, ctx, "canonical-only-readiness-export")
	submission := ctx.SubmitGroupExport(exportRequestForTest(groupID), "api")
	if submission.Err != nil {
		t.Fatalf("submit capacity-queued export: %v", submission.Err)
	}
	if submission.CanonicalJobID == "" {
		t.Fatal("capacity-queued export has no canonical Job id")
	}
	var job models.Job
	if err := ctx.db.Where("id = ?", submission.CanonicalJobID).First(&job).Error; err != nil {
		t.Fatalf("read canonical-only queued export: %v", err)
	}
	if job.State != string(jobs.StateQueued) || job.ReplayClass != string(jobs.ReplayClassReplayable) {
		t.Fatalf("capacity-queued export is %s/%s, want queued/replayable", job.State, job.ReplayClass)
	}
	var mappings int64
	if err := ctx.db.Model(&models.JobSourceMapping{}).Where("job_id = ?", job.ID).Count(&mappings).Error; err != nil {
		t.Fatalf("count source mappings for queued export: %v", err)
	}
	if mappings != 0 {
		t.Fatalf("canonical-only export has %d source mappings, want none", mappings)
	}
	return job.ID
}
