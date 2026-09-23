package jobs

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

func TestForgetReplayAtomicallyPurgesLegacySourceCopies(t *testing.T) {
	deps, _ := newReplayDeps(t)
	if err := deps.DB.AutoMigrate(
		&models.JobSourceMapping{}, &models.DownloadHistoryEntry{}, &models.ScheduledDownload{},
		&models.PluginCommandRun{}, &models.PluginCommandImport{},
	); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2037, 2, 3, 4, 5, 6, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "retirement-key"), Retention: time.Hour}
	jobID := terminalReplayJob(t, svc, deps, &clock)
	sources := seedLegacyReplaySources(t, deps.DB, jobID, clock)

	if err := deps.DB.Exec(`CREATE TRIGGER fail_source_mapping_update BEFORE UPDATE ON job_source_mappings BEGIN SELECT RAISE(ABORT, 'injected mapping failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ForgetReplay(deps, Access{Administrator: true}, jobID); err == nil {
		t.Fatal("ForgetReplay succeeded despite an injected source-mapping failure")
	}
	assertReplayPurgeUnchanged(t, deps.DB, jobID, sources)
	if err := deps.DB.Exec(`DROP TRIGGER fail_source_mapping_update`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ForgetReplay(deps, Access{Administrator: true}, jobID); err != nil {
		t.Fatalf("ForgetReplay after removing fault: %v", err)
	}
	assertReplaySourcesPurged(t, deps.DB, jobID, sources, models.JobReplayPurgeForgotten)
}

func TestExpiredReplaySweepAtomicallyPurgesLegacySourceCopies(t *testing.T) {
	deps, _ := newReplayDeps(t)
	if err := deps.DB.AutoMigrate(
		&models.JobSourceMapping{}, &models.DownloadHistoryEntry{}, &models.ScheduledDownload{},
		&models.PluginCommandRun{}, &models.PluginCommandImport{},
	); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2038, 3, 4, 5, 6, 7, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "retirement-key"), Retention: time.Hour}
	jobID := terminalReplayJob(t, svc, deps, &clock)
	sources := seedLegacyReplaySources(t, deps.DB, jobID, clock)
	clock = clock.Add(2 * time.Hour)

	if err := deps.DB.Exec(`CREATE TRIGGER fail_source_mapping_update BEFORE UPDATE ON job_source_mappings BEGIN SELECT RAISE(ABORT, 'injected mapping failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PurgeExpiredReplay(deps, 10); err == nil {
		t.Fatal("PurgeExpiredReplay succeeded despite an injected source-mapping failure")
	}
	assertReplayPurgeUnchanged(t, deps.DB, jobID, sources)
	if err := deps.DB.Exec(`DROP TRIGGER fail_source_mapping_update`).Error; err != nil {
		t.Fatal(err)
	}
	if purged, err := svc.PurgeExpiredReplay(deps, 10); err != nil || purged != 1 {
		t.Fatalf("PurgeExpiredReplay after removing fault = %d, %v; want 1, nil", purged, err)
	}
	assertReplaySourcesPurged(t, deps.DB, jobID, sources, models.JobReplayPurgeExpired)
}

func TestForgetReplayPurgesUnmappedLegacySourceCopies(t *testing.T) {
	deps, _ := newReplayDeps(t)
	if err := deps.DB.AutoMigrate(
		&models.JobSourceMapping{}, &models.JobLegacyHandle{}, &models.DownloadHistoryEntry{}, &models.ScheduledDownload{},
		&models.PluginCommandRun{}, &models.PluginCommandImport{},
	); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2039, 4, 5, 6, 7, 8, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "retirement-key"), Retention: time.Hour}
	jobID := terminalReplayJob(t, svc, deps, &clock)
	sources := seedUnmappedLegacyReplaySources(t, deps.DB, jobID, clock)

	if _, err := svc.ForgetReplay(deps, Access{Administrator: true}, jobID); err != nil {
		t.Fatalf("ForgetReplay: %v", err)
	}
	assertUnmappedReplaySourcesPurged(t, deps.DB, jobID, sources, models.JobReplayPurgeForgotten)
}

func TestExpiredReplaySweepPurgesUnmappedLegacySourceCopies(t *testing.T) {
	deps, _ := newReplayDeps(t)
	if err := deps.DB.AutoMigrate(
		&models.JobSourceMapping{}, &models.JobLegacyHandle{}, &models.DownloadHistoryEntry{}, &models.ScheduledDownload{},
		&models.PluginCommandRun{}, &models.PluginCommandImport{},
	); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2040, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "retirement-key"), Retention: time.Hour}
	jobID := terminalReplayJob(t, svc, deps, &clock)
	sources := seedUnmappedLegacyReplaySources(t, deps.DB, jobID, clock)
	clock = clock.Add(2 * time.Hour)

	if purged, err := svc.PurgeExpiredReplay(deps, 10); err != nil || purged != 1 {
		t.Fatalf("PurgeExpiredReplay = %d, %v; want 1, nil", purged, err)
	}
	assertUnmappedReplaySourcesPurged(t, deps.DB, jobID, sources, models.JobReplayPurgeExpired)
}

func TestReplayPurgeRefusesWhenSourceMappingSchemaIsUnavailable(t *testing.T) {
	for _, test := range []struct {
		name  string
		purge func(*testing.T, *Service, Deps, string) error
	}{
		{
			name: "forget",
			purge: func(t *testing.T, svc *Service, deps Deps, jobID string) error {
				t.Helper()
				_, err := svc.ForgetReplay(deps, Access{Administrator: true}, jobID)
				return err
			},
		},
		{
			name: "expiry",
			purge: func(t *testing.T, svc *Service, deps Deps, jobID string) error {
				t.Helper()
				_, err := svc.PurgeExpiredReplay(deps, 10)
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			deps, _ := newReplayDeps(t)
			if err := deps.DB.AutoMigrate(&models.JobSourceMapping{}); err != nil {
				t.Fatal(err)
			}
			svc := NewService()
			if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
				t.Fatal(err)
			}
			clock := time.Date(2041, 6, 7, 8, 9, 10, 0, time.UTC)
			deps.Now = func() time.Time { return clock }
			deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "retirement-key"), Retention: time.Hour}
			jobID := terminalReplayJob(t, svc, deps, &clock)
			if test.name == "expiry" {
				clock = clock.Add(2 * time.Hour)
			}
			if err := deps.DB.Migrator().DropTable(&models.JobSourceMapping{}); err != nil {
				t.Fatal(err)
			}
			if err := test.purge(t, svc, deps, jobID); err == nil {
				t.Fatal("purge succeeded without the schema needed to prove legacy source copies are retired")
			}
			envelope := replayEnvelopeRow(t, deps, jobID)
			if envelope.PurgedAt != nil || len(envelope.Ciphertext) == 0 {
				t.Fatalf("failed purge changed canonical input: %+v", envelope)
			}
		})
	}
}

type replaySourceIDs struct {
	download          uint
	scheduled         uint
	fallbackScheduled uint
	shadowedScheduled uint
	run               string
	importID          string
}

func seedUnmappedLegacyReplaySources(t *testing.T, db *gorm.DB, jobID string, now time.Time) replaySourceIDs {
	t.Helper()
	suffix := strings.ReplaceAll(jobID, "-", "")
	if len(suffix) > 16 {
		suffix = suffix[len(suffix)-16:]
	}
	downloadHandle := "unmapped-download-" + suffix
	if err := db.Create(&models.JobLegacyHandle{Namespace: "download", Handle: downloadHandle, JobID: jobID}).Error; err != nil {
		t.Fatal(err)
	}
	download := models.DownloadHistoryEntry{
		JobID: downloadHandle, URL: "https://user:pass@example.test/unmapped?token=raw", Payload: []byte(`{"token":"unmapped-download-secret"}`),
		Status: models.DownloadHistoryStatusFailed, CreatedAt: now, Attempts: 1,
	}
	if err := db.Create(&download).Error; err != nil {
		t.Fatal(err)
	}
	scheduled := models.ScheduledDownload{
		PluginName: "fixture", URL: "https://user:pass@example.test/unmapped-later?token=raw", Payload: []byte(`{"token":"unmapped-scheduled-secret"}`),
		DueAt: now, Status: models.ScheduledDownloadStatusSubmitted, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&scheduled).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.JobLegacyHandle{
		Namespace: "scheduled-download", Handle: strconv.FormatUint(uint64(scheduled.ID), 10), JobID: jobID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	fallbackScheduled := models.ScheduledDownload{
		PluginName: "fixture", URL: "https://user:pass@example.test/unmapped-fallback?token=raw", Payload: []byte(`{"token":"unmapped-fallback-secret"}`),
		DueAt: now, Status: models.ScheduledDownloadStatusSubmitted, CreatedAt: now, UpdatedAt: now,
		JobID: downloadHandle,
	}
	if err := db.Create(&fallbackScheduled).Error; err != nil {
		t.Fatal(err)
	}
	shadowedScheduled := models.ScheduledDownload{
		PluginName: "fixture", URL: "https://user:pass@example.test/unmapped-shadowed?token=raw", Payload: []byte(`{"token":"unmapped-shadowed-secret"}`),
		DueAt: now, Status: models.ScheduledDownloadStatusSubmitted, CreatedAt: now, UpdatedAt: now,
		JobID: downloadHandle,
	}
	if err := db.Create(&shadowedScheduled).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.JobLegacyHandle{
		Namespace: "scheduled-download", Handle: strconv.FormatUint(uint64(shadowedScheduled.ID), 10), JobID: "ffffffff-ffff-4fff-8fff-ffffffffffff",
	}).Error; err != nil {
		t.Fatal(err)
	}
	run := models.PluginCommandRun{ID: "ur" + suffix, JobID: jobID, PluginName: "fixture", CommandName: "run", ParamsJSON: `{"secret":"unmapped-run-secret"}`, InputsJSON: `[{"name":"x"}]`, Status: models.PluginCommandRunStatusFailed, CreatedAt: now}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	commandImport := models.PluginCommandImport{ID: "ui" + suffix, JobID: jobID, RunID: run.ID, FileName: "input.json", FieldsJSON: `{"secret":"unmapped-import-secret"}`, Status: models.PluginCommandImportStatusFailed, CreatedAt: now}
	if err := db.Create(&commandImport).Error; err != nil {
		t.Fatal(err)
	}
	return replaySourceIDs{download: download.ID, scheduled: scheduled.ID, fallbackScheduled: fallbackScheduled.ID, shadowedScheduled: shadowedScheduled.ID, run: run.ID, importID: commandImport.ID}
}

func terminalReplayJob(t *testing.T, svc *Service, deps Deps, clock *time.Time) string {
	t.Helper()
	accepted, err := svc.Accept(deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		Replay: ReplayInput{Input: fixtureReplayInput()},
	})
	if err != nil {
		t.Fatalf("accept replay job: %v", err)
	}
	running := advanceReplayJob(t, svc, deps, accepted, StateRunning)
	*clock = clock.Add(time.Minute)
	advanceReplayJob(t, svc, deps, running, StateFailed)
	return accepted.ID
}

func seedLegacyReplaySources(t *testing.T, db *gorm.DB, jobID string, now time.Time) replaySourceIDs {
	t.Helper()
	download := models.DownloadHistoryEntry{
		JobID: "legacy-download", URL: "https://user:pass@example.test/item?token=raw", Payload: []byte(`{"token":"legacy-secret"}`),
		Status: models.DownloadHistoryStatusFailed, CreatedAt: now, Attempts: 1,
	}
	if err := db.Create(&download).Error; err != nil {
		t.Fatal(err)
	}
	scheduled := models.ScheduledDownload{
		PluginName: "fixture", URL: "https://user:pass@example.test/later?token=raw", Payload: []byte(`{"token":"legacy-secret"}`),
		DueAt: now, Status: models.ScheduledDownloadStatusSubmitted, JobID: "legacy-scheduled", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&scheduled).Error; err != nil {
		t.Fatal(err)
	}
	run := models.PluginCommandRun{ID: "legacy-run", JobID: jobID, PluginName: "fixture", CommandName: "run", ParamsJSON: `{"secret":"legacy"}`, InputsJSON: `[{"name":"x"}]`, Status: models.PluginCommandRunStatusFailed, CreatedAt: now}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	commandImport := models.PluginCommandImport{ID: "legacy-import", JobID: jobID, RunID: run.ID, FileName: "input.json", FieldsJSON: `{"secret":"legacy"}`, Status: models.PluginCommandImportStatusFailed, CreatedAt: now}
	if err := db.Create(&commandImport).Error; err != nil {
		t.Fatal(err)
	}
	rows := []models.JobSourceMapping{
		{SourceKind: "download-history", SourceID: strconv.FormatUint(uint64(download.ID), 10), JobID: jobID, SourceRevision: 1, SourceHash: "before", Status: models.JobSourceMappingCopied, Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now},
		{SourceKind: "scheduled-download", SourceID: strconv.FormatUint(uint64(scheduled.ID), 10), JobID: jobID, SourceRevision: 1, SourceHash: "before", Status: models.JobSourceMappingCopied, Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now},
		{SourceKind: "plugin-command-run", SourceID: run.ID, JobID: jobID, SourceRevision: 1, SourceHash: "before", Status: models.JobSourceMappingCopied, Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now},
		{SourceKind: "plugin-command-import", SourceID: commandImport.ID, JobID: jobID, SourceRevision: 1, SourceHash: "before", Status: models.JobSourceMappingCopied, Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return replaySourceIDs{download: download.ID, scheduled: scheduled.ID, run: run.ID, importID: commandImport.ID}
}

func assertReplayPurgeUnchanged(t *testing.T, db *gorm.DB, jobID string, ids replaySourceIDs) {
	t.Helper()
	envelope := replayEnvelopeRow(t, Deps{DB: db}, jobID)
	if envelope.PurgedAt != nil || len(envelope.Ciphertext) == 0 {
		t.Fatalf("failed purge changed canonical input: %+v", envelope)
	}
	var download models.DownloadHistoryEntry
	if err := db.First(&download, ids.download).Error; err != nil || len(download.Payload) == 0 || download.URL == "" {
		t.Fatalf("failed purge changed download source: %+v, %v", download, err)
	}
	var scheduled models.ScheduledDownload
	if err := db.First(&scheduled, ids.scheduled).Error; err != nil || len(scheduled.Payload) == 0 || scheduled.URL == "" {
		t.Fatalf("failed purge changed scheduled source: %+v, %v", scheduled, err)
	}
	if ids.fallbackScheduled != 0 {
		assertScheduledReplaySourceIntact(t, db, ids.fallbackScheduled, "failed purge changed fallback scheduled source")
	}
	if ids.shadowedScheduled != 0 {
		assertScheduledReplaySourceIntact(t, db, ids.shadowedScheduled, "failed purge changed shadowed scheduled source")
	}
	var run models.PluginCommandRun
	if err := db.First(&run, "id = ?", ids.run).Error; err != nil || run.ParamsJSON == "" || run.InputsJSON == "" {
		t.Fatalf("failed purge changed command source: %+v, %v", run, err)
	}
	var commandImport models.PluginCommandImport
	if err := db.First(&commandImport, "id = ?", ids.importID).Error; err != nil || commandImport.FieldsJSON == "" {
		t.Fatalf("failed purge changed import source: %+v, %v", commandImport, err)
	}
}

func assertReplaySourcesPurged(t *testing.T, db *gorm.DB, jobID string, ids replaySourceIDs, reason string) {
	t.Helper()
	envelope := replayEnvelopeRow(t, Deps{DB: db}, jobID)
	if envelope.PurgedAt == nil || envelope.PurgeReason != reason || envelope.Ciphertext != nil || envelope.Nonce != nil {
		t.Fatalf("canonical purge marker = %+v; want purged reason %q", envelope, reason)
	}
	var download models.DownloadHistoryEntry
	if err := db.First(&download, ids.download).Error; err != nil || len(download.Payload) != 0 || download.URL != "" {
		t.Fatalf("download source retained replay data: %+v, %v", download, err)
	}
	var scheduled models.ScheduledDownload
	if err := db.First(&scheduled, ids.scheduled).Error; err != nil || len(scheduled.Payload) != 0 || scheduled.URL != "" {
		t.Fatalf("scheduled source retained replay data: %+v, %v", scheduled, err)
	}
	if ids.fallbackScheduled != 0 {
		assertScheduledReplaySourcePurged(t, db, ids.fallbackScheduled, "fallback scheduled source retained replay data")
	}
	if ids.shadowedScheduled != 0 {
		assertScheduledReplaySourceIntact(t, db, ids.shadowedScheduled, "purge changed a scheduled row owned by another scheduled handle")
	}
	var run models.PluginCommandRun
	if err := db.First(&run, "id = ?", ids.run).Error; err != nil || run.ParamsJSON != "" || run.InputsJSON != "" {
		t.Fatalf("command source retained replay data: %+v, %v", run, err)
	}
	var commandImport models.PluginCommandImport
	if err := db.First(&commandImport, "id = ?", ids.importID).Error; err != nil || commandImport.FieldsJSON != "" {
		t.Fatalf("import source retained replay data: %+v, %v", commandImport, err)
	}
	sources := []struct{ kind, id string }{
		{kind: "download-history", id: strconv.FormatUint(uint64(ids.download), 10)},
		{kind: "scheduled-download", id: strconv.FormatUint(uint64(ids.scheduled), 10)},
		{kind: "plugin-command-run", id: ids.run},
		{kind: "plugin-command-import", id: ids.importID},
	}
	if ids.fallbackScheduled != 0 {
		sources = append(sources, struct{ kind, id string }{kind: "scheduled-download", id: strconv.FormatUint(uint64(ids.fallbackScheduled), 10)})
	}
	for _, source := range sources {
		var mapping models.JobSourceMapping
		if err := db.Where("job_id = ? AND source_kind = ? AND source_id = ?", jobID, source.kind, source.id).First(&mapping).Error; err != nil {
			t.Fatalf("load %s purge marker: %v", source.kind, err)
		}
		if mapping.Status != models.JobSourceMappingPurged || mapping.PurgedAt == nil || mapping.PurgeReason != reason || mapping.ScrubbedAt == nil {
			t.Errorf("%s source purge marker = %+v", source.kind, mapping)
		}
	}
}

func assertUnmappedReplaySourcesPurged(t *testing.T, db *gorm.DB, jobID string, ids replaySourceIDs, reason string) {
	t.Helper()
	assertSourceFieldsPurged(t, db, ids)
	for _, source := range []struct{ kind, id string }{
		{kind: "download-history", id: strconv.FormatUint(uint64(ids.download), 10)},
		{kind: "scheduled-download", id: strconv.FormatUint(uint64(ids.scheduled), 10)},
		{kind: "plugin-command-run", id: ids.run},
		{kind: "plugin-command-import", id: ids.importID},
	} {
		var mapping models.JobSourceMapping
		if err := db.Where("source_kind = ? AND source_id = ?", source.kind, source.id).First(&mapping).Error; err != nil {
			t.Fatalf("load discovered %s purge marker: %v", source.kind, err)
		}
		if mapping.JobID != jobID || mapping.Status != models.JobSourceMappingPurged || mapping.PurgedAt == nil || mapping.PurgeReason != reason || mapping.ScrubbedAt == nil {
			t.Errorf("discovered %s purge marker = %+v", source.kind, mapping)
		}
	}
}

func assertUnmappedReplaySourcesIntact(t *testing.T, db *gorm.DB, ids replaySourceIDs) {
	t.Helper()
	var download models.DownloadHistoryEntry
	if err := db.First(&download, ids.download).Error; err != nil || len(download.Payload) == 0 || download.URL == "" {
		t.Fatalf("unmapped download source changed during rejected purge: %+v, %v", download, err)
	}
	var scheduled models.ScheduledDownload
	if err := db.First(&scheduled, ids.scheduled).Error; err != nil || len(scheduled.Payload) == 0 || scheduled.URL == "" {
		t.Fatalf("unmapped scheduled source changed during rejected purge: %+v, %v", scheduled, err)
	}
	if ids.fallbackScheduled != 0 {
		assertScheduledReplaySourceIntact(t, db, ids.fallbackScheduled, "unmapped fallback scheduled source changed during rejected purge")
	}
	if ids.shadowedScheduled != 0 {
		assertScheduledReplaySourceIntact(t, db, ids.shadowedScheduled, "unmapped shadowed scheduled source changed during rejected purge")
	}
	var run models.PluginCommandRun
	if err := db.First(&run, "id = ?", ids.run).Error; err != nil || run.ParamsJSON == "" || run.InputsJSON == "" {
		t.Fatalf("unmapped command source changed during rejected purge: %+v, %v", run, err)
	}
	var commandImport models.PluginCommandImport
	if err := db.First(&commandImport, "id = ?", ids.importID).Error; err != nil || commandImport.FieldsJSON == "" {
		t.Fatalf("unmapped import source changed during rejected purge: %+v, %v", commandImport, err)
	}
}

func assertSourceFieldsPurged(t *testing.T, db *gorm.DB, ids replaySourceIDs) {
	t.Helper()
	var download models.DownloadHistoryEntry
	if err := db.First(&download, ids.download).Error; err != nil || len(download.Payload) != 0 || download.URL != "" {
		t.Fatalf("unmapped download source retained replay data: %+v, %v", download, err)
	}
	var scheduled models.ScheduledDownload
	if err := db.First(&scheduled, ids.scheduled).Error; err != nil || len(scheduled.Payload) != 0 || scheduled.URL != "" {
		t.Fatalf("unmapped scheduled source retained replay data: %+v, %v", scheduled, err)
	}
	if ids.fallbackScheduled != 0 {
		assertScheduledReplaySourcePurged(t, db, ids.fallbackScheduled, "unmapped fallback scheduled source retained replay data")
	}
	if ids.shadowedScheduled != 0 {
		assertScheduledReplaySourceIntact(t, db, ids.shadowedScheduled, "purge changed a scheduled row owned by another scheduled handle")
	}
	var run models.PluginCommandRun
	if err := db.First(&run, "id = ?", ids.run).Error; err != nil || run.ParamsJSON != "" || run.InputsJSON != "" {
		t.Fatalf("unmapped command source retained replay data: %+v, %v", run, err)
	}
	var commandImport models.PluginCommandImport
	if err := db.First(&commandImport, "id = ?", ids.importID).Error; err != nil || commandImport.FieldsJSON != "" {
		t.Fatalf("unmapped import source retained replay data: %+v, %v", commandImport, err)
	}
}

func assertScheduledReplaySourceIntact(t *testing.T, db *gorm.DB, id uint, message string) {
	t.Helper()
	var scheduled models.ScheduledDownload
	if err := db.First(&scheduled, id).Error; err != nil || len(scheduled.Payload) == 0 || scheduled.URL == "" {
		t.Fatalf("%s: %+v, %v", message, scheduled, err)
	}
}

func assertScheduledReplaySourcePurged(t *testing.T, db *gorm.DB, id uint, message string) {
	t.Helper()
	var scheduled models.ScheduledDownload
	if err := db.First(&scheduled, id).Error; err != nil || len(scheduled.Payload) != 0 || scheduled.URL != "" {
		t.Fatalf("%s: %+v, %v", message, scheduled, err)
	}
}
