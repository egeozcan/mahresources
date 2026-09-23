package application_context

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"mahresources/jobs"
	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const jobMigrationReadinessBatchSize = 200

// JobMigrationReadiness is a secret-free summary suitable for startup logs and
// the later administrator cutover gate. Counts are grouped by fixed source kind,
// mapping status, and safe blocker code; source IDs and replay values are never
// included.
type JobMigrationReadiness struct {
	Ready        bool                        `json:"ready"`
	WriterEpoch  uint64                      `json:"writerEpoch"`
	Phase        string                      `json:"phase"`
	SourceCounts map[string]map[string]int64 `json:"sourceCounts"`
	Blockers     map[string]int64            `json:"blockers"`
}

// GetJobMigrationReadiness recomputes the retirement gate from the current
// stores. A completed checkpoint alone is not proof: the check re-reads every
// retained mapped source, looks for unmapped source rows, and opens execution
// input for every nonterminal replayable Job, including Jobs without a legacy
// source mapping.
func (ctx *MahresourcesContext) GetJobMigrationReadiness() (JobMigrationReadiness, error) {
	if ctx == nil || ctx.db == nil {
		return JobMigrationReadiness{}, errors.New("job migration readiness requires a database")
	}
	var report JobMigrationReadiness
	report.SourceCounts = make(map[string]map[string]int64, len(jobMigrationSourceKinds))
	report.Blockers = make(map[string]int64)
	report.Phase = models.JobMigrationPhaseCopy
	for _, required := range []any{
		&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{},
		&models.Job{}, &models.JobReplayEnvelope{}, &models.JobLegacyHandle{},
		&models.DownloadHistoryEntry{}, &models.ScheduledDownload{}, &models.PluginCommandRun{},
		&models.PluginCommandImport{}, &models.ResourceReduction{},
	} {
		if !ctx.db.Migrator().HasTable(required) {
			report.Blockers["migration-schema-missing"]++
			report.Ready = false
			return report, nil
		}
	}
	epoch, err := models.JobWriterEpochMinimum(ctx.db)
	if err != nil {
		return report, errors.New("job migration writer epoch could not be read")
	}
	report.WriterEpoch = epoch
	var checkpoint models.JobMigrationCheckpoint
	if err := ctx.db.Where("id = ?", models.JobMigrationCheckpointRowID).First(&checkpoint).Error; err == nil {
		report.Phase = safeJobMigrationPhase(checkpoint.Phase)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return report, errors.New("job migration checkpoint could not be read")
	}
	if epoch < models.JobWriterEpochRetiredPlaintext {
		report.Blockers["writer-epoch-not-retired"]++
	}
	if report.Phase != models.JobMigrationPhaseComplete {
		report.Blockers["migration-incomplete"]++
	}
	barriersInstalled, err := retiredSourceBarriersInstalled(ctx.db)
	if err != nil {
		return report, errors.New("job migration source barriers could not be checked")
	}
	if !barriersInstalled {
		report.Blockers["source-write-barrier-missing"]++
	}

	if err := ctx.db.Transaction(func(tx *gorm.DB) error {
		var groups []struct {
			SourceKind  string
			Status      string
			BlockerCode string
			Count       int64
		}
		if err := tx.Model(&models.JobSourceMapping{}).
			Select("source_kind, status, blocker_code, count(*) AS count").
			Group("source_kind, status, blocker_code").Scan(&groups).Error; err != nil {
			return errors.New("job migration mapping counts could not be read")
		}
		for _, group := range groups {
			kind, status := safeJobMigrationKind(group.SourceKind), safeJobMigrationStatus(group.Status)
			if report.SourceCounts[kind] == nil {
				report.SourceCounts[kind] = make(map[string]int64)
			}
			report.SourceCounts[kind][status] += group.Count
			if kind == "other" {
				report.Blockers["unknown-source-kind"] += group.Count
			}
			if status == "other" {
				report.Blockers["unknown-source-status"] += group.Count
			}
			if group.Status == models.JobSourceMappingQuarantined {
				code := safeJobMigrationBlockerCode(group.BlockerCode)
				report.Blockers["source-quarantined/"+kind+"/"+code] += group.Count
			}
		}

		for _, source := range jobMigrationReadinessSources() {
			unmapped, err := countUnmappedJobMigrationSourceRows(tx, source)
			if err != nil {
				return errors.New("job migration unmapped source count could not be read")
			}
			if unmapped > 0 {
				report.Blockers["source-unmapped/"+safeJobMigrationKind(source.kind)] += unmapped
			}
		}

		for _, kind := range jobMigrationSourceKinds {
			var cursor string
			for {
				var mappings []models.JobSourceMapping
				query := tx.Where("source_kind = ?", kind).Order("source_id ASC").Limit(jobMigrationReadinessBatchSize)
				if cursor != "" {
					query = query.Where("source_id > ?", cursor)
				}
				if err := query.Find(&mappings).Error; err != nil {
					return errors.New("job migration source mappings could not be verified")
				}
				for _, mapping := range mappings {
					if mapping.Status != models.JobSourceMappingScrubbed && mapping.Status != models.JobSourceMappingPurged {
						report.Blockers["source-not-retired/"+safeJobMigrationKind(kind)]++
						continue
					}
					if mapping.ScrubbedAt == nil {
						report.Blockers["source-scrub-marker-missing/"+safeJobMigrationKind(kind)]++
					} else if mapping.Status == models.JobSourceMappingPurged {
						if mapping.PurgedAt == nil || mapping.PurgeReason == "" {
							report.Blockers["purge-marker-missing/"+safeJobMigrationKind(kind)]++
						} else if mapping.PurgeReason != models.JobReplayPurgeForgotten && mapping.PurgeReason != models.JobReplayPurgeExpired {
							report.Blockers["purge-reason-invalid/"+safeJobMigrationKind(kind)]++
						}
						needsScrub, exists, err := retiredSourceNeedsScrub(tx, kind, mapping.SourceID)
						if err != nil {
							return errors.New("job migration purged source could not be checked")
						}
						if exists && needsScrub {
							report.Blockers["purged-source-copy-remains/"+safeJobMigrationKind(kind)]++
						}
					} else if mapping.PostScrubHash == "" {
						report.Blockers["source-scrub-hash-missing/"+safeJobMigrationKind(kind)]++
					} else {
						exists, currentHash, err := currentRetiredSourceHash(tx, kind, mapping.SourceID)
						if err != nil {
							return errors.New("job migration retired source could not be checked")
						}
						if exists && currentHash != mapping.PostScrubHash {
							report.Blockers["source-retirement-hash-mismatch/"+safeJobMigrationKind(kind)]++
						}
					}
					if mapping.JobID != "" && !migrationJobReplayReady(tx, ctx.JobService(), ctx.jobDepsWithDB(tx), kind, mapping.JobID) {
						report.Blockers["nonterminal-replay-unavailable/"+safeJobMigrationKind(kind)]++
					}
				}
				if len(mappings) < jobMigrationReadinessBatchSize {
					break
				}
				cursor = mappings[len(mappings)-1].SourceID
			}
		}

		var cursor string
		for {
			var canonicalOnly []struct{ ID string }
			query := tx.Model(&models.Job{}).
				Select("id").
				Where("replay_class = ?", jobs.ReplayClassReplayable).
				Where("state IN ?", []string{
					string(jobs.StateScheduled), string(jobs.StateQueued), string(jobs.StateRunning),
					string(jobs.StatePaused), string(jobs.StateBlocked),
				}).
				Where("NOT EXISTS (SELECT 1 FROM job_source_mappings AS mapping WHERE mapping.job_id = jobs.id)").
				Order("id ASC").Limit(jobMigrationReadinessBatchSize)
			if cursor != "" {
				query = query.Where("id > ?", cursor)
			}
			if err := query.Find(&canonicalOnly).Error; err != nil {
				return errors.New("job migration canonical replay candidates could not be read")
			}
			for _, job := range canonicalOnly {
				if !canonicalJobReplayReady(ctx.JobService(), ctx.jobDepsWithDB(tx), job.ID) {
					report.Blockers["nonterminal-replay-unavailable/canonical"]++
				}
			}
			if len(canonicalOnly) < jobMigrationReadinessBatchSize {
				break
			}
			cursor = canonicalOnly[len(canonicalOnly)-1].ID
		}
		return nil
	}); err != nil {
		return report, err
	}
	report.Ready = len(report.Blockers) == 0
	return report, nil
}

func retiredSourceBarriersInstalled(db *gorm.DB) (bool, error) {
	var count int64
	if db.Dialector.Name() == "postgres" {
		err := db.Raw(`SELECT count(*) FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
			WHERE NOT t.tgisinternal AND t.tgname IN ?`, []string{
			"retired_job_source_plaintext_download_history_entries",
			"retired_job_source_plaintext_scheduled_downloads",
			"retired_job_source_plaintext_plugin_command_runs",
			"retired_job_source_plaintext_plugin_command_imports",
		}).Scan(&count).Error
		return count == 4, err
	}
	if db.Dialector.Name() != "sqlite" {
		return false, nil
	}
	err := db.Raw(`SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND name IN ?`, []string{
		"job_barrier_download_history_insert", "job_barrier_download_history_update",
		"job_barrier_scheduled_download_insert", "job_barrier_scheduled_download_update",
		"job_barrier_command_run_insert", "job_barrier_command_run_update",
		"job_barrier_command_import_insert", "job_barrier_command_import_update",
	}).Scan(&count).Error
	return count == 8, err
}

// retiredSourceNeedsScrub reports whether a mapped legacy row still carries
// replay material or a URL outside the conservative display projection. Purged
// sources do not retain a post-scrub hash because the purge marker, rather than
// a replay-derived checksum, is the durable authority for their empty input.
func retiredSourceNeedsScrub(db *gorm.DB, kind, sourceID string) (bool, bool, error) {
	switch kind {
	case jobMigrationDownloadHistory:
		id, err := strconv.ParseUint(sourceID, 10, 64)
		if err != nil {
			return false, false, errors.New("invalid source id")
		}
		var row models.DownloadHistoryEntry
		err = db.First(&row, uint(id)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, false, nil
		}
		return len(row.Payload) > 0 || row.URL != downloadURLProjection(row.URL), err == nil, err
	case jobMigrationScheduledDownload:
		id, err := strconv.ParseUint(sourceID, 10, 64)
		if err != nil {
			return false, false, errors.New("invalid source id")
		}
		var row models.ScheduledDownload
		err = db.First(&row, uint(id)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, false, nil
		}
		return len(row.Payload) > 0 || row.URL != downloadURLProjection(row.URL), err == nil, err
	case jobMigrationPluginCommandRun:
		var row models.PluginCommandRun
		err := db.Where("id = ?", sourceID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, false, nil
		}
		return row.ParamsJSON != "" || row.InputsJSON != "", err == nil, err
	case jobMigrationPluginCommandImport:
		var row models.PluginCommandImport
		err := db.Where("id = ?", sourceID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, false, nil
		}
		return row.FieldsJSON != "", err == nil, err
	case jobMigrationReduction:
		id, err := strconv.ParseUint(sourceID, 10, 64)
		if err != nil {
			return false, false, errors.New("invalid source id")
		}
		var row models.ResourceReduction
		err = db.First(&row, uint(id)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, false, nil
		}
		return false, err == nil, err
	default:
		return false, false, nil
	}
}

type jobMigrationReadinessSource struct {
	kind  string
	table string
}

func jobMigrationReadinessSources() []jobMigrationReadinessSource {
	return []jobMigrationReadinessSource{
		{kind: jobMigrationDownloadHistory, table: "download_history_entries"},
		{kind: jobMigrationScheduledDownload, table: "scheduled_downloads"},
		{kind: jobMigrationPluginCommandRun, table: "plugin_command_runs"},
		{kind: jobMigrationPluginCommandImport, table: "plugin_command_imports"},
		{kind: jobMigrationReduction, table: "resource_reductions"},
	}
}

func countUnmappedJobMigrationSourceRows(tx *gorm.DB, source jobMigrationReadinessSource) (int64, error) {
	query := tx.Table(source.table).
		Joins("LEFT JOIN job_source_mappings AS mapping ON mapping.source_kind = ? AND mapping.source_id = CAST("+source.table+".id AS TEXT)", source.kind).
		Where("mapping.source_id IS NULL")
	if source.kind == jobMigrationReduction {
		query = query.Where("resource_reductions.status = ? AND resource_reductions.compute_job_id <> '' AND resource_reductions.computed_at IS NOT NULL AND EXISTS (SELECT 1 FROM job_legacy_handles WHERE job_legacy_handles.namespace = ? AND job_legacy_handles.handle = resource_reductions.compute_job_id)",
			models.ReductionStatusReady, ReductionComputeHandleNamespace)
	}
	var count int64
	return count, query.Count(&count).Error
}

func currentRetiredSourceHash(db *gorm.DB, kind, sourceID string) (bool, string, error) {
	switch kind {
	case jobMigrationDownloadHistory:
		id, err := strconv.ParseUint(sourceID, 10, 64)
		if err != nil {
			return false, "", nil
		}
		var row models.DownloadHistoryEntry
		err = db.First(&row, uint(id)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "", nil
		}
		return err == nil, hashRetiredDownloadHistory(row), err
	case jobMigrationScheduledDownload:
		id, err := strconv.ParseUint(sourceID, 10, 64)
		if err != nil {
			return false, "", nil
		}
		var row models.ScheduledDownload
		err = db.First(&row, uint(id)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "", nil
		}
		return err == nil, hashRetiredScheduledDownload(row), err
	case jobMigrationPluginCommandRun:
		var row models.PluginCommandRun
		err := db.Where("id = ?", sourceID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "", nil
		}
		return err == nil, hashRetiredPluginCommandRun(row), err
	case jobMigrationPluginCommandImport:
		var row models.PluginCommandImport
		err := db.Where("id = ?", sourceID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "", nil
		}
		return err == nil, hashRetiredPluginCommandImport(row), err
	case jobMigrationReduction:
		id, err := strconv.ParseUint(sourceID, 10, 64)
		if err != nil {
			return false, "", nil
		}
		var row models.ResourceReduction
		err = db.First(&row, uint(id)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "", nil
		}
		return err == nil, hashReductionExecution(row), err
	default:
		return false, "", errors.New("unknown job migration source")
	}
}

func totalMigrationReadinessBlockers(report JobMigrationReadiness) int {
	total := 0
	for _, count := range report.Blockers {
		if count > int64(^uint(0)>>1)-int64(total) {
			return int(^uint(0) >> 1)
		}
		total += int(count)
	}
	return total
}

// rearmOneRestoredSource detects one source whose scrubbed safety projection was
// restored from a pre-retirement backup. It re-proves ordinary input against the
// canonical envelope before making the source eligible for scrub again. A purge
// marker wins and schedules immediate deletion without reopening or recreating
// the purged input.
func (ctx *MahresourcesContext) rearmOneRestoredSource(now time.Time) (string, bool, error) {
	for _, kind := range jobMigrationSourceKinds {
		var cursor string
		for {
			var mappings []models.JobSourceMapping
			query := ctx.db.Where("source_kind = ? AND status IN ?", kind,
				[]string{models.JobSourceMappingScrubbed, models.JobSourceMappingPurged}).
				Order("source_id ASC").Limit(jobMigrationReadinessBatchSize)
			if cursor != "" {
				query = query.Where("source_id > ?", cursor)
			}
			if err := query.Find(&mappings).Error; err != nil {
				return "", false, errors.New("job migration restored source scan failed")
			}
			for _, candidate := range mappings {
				needsRepair := false
				var err error
				if candidate.Status == models.JobSourceMappingPurged {
					needsRepair, _, err = retiredSourceNeedsScrub(ctx.db, kind, candidate.SourceID)
				} else {
					var exists bool
					var currentProjection string
					exists, currentProjection, err = currentRetiredSourceHash(ctx.db, kind, candidate.SourceID)
					needsRepair = exists && currentProjection != candidate.PostScrubHash
				}
				if err != nil {
					return "", false, errors.New("job migration restored source check failed")
				}
				if !needsRepair {
					continue
				}
				var rearmed bool
				err = ctx.db.Transaction(func(tx *gorm.DB) error {
					var mapping models.JobSourceMapping
					if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source_kind = ? AND source_id = ?", kind, candidate.SourceID).First(&mapping).Error; err != nil {
						return errors.New("job migration restored mapping disappeared")
					}
					if mapping.Status != models.JobSourceMappingScrubbed && mapping.Status != models.JobSourceMappingPurged {
						return nil
					}
					if mapping.Status == models.JobSourceMappingPurged {
						needsRepair, _, err := retiredSourceNeedsScrub(tx, kind, mapping.SourceID)
						if err != nil {
							return errors.New("job migration restored purged source could not be rechecked")
						}
						if !needsRepair {
							return nil
						}
						mapping.ScrubbedAt, mapping.PostScrubHash, mapping.UpdatedAt = nil, "", now
						if err := tx.Save(&mapping).Error; err != nil {
							return errors.New("job migration purge scrub marker could not be reset")
						}
						rearmed = true
						return nil
					}
					exists, currentProjection, err := currentRetiredSourceHash(tx, kind, mapping.SourceID)
					if err != nil {
						return errors.New("job migration restored source could not be rechecked")
					}
					if !exists || currentProjection == mapping.PostScrubHash {
						return nil
					}
					fullHash, valid, err := verifyRestoredMigrationSource(ctx, tx, kind, mapping)
					if err != nil {
						mapping.Status, mapping.BlockerCode, mapping.UpdatedAt = models.JobSourceMappingQuarantined, "restored-source-not-proven", now
						if saveErr := tx.Save(&mapping).Error; saveErr != nil {
							return errors.New("job migration restored source blocker could not be saved")
						}
						rearmed = true
						return nil
					}
					if !valid {
						return nil
					}
					mapping.SourceRevision++
					if mapping.SourceRevision == 0 {
						mapping.SourceRevision = 1
					}
					mapping.SourceHash, mapping.Status, mapping.BlockerCode = fullHash, models.JobSourceMappingVerified, ""
					mapping.VerifiedAt, mapping.ScrubbedAt, mapping.PostScrubHash = &now, nil, ""
					mapping.CopiedAt, mapping.UpdatedAt = now, now
					if err := tx.Save(&mapping).Error; err != nil {
						return errors.New("job migration restored source verification could not be saved")
					}
					rearmed = true
					return nil
				})
				if err != nil {
					return "", false, err
				}
				if rearmed {
					return kind, true, nil
				}
			}
			if len(mappings) < jobMigrationReadinessBatchSize {
				break
			}
			cursor = mappings[len(mappings)-1].SourceID
		}
	}
	return "", false, nil
}

func verifyRestoredMigrationSource(ctx *MahresourcesContext, tx *gorm.DB, kind string, mapping models.JobSourceMapping) (string, bool, error) {
	switch kind {
	case jobMigrationDownloadHistory:
		id, err := strconv.ParseUint(mapping.SourceID, 10, 64)
		if err != nil {
			return "", false, errors.New("invalid source id")
		}
		var row models.DownloadHistoryEntry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, uint(id)).Error; err != nil {
			return "", false, err
		}
		if err := ctx.verifyDownloadReplay(tx, mapping.JobID, row); err != nil {
			return "", false, err
		}
		return hashDownloadHistory(row), true, nil
	case jobMigrationScheduledDownload:
		id, err := strconv.ParseUint(mapping.SourceID, 10, 64)
		if err != nil {
			return "", false, errors.New("invalid source id")
		}
		var row models.ScheduledDownload
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, uint(id)).Error; err != nil {
			return "", false, err
		}
		if err := ctx.verifyScheduledDownloadReplay(tx, mapping.JobID, row); err != nil {
			return "", false, err
		}
		return hashScheduledDownload(row), true, nil
	case jobMigrationPluginCommandRun:
		var row models.PluginCommandRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", mapping.SourceID).First(&row).Error; err != nil {
			return "", false, err
		}
		if err := ctx.verifyPluginCommandRunReplay(tx, mapping.JobID, row); err != nil {
			return "", false, err
		}
		return hashPluginCommandRun(row), true, nil
	case jobMigrationPluginCommandImport:
		var row models.PluginCommandImport
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", mapping.SourceID).First(&row).Error; err != nil {
			return "", false, err
		}
		if err := ctx.verifyPluginCommandImportReplay(tx, mapping.JobID, row); err != nil {
			return "", false, err
		}
		return hashPluginCommandImport(row), true, nil
	case jobMigrationReduction:
		id, err := strconv.ParseUint(mapping.SourceID, 10, 64)
		if err != nil {
			return "", false, errors.New("invalid source id")
		}
		var row models.ResourceReduction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, uint(id)).Error; err != nil {
			return "", false, err
		}
		if err := verifyReductionJob(tx, ctx.JobService(), ctx.jobDepsWithDB(tx), mapping.JobID, row, false); err != nil {
			return "", false, err
		}
		return hashReductionExecution(row), true, nil
	default:
		return "", false, errors.New("unknown source kind")
	}
}

func migrationJobReplayReady(db *gorm.DB, service *jobs.Service, deps jobs.Deps, kind, jobID string) bool {
	var job models.Job
	if err := db.Where("id = ?", jobID).First(&job).Error; err != nil {
		return false
	}
	if jobs.State(job.State).Terminal() || jobs.ReplayClass(job.ReplayClass) == jobs.ReplayClassNonReplayable {
		return true
	}
	if service == nil {
		return false
	}
	opened, err := service.OpenReplay(deps, jobs.Access{Administrator: true}, jobID)
	if err != nil || len(opened.Input) == 0 {
		return false
	}
	switch kind {
	case jobMigrationDownloadHistory, jobMigrationScheduledDownload:
		var input downloadJobInput
		return json.Unmarshal(opened.Input, &input) == nil && input.Creator != nil && input.Creator.URL != ""
	case jobMigrationPluginCommandRun:
		var input pluginCommandRunReplayInput
		return json.Unmarshal(opened.Input, &input) == nil && input.PluginName != "" && input.CommandName != "" &&
			(input.ParamsJSON == "" || json.Valid([]byte(input.ParamsJSON))) &&
			(input.InputsJSON == "" || json.Valid([]byte(input.InputsJSON)))
	case jobMigrationPluginCommandImport:
		var input pluginCommandImportReplayInput
		return json.Unmarshal(opened.Input, &input) == nil && input.RunID != "" && input.FileName != "" && json.Valid([]byte(input.FieldsJSON))
	case jobMigrationReduction:
		_, err := reductionComputeInputOf(opened.Input)
		return err == nil
	default:
		return false
	}
}

// canonicalJobReplayReady checks a canonical-only Job's required execution
// input through the same authenticated decrypt and Kind decoder dispatch uses.
// The opened bytes are used only as a success signal and cleared before return;
// the readiness report contains neither input nor Job identity.
func canonicalJobReplayReady(service *jobs.Service, deps jobs.Deps, jobID string) bool {
	if service == nil {
		return false
	}
	opened, err := service.OpenReplay(deps, jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return false
	}
	readable := len(opened.Input) != 0
	for i := range opened.Input {
		opened.Input[i] = 0
	}
	return readable
}

func safeJobMigrationKind(kind string) string {
	for _, known := range jobMigrationSourceKinds {
		if kind == known {
			return known
		}
	}
	return "other"
}

func safeJobMigrationStatus(status string) string {
	switch status {
	case models.JobSourceMappingCopied, models.JobSourceMappingVerified, models.JobSourceMappingScrubbed,
		models.JobSourceMappingPurged, models.JobSourceMappingQuarantined:
		return status
	default:
		return "other"
	}
}

func safeJobMigrationPhase(phase string) string {
	switch phase {
	case models.JobMigrationPhaseCopy, models.JobMigrationPhaseVerify, models.JobMigrationPhaseDrainFence,
		models.JobMigrationPhaseScrub, models.JobMigrationPhaseComplete:
		return phase
	default:
		return "unknown"
	}
}

func safeJobMigrationBlockerCode(code string) string {
	// Persisted codes are selected from these fixed migration branches. Any other
	// value is summarized without echoing data from the database.
	switch code {
	case "payload-unreadable", "execution-url-missing", "source-time-missing", "outcome-time-unproven",
		"submitted-job-unmapped", "outcome-unknown", "canonical-replay-unavailable", "source-id-invalid",
		"source-row-missing", "canonical-handle-missing", "source-canonical-replay-mismatch",
		"input-not-encodable", "canonical-job-import-failed", "source-input-unreadable",
		"source-input-changed-after-copy", "canonical-replay-mismatch", "running-source-without-canonical-job",
		"reduction-outcome-unprovable", "reduction-handle-missing":
		return code
	default:
		return "other"
	}
}
