package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_commands"
)

func TestJobMigrationResumesProvenBlankSuccessfulPluginImports(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.PluginCommandImportMap{}); err != nil {
		t.Fatal(err)
	}
	category := models.ResourceCategory{Name: "migration-proof"}
	if err := ctx.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}

	created := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	runStarted, runFinished := created.Add(time.Minute), created.Add(2*time.Minute)
	run := models.PluginCommandRun{ID: "legacy-import-proof-run", PluginName: "worker", CommandName: "ingest",
		ParamsJSON: `{}`, Status: plugin_commands.RunStatusSucceeded, CreatedAt: created,
		StartedAt: &runStarted, FinishedAt: &runFinished}
	if err := ctx.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}

	imports := make([]models.PluginCommandImport, 8)
	resources := make([]models.Resource, len(imports))
	migrationNow := runFinished.Add(10 * time.Minute)
	for i := range imports {
		fileName := fmt.Sprintf("historical-%02d.csv", i)
		if i == len(imports)-1 {
			fileName = strings.Repeat("界", 90) + ".csv"
		}
		createdAt := runFinished.Add(time.Duration(i+1) * time.Minute)
		startedAt, finishedAt := createdAt.Add(10*time.Second), createdAt.Add(time.Minute)
		imports[i] = models.PluginCommandImport{ID: fmt.Sprintf("legacy-import-proof-%02d", i), RunID: run.ID,
			FileName: fileName, PluginGeneration: 8, Status: plugin_commands.ImportStatusSucceeded,
			CreatedAt: createdAt, StartedAt: &startedAt, FinishedAt: &finishedAt}
		resources[i] = models.Resource{Name: fmt.Sprintf("historical resource %02d", i), ResourceCategoryId: category.ID}
		if err := ctx.db.Create(&resources[i]).Error; err != nil {
			t.Fatalf("create imported Resource %d: %v", i, err)
		}
		if err := ctx.db.Create(&imports[i]).Error; err != nil {
			t.Fatalf("create historical import %d: %v", i, err)
		}
		resourceID := resources[i].ID
		if err := ctx.db.Create(&models.PluginCommandImportMap{RunID: run.ID, FileName: fileName, ImportID: imports[i].ID,
			ResourceID: &resourceID, Status: plugin_commands.ImportStatusSucceeded}).Error; err != nil {
			t.Fatalf("create successful import map %d: %v", i, err)
		}
		if err := ctx.db.Create(&models.JobSourceMapping{SourceKind: jobMigrationPluginCommandImport, SourceID: imports[i].ID,
			SourceRevision: 1, SourceHash: hashPluginCommandImport(imports[i]), Status: models.JobSourceMappingQuarantined,
			BlockerCode: "source-input-unreadable", Origin: models.JobSourceOriginBackfilled, CopiedAt: migrationNow,
			CreatedAt: migrationNow, UpdatedAt: migrationNow}).Error; err != nil {
			t.Fatalf("seed quarantined import mapping %d: %v", i, err)
		}
	}

	if err := ctx.copyPluginCommandRun(run, migrationNow); err != nil {
		t.Fatalf("copy successful parent run: %v", err)
	}
	more, _, err := ctx.copyPluginCommandImportsBatch("", 20, migrationNow)
	if err != nil || more {
		t.Fatalf("copy proven blank imports: more=%t err=%v", more, err)
	}
	more, _, err = ctx.copyPluginCommandImportsBatch("", 20, migrationNow.Add(time.Minute))
	if err != nil || more {
		t.Fatalf("repeat import copy: more=%t err=%v", more, err)
	}
	if more, _, err := ctx.verifyPluginCommandImportsBatch("", 20, migrationNow.Add(2*time.Minute)); err != nil || more {
		t.Fatalf("verify proven blank imports: more=%t err=%v", more, err)
	}

	var migratedRun models.PluginCommandRun
	if err := ctx.db.Where("id = ?", run.ID).First(&migratedRun).Error; err != nil {
		t.Fatal(err)
	}
	var jobCount int64
	if err := ctx.db.Model(&models.Job{}).Count(&jobCount).Error; err != nil {
		t.Fatal(err)
	}
	if jobCount != int64(len(imports)+1) {
		t.Fatalf("canonical Job count = %d, want parent plus %d imports", jobCount, len(imports))
	}
	for i, source := range imports {
		var migrated models.PluginCommandImport
		if err := ctx.db.Where("id = ?", source.ID).First(&migrated).Error; err != nil {
			t.Fatal(err)
		}
		if migrated.JobID == "" || migrated.FieldsJSON != "" {
			t.Fatalf("source import %d was not mapped without adding fields: %+v", i, migrated)
		}
		var mapping models.JobSourceMapping
		if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandImport, source.ID).First(&mapping).Error; err != nil {
			t.Fatal(err)
		}
		if mapping.JobID != migrated.JobID || mapping.Status != models.JobSourceMappingVerified || mapping.BlockerCode != "" || mapping.SourceHash != hashPluginCommandImport(source) || mapping.Origin != models.JobSourceOriginBackfilled || mapping.SourceRevision != 1 || !mapping.CreatedAt.Equal(migrationNow) || !mapping.CopiedAt.Equal(migrationNow) {
			t.Fatalf("resumed import mapping %d = %+v", i, mapping)
		}
		var job models.Job
		if err := ctx.db.Where("id = ?", migrated.JobID).First(&job).Error; err != nil {
			t.Fatal(err)
		}
		if job.State != string(jobs.StateSucceeded) || jobs.ReplayClass(job.ReplayClass) != jobs.ReplayClassNonReplayable ||
			!job.AcceptedAt.Equal(source.CreatedAt) || !sameJobTime(job.StartedAt, source.StartedAt) || !sameJobTime(job.FinishedAt, source.FinishedAt) {
			t.Fatalf("canonical import Job %d lost outcome or timestamps: %+v", i, job)
		}
		var envelopes int64
		if err := ctx.db.Model(&models.JobReplayEnvelope{}).Where("job_id = ?", job.ID).Count(&envelopes).Error; err != nil {
			t.Fatal(err)
		}
		if envelopes != 0 {
			t.Fatalf("blank historical import %d received a fabricated replay envelope", i)
		}
		var link models.JobLink
		if err := ctx.db.Where("type = ? AND from_job_id = ? AND to_job_id = ?", models.JobLinkParentChild, migratedRun.JobID, job.ID).First(&link).Error; err != nil {
			t.Fatalf("import %d parent lineage: %v", i, err)
		}
		var output models.JobOutput
		if err := ctx.db.Where("job_id = ? AND key = ?", job.ID, jobMigrationPluginCommandImportResourceOutput).First(&output).Error; err != nil {
			t.Fatalf("import %d Resource output: %v", i, err)
		}
		var reference map[string]uint
		if err := json.Unmarshal(output.Reference, &reference); err != nil || reference["resourceId"] != resources[i].ID {
			t.Fatalf("import %d output reference = %s, err=%v", i, output.Reference, err)
		}
		var events []models.JobEvent
		if err := ctx.db.Where("job_id = ?", job.ID).Order("sequence ASC").Find(&events).Error; err != nil {
			t.Fatalf("read import %d timeline: %v", i, err)
		}
		if len(events) != 4 {
			t.Fatalf("import %d timeline has %d events, want lifecycle plus output publication: %+v", i, len(events), events)
		}
		for eventIndex, event := range events {
			if event.Sequence != uint64(eventIndex+1) {
				t.Fatalf("import %d timeline sequence at index %d = %d", i, eventIndex, event.Sequence)
			}
		}
		succeeded := events[len(events)-2]
		publication := events[len(events)-1]
		if source.FinishedAt == nil {
			t.Fatalf("import %d source has no finish timestamp", i)
		}
		var detail struct {
			Key   string `json:"key"`
			Type  string `json:"type"`
			State string `json:"state"`
		}
		if publication.Type != jobs.EventOutputPublished || succeeded.Type != jobs.EventSucceeded ||
			publication.JobVersion != succeeded.JobVersion || succeeded.JobVersion != job.Version ||
			publication.Sequence != succeeded.Sequence+1 ||
			!publication.CreatedAt.UTC().Equal(source.FinishedAt.UTC()) || json.Unmarshal(publication.Detail, &detail) != nil ||
			detail.Key != jobMigrationPluginCommandImportResourceOutput || detail.Type != string(jobs.OutputTypeEntity) || detail.State != string(jobs.OutputAvailable) {
			t.Fatalf("import %d success/output events = %+v / %+v detail=%+v, want append-only publication after success at source finish time", i, succeeded, publication, detail)
		}
		if i == len(imports)-1 && (len(job.Title) > jobs.MaxTitleBytes || !utf8.ValidString(job.Title)) {
			t.Fatalf("long filename produced invalid Job title %q (%d bytes)", job.Title, len(job.Title))
		}
	}

	result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 20, MaxBatches: 100, WritersDrained: true,
		Now: func() time.Time { return migrationNow.Add(time.Hour) }})
	if err != nil || !result.Complete {
		var checkpoint models.JobMigrationCheckpoint
		_ = ctx.db.First(&checkpoint, models.JobMigrationCheckpointRowID).Error
		var mappings []models.JobSourceMapping
		_ = ctx.db.Order("source_kind, source_id").Find(&mappings).Error
		t.Fatalf("complete migration after blank imports: %+v, %v (checkpoint=%+v mappings=%+v)", result, err, checkpoint, mappings)
	}
	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil || !readiness.Ready {
		t.Fatalf("readiness after proven import migration = %+v, %v", readiness, err)
	}
	for _, source := range imports {
		var migrated models.PluginCommandImport
		if err := ctx.db.Where("id = ?", source.ID).First(&migrated).Error; err != nil {
			t.Fatal(err)
		}
		if migrated.FieldsJSON != "" || migrated.JobID == "" {
			t.Fatalf("retired import source has unexpected state: %+v", migrated)
		}
	}

	// A pre-retirement backup can restore the source row without the backfilled
	// canonical Job ID. Successful result evidence and the mapping must still
	// prove that this blank historical import can be retired again.
	restoredAt := migrationNow.Add(2 * time.Hour)
	if err := ctx.db.Model(&models.PluginCommandImport{}).Where("id = ?", imports[0].ID).Update("job_id", "").Error; err != nil {
		t.Fatalf("restore old plugin import source projection: %v", err)
	}
	recovered, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 20, MaxBatches: 100, WritersDrained: true,
		Now: func() time.Time { return restoredAt.Add(time.Hour) }})
	if err != nil || !recovered.Complete {
		t.Fatalf("rescrub restored blank import source: %+v, %v", recovered, err)
	}
	var recoveredMapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandImport, imports[0].ID).First(&recoveredMapping).Error; err != nil {
		t.Fatal(err)
	}
	if recoveredMapping.Status != models.JobSourceMappingScrubbed || recoveredMapping.SourceRevision != 2 || recoveredMapping.JobID == "" || recoveredMapping.BlockerCode != "" {
		t.Fatalf("recovered blank import mapping = %+v", recoveredMapping)
	}
	readiness, err = ctx.GetJobMigrationReadiness()
	if err != nil || !readiness.Ready {
		t.Fatalf("readiness after restored blank import recovery = %+v, %v", readiness, err)
	}
}

func TestJobMigrationAppendsBlankImportOutputAfterPublishedSuccess(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.PluginCommandImportMap{}); err != nil {
		t.Fatal(err)
	}
	category := models.ResourceCategory{Name: "published-import-proof"}
	if err := ctx.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}

	created := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	runStarted, runFinished := created.Add(time.Minute), created.Add(2*time.Minute)
	run := models.PluginCommandRun{ID: "published-import-parent", PluginName: "worker", CommandName: "ingest",
		ParamsJSON: `{}`, Status: plugin_commands.RunStatusSucceeded, CreatedAt: created,
		StartedAt: &runStarted, FinishedAt: &runFinished}
	if err := ctx.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	migrationNow := runFinished.Add(10 * time.Minute)
	if err := ctx.copyPluginCommandRun(run, migrationNow); err != nil {
		t.Fatalf("copy successful parent run: %v", err)
	}
	var migratedRun models.PluginCommandRun
	if err := ctx.db.Where("id = ?", run.ID).First(&migratedRun).Error; err != nil {
		t.Fatal(err)
	}

	importCreated := runFinished.Add(time.Minute)
	importStarted, importFinished := importCreated.Add(10*time.Second), importCreated.Add(time.Minute)
	row := models.PluginCommandImport{ID: "published-blank-import", RunID: run.ID, FileName: "historical.csv",
		Status: plugin_commands.ImportStatusSucceeded, CreatedAt: importCreated, StartedAt: &importStarted, FinishedAt: &importFinished}
	if err := ctx.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.Resource{Name: "historical import result", ResourceCategoryId: category.ID}
	if err := ctx.db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	resourceID := resource.ID
	if err := ctx.db.Create(&models.PluginCommandImportMap{RunID: run.ID, FileName: row.FileName, ImportID: row.ID,
		ResourceID: &resourceID, Status: plugin_commands.ImportStatusSucceeded}).Error; err != nil {
		t.Fatal(err)
	}

	canonical, err := ctx.JobService().ImportLegacy(ctx.jobDeps(), jobs.LegacyImport{
		Acceptance: jobs.Acceptance{Kind: JobKindPluginCommandImport, KindVersion: jobPluginCommandVersion,
			State: jobs.StateSucceeded, Origin: "plugin", Title: pluginCommandImportJobTitle(row.FileName),
			Replay: jobs.ReplayInput{NonReplayable: true}, Parents: []string{migratedRun.JobID},
			LegacyRefs: []jobs.LegacyRef{{Namespace: pluginCommandImportHandleNamespace, Handle: row.ID}}},
		State: jobs.StateSucceeded, AcceptedAt: row.CreatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
	})
	if err != nil {
		t.Fatalf("seed existing canonical import Job: %v", err)
	}
	if _, err := ctx.JobService().PublishPendingEvents(ctx.jobDeps(), 100); err != nil {
		t.Fatalf("publish existing Job events: %v", err)
	}
	var beforeJob models.Job
	if err := ctx.db.Where("id = ?", canonical.ID).First(&beforeJob).Error; err != nil {
		t.Fatal(err)
	}
	var beforeEvents []models.JobEvent
	if err := ctx.db.Where("job_id = ?", canonical.ID).Order("sequence ASC").Find(&beforeEvents).Error; err != nil {
		t.Fatal(err)
	}
	if len(beforeEvents) < 2 {
		t.Fatalf("existing canonical Job timeline = %+v, want lifecycle events", beforeEvents)
	}
	var success models.JobEvent
	for _, event := range beforeEvents {
		if event.Type == jobs.EventSucceeded {
			success = event
		}
	}
	if success.ID == "" || success.DeliverySequence == nil {
		t.Fatalf("canonical success event is not published: %+v", success)
	}
	cursor := *success.DeliverySequence
	if tail, err := ctx.JobService().PublishedEvents(ctx.jobDeps(), jobs.Access{Administrator: true}, cursor, 0); err != nil || len(tail) != 0 {
		t.Fatalf("stream after existing success before migration = %+v, %v; want no events", tail, err)
	}

	if err := ctx.copyPluginCommandImport(row, migrationNow); err != nil {
		t.Fatalf("copy proven blank import onto existing canonical Job: %v", err)
	}
	if _, err := ctx.JobService().PublishPendingEvents(ctx.jobDeps(), 100); err != nil {
		t.Fatalf("publish appended historical output event: %v", err)
	}
	var afterJob models.Job
	if err := ctx.db.Where("id = ?", canonical.ID).First(&afterJob).Error; err != nil {
		t.Fatal(err)
	}
	if afterJob.Version != beforeJob.Version || afterJob.State != beforeJob.State || !sameJobTime(afterJob.FinishedAt, beforeJob.FinishedAt) {
		t.Fatalf("migration changed canonical Job version or outcome: before=%+v after=%+v", beforeJob, afterJob)
	}
	var afterEvents []models.JobEvent
	if err := ctx.db.Where("job_id = ?", canonical.ID).Order("sequence ASC").Find(&afterEvents).Error; err != nil {
		t.Fatal(err)
	}
	if len(afterEvents) != len(beforeEvents)+1 {
		t.Fatalf("canonical timeline grew from %d to %d events, want one append", len(beforeEvents), len(afterEvents))
	}
	for i, before := range beforeEvents {
		after := afterEvents[i]
		if before.ID != after.ID || before.Sequence != after.Sequence || before.JobVersion != after.JobVersion ||
			before.Type != after.Type || string(before.Detail) != string(after.Detail) || before.ReservedHost != after.ReservedHost ||
			!before.CreatedAt.Equal(after.CreatedAt) || !sameDeliverySequence(before.DeliverySequence, after.DeliverySequence) {
			t.Fatalf("existing Job event %d changed during migration: before=%+v after=%+v", i, before, after)
		}
	}
	publication := afterEvents[len(afterEvents)-1]
	if publication.Type != jobs.EventOutputPublished || publication.Sequence != beforeEvents[len(beforeEvents)-1].Sequence+1 ||
		publication.JobVersion != beforeJob.Version || publication.DeliverySequence == nil || *publication.DeliverySequence <= cursor {
		t.Fatalf("historical output was not appended after the published timeline: %+v", publication)
	}
	tail, err := ctx.JobService().PublishedEvents(ctx.jobDeps(), jobs.Access{Administrator: true}, cursor, 0)
	if err != nil || len(tail) != 1 || tail[0].ID != publication.ID || tail[0].DeliverySequence == nil || *tail[0].DeliverySequence != *publication.DeliverySequence {
		t.Fatalf("published stream after prior success cursor = %+v, %v; want only appended output event %+v", tail, err, publication)
	}
}

func sameDeliverySequence(left, right *uint64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func TestJobMigrationBlocksBlankPluginImportsWithoutTerminalProof(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	created := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	started, finished := created.Add(time.Minute), created.Add(2*time.Minute)
	rows := []models.PluginCommandImport{
		{ID: "blank-import-queued", RunID: "parent", FileName: "queued.csv", Status: plugin_commands.ImportStatusPending, CreatedAt: created},
		{ID: "blank-import-running", RunID: "parent", FileName: "running.csv", Status: plugin_commands.ImportStatusRunning, CreatedAt: created, StartedAt: &started},
		{ID: "blank-import-no-proof", RunID: "missing-parent", FileName: "missing.csv", Status: plugin_commands.ImportStatusSucceeded, CreatedAt: created, FinishedAt: &finished},
	}
	for _, row := range rows {
		if err := ctx.db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		err := ctx.copyPluginCommandImport(row, finished.Add(time.Hour))
		var blocker *jobMigrationBlockerError
		if !errors.As(err, &blocker) {
			t.Fatalf("blank import %s error = %v, want a durable migration blocker", row.ID, err)
		}
		if row.Status != plugin_commands.ImportStatusSucceeded && blocker.code != "source-input-unreadable" {
			t.Fatalf("blank nonterminal import %s blocker = %q", row.ID, blocker.code)
		}
	}
	var jobs int64
	if err := ctx.db.Model(&models.Job{}).Count(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("unproven blank imports created %d canonical Jobs", jobs)
	}
}

func TestJobMigrationRecognizesDualPublishedRunningPluginImport(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	created := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	runStarted, runFinished := created.Add(time.Minute), created.Add(2*time.Minute)
	run := models.PluginCommandRun{ID: "running-import-parent", PluginName: "worker", CommandName: "ingest",
		ParamsJSON: `{}`, InputsJSON: `[]`, Status: plugin_commands.RunStatusSucceeded,
		CreatedAt: created, StartedAt: &runStarted, FinishedAt: &runFinished}
	if err := ctx.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	migrationNow := runFinished.Add(10 * time.Minute)
	if err := ctx.copyPluginCommandRun(run, migrationNow); err != nil {
		t.Fatalf("copy successful parent run: %v", err)
	}
	var migratedRun models.PluginCommandRun
	if err := ctx.db.Where("id = ?", run.ID).First(&migratedRun).Error; err != nil {
		t.Fatal(err)
	}

	importCreated := runFinished.Add(time.Minute)
	importStarted := importCreated.Add(10 * time.Second)
	row := models.PluginCommandImport{ID: "dual-published-running-import", RunID: run.ID, FileName: "active.csv",
		FieldsJSON: `{"title":"active"}`, PluginGeneration: 8, Status: plugin_commands.ImportStatusRunning,
		CreatedAt: importCreated, StartedAt: &importStarted}
	jobID, err := ctx.acceptPluginCommandImportJob(ctx.db, migratedRun, row, "")
	if err != nil {
		t.Fatalf("accept live import Job: %v", err)
	}
	row.JobID = jobID
	if err := ctx.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	registerClaimableKind(t, ctx, JobKindPluginCommandImport, jobPluginCommandVersion)
	execution, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindPluginCommandImport, KindVersion: jobPluginCommandVersion, Claimant: "migration-test",
	})
	if err != nil || !claimed || execution.JobID != jobID {
		t.Fatalf("claim live import Job = %+v, claimed=%t, err=%v", execution, claimed, err)
	}
	before := jobSnapshot(t, ctx.JobService(), ctx, jobID)
	if before.State != jobs.StateRunning {
		t.Fatalf("live import Job state = %q, want running", before.State)
	}
	var mappingsBefore int64
	if err := ctx.db.Model(&models.JobSourceMapping{}).Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandImport, row.ID).Count(&mappingsBefore).Error; err != nil {
		t.Fatal(err)
	}
	if mappingsBefore != 0 {
		t.Fatalf("live import has %d migration mappings before copy, want none", mappingsBefore)
	}

	if err := ctx.copyPluginCommandImport(row, migrationNow); err != nil {
		t.Fatalf("copy live dual-published import: %v", err)
	}
	more, _, err := ctx.verifyPluginCommandImportsBatch("", 10, migrationNow.Add(time.Minute))
	if err != nil || more {
		t.Fatalf("verify live dual-published import: more=%t err=%v", more, err)
	}
	after := jobSnapshot(t, ctx.JobService(), ctx, jobID)
	if after.State != jobs.StateRunning || after.Version != before.Version {
		t.Fatalf("migration changed live canonical Job: before=%+v after=%+v", before, after)
	}
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandImport, row.ID).First(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if mapping.JobID != jobID || mapping.Origin != models.JobSourceOriginDualPublished ||
		mapping.Status != models.JobSourceMappingVerified || mapping.BlockerCode != "" {
		t.Fatalf("live import mapping = %+v, want verified dual-published mapping", mapping)
	}
	var jobsAfter int64
	if err := ctx.db.Model(&models.Job{}).Count(&jobsAfter).Error; err != nil {
		t.Fatal(err)
	}
	if jobsAfter != 2 {
		t.Fatalf("live parent/import migration has %d canonical Jobs, want exactly two", jobsAfter)
	}
}

func TestJobMigrationDoesNotRearmBlankImportWithoutResultProof(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	finished := now.Add(time.Minute)
	row := models.PluginCommandImport{
		ID: "unproven-blank-import", RunID: "missing-parent", FileName: "missing.csv",
		Status: plugin_commands.ImportStatusSucceeded, CreatedAt: now, FinishedAt: &finished,
	}
	if err := ctx.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Create(&models.JobSourceMapping{
		SourceKind: jobMigrationPluginCommandImport, SourceID: row.ID, SourceRevision: 1,
		SourceHash: hashPluginCommandImport(row), Status: models.JobSourceMappingQuarantined,
		BlockerCode: "source-input-unreadable", Origin: models.JobSourceOriginBackfilled,
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
	if err != nil || result.Phase != models.JobMigrationPhaseDrainFence || result.BlockedSources != 1 || result.Batches != 0 {
		t.Fatalf("unproven import rearm = %+v, %v", result, err)
	}
	var checkpoint models.JobMigrationCheckpoint
	if err := ctx.db.First(&checkpoint, models.JobMigrationCheckpointRowID).Error; err != nil {
		t.Fatal(err)
	}
	if checkpoint.Phase != models.JobMigrationPhaseDrainFence || checkpoint.SourceKind != "" || checkpoint.CursorID != "" {
		t.Fatalf("unproven import moved checkpoint: %+v", checkpoint)
	}
}
