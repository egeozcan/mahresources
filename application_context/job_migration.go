package application_context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const jobMigrationDownloadHistory = "download-history"
const jobMigrationScheduledDownload = "scheduled-download"
const jobMigrationReduction = "resource-reduction"

var jobMigrationSourceKinds = []string{
	jobMigrationDownloadHistory, jobMigrationScheduledDownload,
	jobMigrationPluginCommandRun, jobMigrationPluginCommandImport, jobMigrationReduction,
}

type jobMigrationBlockerError struct {
	sourceKind string
	sourceID   string
	code       string
}

func (e *jobMigrationBlockerError) Error() string {
	return fmt.Sprintf("job migration source %s/%s is blocked (%s)", e.sourceKind, e.sourceID, e.code)
}

func migrationBlocker(kind, id, code string) error {
	return &jobMigrationBlockerError{sourceKind: kind, sourceID: id, code: code}
}

// JobMigrationOptions bounds one startup invocation. WritersDrained is an
// explicit operator attestation that every pre-fence process has stopped; an
// unset value allows copy and verification to proceed but keeps epoch 1 and all
// legacy input intact.
type JobMigrationOptions struct {
	BatchSize      int
	MaxBatches     int
	WritersDrained bool
	Now            func() time.Time
}

// JobMigrationResult reports the durable phase reached by this bounded pass.
type JobMigrationResult struct {
	Phase          string
	Batches        int
	Complete       bool
	BlockedSources int
}

func (ctx *MahresourcesContext) legacyJobInputsRetired() bool {
	if ctx == nil || ctx.db == nil {
		return false
	}
	epoch, err := models.JobWriterEpochMinimum(ctx.db)
	return err == nil && epoch >= models.JobWriterEpochRetiredPlaintext
}

func (ctx *MahresourcesContext) recordDualPublishedDownloadHistory(row models.DownloadHistoryEntry, scrubbed bool, now time.Time) error {
	if ctx == nil || ctx.db == nil || ctx.JobService() == nil {
		return nil
	}
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		return ctx.recordDualPublishedDownloadHistoryTx(tx, row, scrubbed, now)
	})
}

func (ctx *MahresourcesContext) recordDualPublishedDownloadHistoryTx(tx *gorm.DB, row models.DownloadHistoryEntry, scrubbed bool, now time.Time) error {
	var handle models.JobLegacyHandle
	if err := tx.Where("namespace = ? AND handle = ?", DownloadHandleNamespace, row.JobID).First(&handle).Error; err != nil {
		return errors.New("download history canonical handle is unavailable")
	}
	return ctx.recordDualPublishedSourceTx(tx, jobMigrationDownloadHistory, strconv.FormatUint(uint64(row.ID), 10), handle.JobID,
		hashDownloadHistory(row), hashRetiredDownloadHistory(row), scrubbed, now)
}

func (ctx *MahresourcesContext) recordDualPublishedScheduledDownload(row models.ScheduledDownload, scrubbed bool, now time.Time) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		return ctx.recordDualPublishedScheduledDownloadTx(tx, row, scrubbed, now)
	})
}

func (ctx *MahresourcesContext) recordDualPublishedScheduledDownloadTx(tx *gorm.DB, row models.ScheduledDownload, scrubbed bool, now time.Time) error {
	var handle models.JobLegacyHandle
	if err := tx.Where("namespace = ? AND handle = ?", ScheduledDownloadHandleNamespace, strconv.FormatUint(uint64(row.ID), 10)).First(&handle).Error; err != nil {
		return errors.New("scheduled download canonical handle is unavailable")
	}
	return ctx.recordDualPublishedSourceTx(tx, jobMigrationScheduledDownload, strconv.FormatUint(uint64(row.ID), 10), handle.JobID,
		hashScheduledDownload(row), hashRetiredScheduledDownload(row), scrubbed, now)
}

// refreshChangedDownloadHistoryMapping and refreshChangedScheduledDownloadMapping
// recover a source mutation from an older dual-publisher that did not know about
// JobSourceMapping. They only advance the source revision after the current
// canonical handle still opens to the exact execution input in the source row.
// A purge marker wins and is carried forward without recreating replay input.
func (ctx *MahresourcesContext) refreshChangedDownloadHistoryMapping(tx *gorm.DB, mapping *models.JobSourceMapping, now time.Time) error {
	if tx == nil || mapping == nil {
		return errors.New("download history mapping is unavailable")
	}
	var updated models.JobSourceMapping
	err := func() error {
		var current models.JobSourceMapping
		if err := tx.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, mapping.SourceID).First(&current).Error; err != nil {
			return errors.New("download history mapping disappeared during source refresh")
		}
		if current.Status == models.JobSourceMappingPurged || current.Status == models.JobSourceMappingScrubbed {
			updated = current
			return nil
		}
		id, err := strconv.ParseUint(current.SourceID, 10, 64)
		if err != nil {
			return migrationBlocker(jobMigrationDownloadHistory, current.SourceID, "source-id-invalid")
		}
		var row models.DownloadHistoryEntry
		if err := tx.First(&row, uint(id)).Error; err != nil {
			return migrationBlocker(jobMigrationDownloadHistory, current.SourceID, "source-row-missing")
		}
		hash := hashDownloadHistory(row)
		if current.SourceHash == hash {
			updated = current
			return nil
		}
		var handle models.JobLegacyHandle
		if err := tx.Where("namespace = ? AND handle = ?", DownloadHandleNamespace, row.JobID).First(&handle).Error; err != nil {
			return migrationBlocker(jobMigrationDownloadHistory, current.SourceID, "canonical-handle-missing")
		}
		current.JobID, current.Origin = handle.JobID, models.JobSourceOriginDualPublished
		purged, reason, err := ctx.downloadReplayPurged(tx, handle.JobID, now)
		if err != nil {
			return migrationBlocker(jobMigrationDownloadHistory, current.SourceID, "canonical-replay-unavailable")
		}
		if purged {
			at := now
			current.Status, current.PurgedAt, current.PurgeReason = models.JobSourceMappingPurged, &at, reason
			current.ScrubbedAt, current.PostScrubHash = nil, ""
		} else if err := ctx.verifyDownloadReplay(tx, handle.JobID, row); err != nil {
			return migrationBlocker(jobMigrationDownloadHistory, current.SourceID, "source-canonical-replay-mismatch")
		} else {
			current.Status, current.PurgedAt, current.PurgeReason = models.JobSourceMappingCopied, nil, ""
			current.VerifiedAt, current.ScrubbedAt, current.PostScrubHash = nil, nil, ""
		}
		current.SourceRevision++
		if current.SourceRevision == 0 {
			current.SourceRevision = 1
		}
		current.SourceHash, current.CopiedAt, current.UpdatedAt = hash, now, now
		current.BlockerCode = ""
		if err := tx.Save(&current).Error; err != nil {
			return errors.New("download history source revision could not be stored")
		}
		updated = current
		return nil
	}()
	if err == nil {
		*mapping = updated
	}
	return err
}

func (ctx *MahresourcesContext) refreshChangedScheduledDownloadMapping(tx *gorm.DB, mapping *models.JobSourceMapping, now time.Time) error {
	if tx == nil || mapping == nil {
		return errors.New("scheduled download mapping is unavailable")
	}
	var updated models.JobSourceMapping
	err := func() error {
		var current models.JobSourceMapping
		if err := tx.Where("source_kind = ? AND source_id = ?", jobMigrationScheduledDownload, mapping.SourceID).First(&current).Error; err != nil {
			return errors.New("scheduled download mapping disappeared during source refresh")
		}
		if current.Status == models.JobSourceMappingPurged || current.Status == models.JobSourceMappingScrubbed {
			updated = current
			return nil
		}
		id, err := strconv.ParseUint(current.SourceID, 10, 64)
		if err != nil {
			return migrationBlocker(jobMigrationScheduledDownload, current.SourceID, "source-id-invalid")
		}
		var row models.ScheduledDownload
		if err := tx.First(&row, uint(id)).Error; err != nil {
			return migrationBlocker(jobMigrationScheduledDownload, current.SourceID, "source-row-missing")
		}
		hash := hashScheduledDownload(row)
		if current.SourceHash == hash {
			updated = current
			return nil
		}
		var handle models.JobLegacyHandle
		err = tx.Where("namespace = ? AND handle = ?", ScheduledDownloadHandleNamespace, current.SourceID).First(&handle).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && row.JobID != "" {
			err = tx.Where("namespace = ? AND handle = ?", DownloadHandleNamespace, row.JobID).First(&handle).Error
		}
		if err != nil {
			return migrationBlocker(jobMigrationScheduledDownload, current.SourceID, "canonical-handle-missing")
		}
		current.JobID, current.Origin = handle.JobID, models.JobSourceOriginDualPublished
		purged, reason, err := ctx.downloadReplayPurged(tx, handle.JobID, now)
		if err != nil {
			return migrationBlocker(jobMigrationScheduledDownload, current.SourceID, "canonical-replay-unavailable")
		}
		if purged {
			at := now
			current.Status, current.PurgedAt, current.PurgeReason = models.JobSourceMappingPurged, &at, reason
			current.ScrubbedAt, current.PostScrubHash = nil, ""
		} else if err := ctx.verifyScheduledDownloadReplay(tx, handle.JobID, row); err != nil {
			return migrationBlocker(jobMigrationScheduledDownload, current.SourceID, "source-canonical-replay-mismatch")
		} else {
			current.Status, current.PurgedAt, current.PurgeReason = models.JobSourceMappingCopied, nil, ""
			current.VerifiedAt, current.ScrubbedAt, current.PostScrubHash = nil, nil, ""
		}
		current.SourceRevision++
		if current.SourceRevision == 0 {
			current.SourceRevision = 1
		}
		current.SourceHash, current.CopiedAt, current.UpdatedAt = hash, now, now
		current.BlockerCode = ""
		if err := tx.Save(&current).Error; err != nil {
			return errors.New("scheduled download source revision could not be stored")
		}
		updated = current
		return nil
	}()
	if err == nil {
		*mapping = updated
	}
	return err
}

func (ctx *MahresourcesContext) recordDualPublishedSource(kind, sourceID, jobID, hash string, scrubbed bool, now time.Time) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		return ctx.recordDualPublishedSourceTx(tx, kind, sourceID, jobID, hash, hash, scrubbed, now)
	})
}

func (ctx *MahresourcesContext) recordDualPublishedSourceTx(tx *gorm.DB, kind, sourceID, jobID, hash, postScrubHash string, scrubbed bool, now time.Time) error {
	var mapping models.JobSourceMapping
	err := tx.Where("source_kind = ? AND source_id = ?", kind, sourceID).First(&mapping).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("source mapping lookup failed")
	}
	if mapping.Status == models.JobSourceMappingPurged || mapping.PurgedAt != nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		mapping = models.JobSourceMapping{SourceKind: kind, SourceID: sourceID, SourceRevision: 1, CreatedAt: now}
	} else {
		mapping.SourceRevision++
		if mapping.SourceRevision == 0 {
			mapping.SourceRevision = 1
		}
	}
	mapping.JobID, mapping.SourceHash = jobID, hash
	mapping.Origin, mapping.BlockerCode = models.JobSourceOriginDualPublished, ""
	mapping.CopiedAt, mapping.UpdatedAt = now, now
	mapping.VerifiedAt, mapping.ScrubbedAt = nil, nil
	mapping.PostScrubHash = ""
	if scrubbed {
		mapping.Status, mapping.ScrubbedAt, mapping.PostScrubHash = models.JobSourceMappingScrubbed, &now, postScrubHash
	} else {
		mapping.Status = models.JobSourceMappingCopied
	}
	return tx.Save(&mapping).Error
}

func (ctx *MahresourcesContext) RunJobMigration(options JobMigrationOptions) (JobMigrationResult, error) {
	if ctx == nil || ctx.db == nil || ctx.JobService() == nil {
		return JobMigrationResult{}, errors.New("job migration requires the database and registered Job service")
	}
	if options.BatchSize <= 0 {
		options.BatchSize = 100
	}
	if options.MaxBatches <= 0 {
		options.MaxBatches = 20
	}
	now := time.Now().UTC
	if options.Now != nil {
		now = func() time.Time { return options.Now().UTC() }
	}
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}, &models.JobSourceMapping{}, &models.JobMigrationCheckpoint{}); err != nil {
		return JobMigrationResult{}, fmt.Errorf("job migration schema: %w", err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		return JobMigrationResult{}, err
	}
	checkpoint, err := ctx.ensureJobMigrationCheckpoint(now())
	if err != nil {
		return JobMigrationResult{}, err
	}
	result := JobMigrationResult{}
	for result.Batches < options.MaxBatches {
		result.Phase = checkpoint.Phase
		switch checkpoint.Phase {
		case models.JobMigrationPhaseCopy:
			more, lastID, err := ctx.copyJobMigrationSourceBatch(checkpoint.SourceKind, checkpoint.CursorID, options.BatchSize, now())
			if err != nil {
				return result, ctx.recordJobMigrationError(err, now())
			}
			result.Batches++
			if more {
				checkpoint.CursorID = lastID
			} else {
				if next, ok := nextJobMigrationSourceKind(checkpoint.SourceKind); ok {
					checkpoint.SourceKind, checkpoint.CursorID = next, ""
				} else {
					checkpoint.Phase, checkpoint.SourceKind, checkpoint.CursorID = models.JobMigrationPhaseVerify, jobMigrationSourceKinds[0], ""
				}
			}
		case models.JobMigrationPhaseVerify:
			more, lastID, err := ctx.verifyJobMigrationSourceBatch(checkpoint.SourceKind, checkpoint.CursorID, options.BatchSize, now(), false)
			if err != nil {
				return result, ctx.recordJobMigrationError(err, now())
			}
			result.Batches++
			if more {
				checkpoint.CursorID = lastID
			} else {
				if next, ok := nextJobMigrationSourceKind(checkpoint.SourceKind); ok {
					checkpoint.SourceKind, checkpoint.CursorID = next, ""
				} else {
					checkpoint.Phase, checkpoint.SourceKind, checkpoint.CursorID = models.JobMigrationPhaseDrainFence, "", ""
				}
			}
		case models.JobMigrationPhaseDrainFence:
			writerEpoch, err := models.JobWriterEpochMinimum(ctx.db)
			if err != nil {
				return result, errors.New("job migration writer epoch could not be read")
			}
			if !options.WritersDrained && writerEpoch < models.JobWriterEpochRetiredPlaintext {
				var blocked int64
				if err := ctx.db.Model(&models.JobSourceMapping{}).Where("status = ?", models.JobSourceMappingQuarantined).Count(&blocked).Error; err != nil {
					return result, errors.New("job migration could not read its quarantine ledger")
				}
				result.BlockedSources = int(blocked)
				if blocked > 0 {
					checkpoint.LastError = "source conversion is quarantined; writer drain and scrub are blocked"
				}
				if err := ctx.saveJobMigrationCheckpoint(checkpoint, now()); err != nil {
					return result, err
				}
				result.Phase = checkpoint.Phase
				return result, nil
			}
			if checkpoint.SourceKind == "" {
				checkpoint.SourceKind, checkpoint.CursorID = jobMigrationSourceKinds[0], ""
				continue
			}
			more, lastID, err := ctx.verifyJobMigrationSourceBatch(checkpoint.SourceKind, checkpoint.CursorID, options.BatchSize, now(), true)
			if err != nil {
				return result, ctx.recordJobMigrationError(err, now())
			}
			result.Batches++
			if more {
				checkpoint.CursorID = lastID
				if err := ctx.saveJobMigrationCheckpoint(checkpoint, now()); err != nil {
					return result, err
				}
				continue
			}
			if next, ok := nextJobMigrationSourceKind(checkpoint.SourceKind); ok {
				checkpoint.SourceKind, checkpoint.CursorID = next, ""
				if err := ctx.saveJobMigrationCheckpoint(checkpoint, now()); err != nil {
					return result, err
				}
				continue
			}
			checkpoint.SourceKind, checkpoint.CursorID = "", ""
			var blocked int64
			if err := ctx.db.Model(&models.JobSourceMapping{}).Where("status = ?", models.JobSourceMappingQuarantined).Count(&blocked).Error; err != nil {
				return result, errors.New("job migration could not read its quarantine ledger")
			}
			result.BlockedSources = int(blocked)
			if blocked > 0 {
				checkpoint.LastError = "source conversion is quarantined; writer drain and scrub are blocked"
				if err := ctx.saveJobMigrationCheckpoint(checkpoint, now()); err != nil {
					return result, err
				}
				result.Phase = checkpoint.Phase
				return result, nil
			}
			unmapped, err := ctx.hasUnmappedJobMigrationSources()
			if err != nil {
				return result, err
			}
			if unmapped {
				checkpoint.Phase, checkpoint.SourceKind, checkpoint.CursorID = models.JobMigrationPhaseCopy, jobMigrationSourceKinds[0], ""
				continue
			}
			if err := ctx.installRetiredPlaintextBarrier(now()); err != nil {
				return result, err
			}
			checkpoint.Phase = models.JobMigrationPhaseScrub
			checkpoint.SourceKind, checkpoint.CursorID = jobMigrationSourceKinds[0], ""
			attested := now()
			checkpoint.WritersDrainAt = &attested
			barrier := now()
			checkpoint.BarrierInstalledAt = &barrier
			result.Batches++
		case models.JobMigrationPhaseScrub:
			more, lastID, err := ctx.scrubJobMigrationSourceBatch(checkpoint.SourceKind, checkpoint.CursorID, options.BatchSize, now())
			if err != nil {
				return result, err
			}
			result.Batches++
			if more {
				checkpoint.CursorID = lastID
			} else {
				if next, ok := nextJobMigrationSourceKind(checkpoint.SourceKind); ok {
					checkpoint.SourceKind, checkpoint.CursorID = next, ""
				} else {
					checkpoint.Phase, checkpoint.SourceKind, checkpoint.CursorID = models.JobMigrationPhaseComplete, "", ""
					completed := now()
					checkpoint.CompletedAt = &completed
				}
			}
		case models.JobMigrationPhaseComplete:
			readiness, err := ctx.GetJobMigrationReadiness()
			if err != nil {
				return result, err
			}
			if readiness.Ready {
				result.Phase, result.Complete = checkpoint.Phase, true
				if err := ctx.saveJobMigrationCheckpoint(checkpoint, now()); err != nil {
					return result, err
				}
				return result, nil
			}
			_, rearmed, err := ctx.rearmOneRestoredSource(now())
			if err != nil {
				return result, err
			}
			if rearmed {
				result.Batches++
				checkpoint.Phase, checkpoint.SourceKind, checkpoint.CursorID = models.JobMigrationPhaseCopy, jobMigrationSourceKinds[0], ""
				checkpoint.CompletedAt = nil
				if err := ctx.saveJobMigrationCheckpoint(checkpoint, now()); err != nil {
					return result, err
				}
				continue
			}
			unmapped, err := ctx.hasUnmappedJobMigrationSources()
			if err != nil {
				return result, errors.New("job migration could not recheck source coverage")
			}
			if unmapped {
				result.Batches++
				checkpoint.Phase, checkpoint.SourceKind, checkpoint.CursorID = models.JobMigrationPhaseCopy, jobMigrationSourceKinds[0], ""
				checkpoint.CompletedAt = nil
				if err := ctx.saveJobMigrationCheckpoint(checkpoint, now()); err != nil {
					return result, err
				}
				continue
			}
			result.Phase, result.Complete = models.JobMigrationPhaseDrainFence, false
			result.BlockedSources = totalMigrationReadinessBlockers(readiness)
			checkpoint.Phase, checkpoint.SourceKind, checkpoint.CursorID = models.JobMigrationPhaseDrainFence, "", ""
			checkpoint.CompletedAt = nil
			if err := ctx.saveJobMigrationCheckpoint(checkpoint, now()); err != nil {
				return result, err
			}
			return result, nil
		default:
			return result, fmt.Errorf("job migration has unknown phase %q", checkpoint.Phase)
		}
		if err := ctx.saveJobMigrationCheckpoint(checkpoint, now()); err != nil {
			return result, err
		}
	}
	result.Phase = checkpoint.Phase
	return result, nil
}

// RunJobMigrationToGate advances the resumable migration in bounded passes.
// A startup may have more sources than one invocation's MaxBatches budget can
// process, so it keeps invoking RunJobMigration until the migration is complete
// or reaches a gate that requires a drain attestation or source repair. Each
// pass commits its own checkpoint; a process crash resumes from that checkpoint.
func (ctx *MahresourcesContext) RunJobMigrationToGate(options JobMigrationOptions) (JobMigrationResult, error) {
	var totalBatches int
	for {
		result, err := ctx.RunJobMigration(options)
		passBatches := result.Batches
		totalBatches += passBatches
		result.Batches = totalBatches
		if err != nil {
			return result, err
		}
		needsExternalDrain := false
		if !options.WritersDrained && result.Phase == models.JobMigrationPhaseDrainFence {
			epoch, err := models.JobWriterEpochMinimum(ctx.db)
			if err != nil {
				return result, errors.New("job migration writer epoch could not be read")
			}
			needsExternalDrain = epoch < models.JobWriterEpochRetiredPlaintext
		}
		if result.Complete || needsExternalDrain ||
			(result.Phase == models.JobMigrationPhaseDrainFence && result.BlockedSources > 0) || passBatches == 0 {
			return result, nil
		}
	}
}

func (ctx *MahresourcesContext) ensureJobMigrationCheckpoint(now time.Time) (models.JobMigrationCheckpoint, error) {
	var checkpoint models.JobMigrationCheckpoint
	err := ctx.db.Where("id = ?", models.JobMigrationCheckpointRowID).First(&checkpoint).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		checkpoint = models.JobMigrationCheckpoint{ID: models.JobMigrationCheckpointRowID, Phase: models.JobMigrationPhaseCopy, SourceKind: jobMigrationDownloadHistory, UpdatedAt: now}
		err = ctx.db.Create(&checkpoint).Error
	}
	return checkpoint, err
}

func (ctx *MahresourcesContext) saveJobMigrationCheckpoint(checkpoint models.JobMigrationCheckpoint, now time.Time) error {
	checkpoint.UpdatedAt = now
	return ctx.db.Save(&checkpoint).Error
}

func (ctx *MahresourcesContext) recordJobMigrationError(err error, now time.Time) error {
	var checkpoint models.JobMigrationCheckpoint
	if loadErr := ctx.db.Where("id = ?", models.JobMigrationCheckpointRowID).First(&checkpoint).Error; loadErr != nil {
		return errors.New("job migration checkpoint is unavailable; details are redacted")
	}
	checkpoint.LastError = "job migration operation failed; details are redacted"
	var blocker *jobMigrationBlockerError
	if errors.As(err, &blocker) {
		checkpoint.LastError = fmt.Sprintf("source %s/%s is quarantined (%s)", blocker.sourceKind, blocker.sourceID, blocker.code)
	}
	checkpoint.UpdatedAt = now
	if saveErr := ctx.db.Save(&checkpoint).Error; saveErr != nil {
		return errors.New("job migration could not persist its safe diagnostic")
	}
	return errors.New(checkpoint.LastError)
}

func (ctx *MahresourcesContext) recordSafeSourceBlocker(blocker *jobMigrationBlockerError, now time.Time) error {
	var checkpoint models.JobMigrationCheckpoint
	if err := ctx.db.Where("id = ?", models.JobMigrationCheckpointRowID).First(&checkpoint).Error; err != nil {
		return errors.New("job migration checkpoint is unavailable")
	}
	checkpoint.LastError = fmt.Sprintf("source %s/%s is quarantined (%s)", blocker.sourceKind, blocker.sourceID, blocker.code)
	checkpoint.UpdatedAt = now
	if err := ctx.db.Save(&checkpoint).Error; err != nil {
		return errors.New("job migration could not persist safe source diagnostics")
	}
	return nil
}

func (ctx *MahresourcesContext) quarantineDownloadHistory(row models.DownloadHistoryEntry, code string, now time.Time) error {
	id := strconv.FormatUint(uint64(row.ID), 10)
	mapping := models.JobSourceMapping{
		SourceKind: jobMigrationDownloadHistory, SourceID: id, SourceRevision: 1,
		SourceHash: hashDownloadHistory(row), Status: models.JobSourceMappingQuarantined,
		BlockerCode: code, Origin: models.JobSourceOriginBackfilled,
		CopiedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	return ctx.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "source_kind"}, {Name: "source_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "blocker_code", "source_hash", "updated_at"}),
	}).Create(&mapping).Error
}

func (ctx *MahresourcesContext) copyDownloadHistoryBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	var after uint
	if cursor != "" {
		parsed, err := strconv.ParseUint(cursor, 10, 64)
		if err != nil {
			return false, cursor, fmt.Errorf("invalid download migration cursor %q", cursor)
		}
		after = uint(parsed)
	}
	var rows []models.DownloadHistoryEntry
	if err := ctx.db.Where("id > ?", after).Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return false, cursor, err
	}
	for _, row := range rows {
		if err := ctx.copyDownloadHistory(row, now); err != nil {
			var blocker *jobMigrationBlockerError
			if !errors.As(err, &blocker) {
				return false, strconv.FormatUint(uint64(row.ID), 10), errors.New("job migration source copy failed; details are redacted")
			}
			if err := ctx.quarantineDownloadHistory(row, blocker.code, now); err != nil {
				return false, strconv.FormatUint(uint64(row.ID), 10), errors.New("job migration could not persist source quarantine")
			}
			if err := ctx.recordSafeSourceBlocker(blocker, now); err != nil {
				return false, strconv.FormatUint(uint64(row.ID), 10), err
			}
		}
	}
	if len(rows) == limit {
		return true, strconv.FormatUint(uint64(rows[len(rows)-1].ID), 10), nil
	}
	return false, "", nil
}

func nextJobMigrationSourceKind(kind string) (string, bool) {
	for i, current := range jobMigrationSourceKinds {
		if current == kind && i+1 < len(jobMigrationSourceKinds) {
			return jobMigrationSourceKinds[i+1], true
		}
	}
	return "", false
}

func (ctx *MahresourcesContext) copyJobMigrationSourceBatch(kind, cursor string, limit int, now time.Time) (bool, string, error) {
	switch kind {
	case jobMigrationDownloadHistory:
		return ctx.copyDownloadHistoryBatch(cursor, limit, now)
	case jobMigrationScheduledDownload:
		return ctx.copyScheduledDownloadsBatch(cursor, limit, now)
	case jobMigrationPluginCommandRun:
		return ctx.copyPluginCommandRunsBatch(cursor, limit, now)
	case jobMigrationPluginCommandImport:
		return ctx.copyPluginCommandImportsBatch(cursor, limit, now)
	case jobMigrationReduction:
		return ctx.copyReductionsBatch(cursor, limit, now)
	default:
		return false, cursor, fmt.Errorf("unknown job migration source kind %q", kind)
	}
}

type scheduledDownloadSourceHash struct {
	ID              uint
	PluginName      string
	URL             string
	Payload         []byte
	DueAt           time.Time
	ClaimToken      string
	ClaimedAt       *time.Time
	Status          string
	JobID           string
	LastError       string
	Attempts        int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CreatedByUserID *uint
}

func hashScheduledDownload(row models.ScheduledDownload) string {
	projection := scheduledDownloadSourceHash{
		ID: row.ID, PluginName: row.PluginName, URL: row.URL, Payload: append([]byte(nil), row.Payload...),
		DueAt: row.DueAt.UTC(), ClaimToken: row.ClaimToken, ClaimedAt: utcTimePtr(row.ClaimedAt),
		Status: row.Status, JobID: row.JobID, LastError: row.LastError, Attempts: row.Attempts,
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(), CreatedByUserID: row.CreatedByUserId,
	}
	encoded, _ := json.Marshal(projection)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func scheduledDownloadCreator(row models.ScheduledDownload) (*query_models.ResourceFromRemoteCreator, error) {
	creator := &query_models.ResourceFromRemoteCreator{}
	if len(row.Payload) > 0 {
		if err := json.Unmarshal(row.Payload, creator); err != nil {
			return nil, migrationBlocker(jobMigrationScheduledDownload, strconv.FormatUint(uint64(row.ID), 10), "payload-unreadable")
		}
	}
	if creator.URL == "" {
		creator.URL = row.URL
	}
	if creator.URL == "" {
		return nil, migrationBlocker(jobMigrationScheduledDownload, strconv.FormatUint(uint64(row.ID), 10), "execution-url-missing")
	}
	return creator, nil
}

func (ctx *MahresourcesContext) copyScheduledDownloadsBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	var after uint
	if cursor != "" {
		parsed, err := strconv.ParseUint(cursor, 10, 64)
		if err != nil {
			return false, cursor, errors.New("scheduled download migration cursor is invalid")
		}
		after = uint(parsed)
	}
	var rows []models.ScheduledDownload
	if err := ctx.db.Where("id > ?", after).Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return false, cursor, errors.New("scheduled download source scan failed")
	}
	for _, row := range rows {
		if err := ctx.copyScheduledDownload(row, now); err != nil {
			var blocker *jobMigrationBlockerError
			if !errors.As(err, &blocker) {
				return false, strconv.FormatUint(uint64(row.ID), 10), errors.New("scheduled download source copy failed; details are redacted")
			}
			if err := ctx.quarantineScheduledDownload(row, blocker.code, now); err != nil {
				return false, strconv.FormatUint(uint64(row.ID), 10), errors.New("scheduled download quarantine could not be persisted")
			}
			if err := ctx.recordSafeSourceBlocker(blocker, now); err != nil {
				return false, strconv.FormatUint(uint64(row.ID), 10), err
			}
		}
	}
	if len(rows) == limit {
		return true, strconv.FormatUint(uint64(rows[len(rows)-1].ID), 10), nil
	}
	return false, "", nil
}

func (ctx *MahresourcesContext) copyScheduledDownload(row models.ScheduledDownload, now time.Time) error {
	if row.CreatedAt.IsZero() || row.DueAt.IsZero() {
		return migrationBlocker(jobMigrationScheduledDownload, strconv.FormatUint(uint64(row.ID), 10), "source-time-missing")
	}
	hash := hashScheduledDownload(row)
	id := strconv.FormatUint(uint64(row.ID), 10)
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		var prior models.JobSourceMapping
		err := tx.Where("source_kind = ? AND source_id = ?", jobMigrationScheduledDownload, id).First(&prior).Error
		if err == nil {
			if prior.Status == models.JobSourceMappingPurged || prior.Status == models.JobSourceMappingScrubbed {
				return nil
			}
			if prior.SourceHash != hash {
				return ctx.refreshChangedScheduledDownloadMapping(tx, &prior, now)
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("scheduled download mapping lookup failed")
		}
		mapping := models.JobSourceMapping{
			SourceKind: jobMigrationScheduledDownload, SourceID: id, SourceRevision: 1,
			SourceHash: hash, Status: models.JobSourceMappingCopied,
			Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		var handle models.JobLegacyHandle
		handleErr := tx.Where("namespace = ? AND handle = ?", ScheduledDownloadHandleNamespace, id).First(&handle).Error
		if errors.Is(handleErr, gorm.ErrRecordNotFound) && row.JobID != "" {
			handleErr = tx.Where("namespace = ? AND handle = ?", DownloadHandleNamespace, row.JobID).First(&handle).Error
		}
		if handleErr == nil {
			mapping.JobID, mapping.Origin = handle.JobID, models.JobSourceOriginDualPublished
			purged, reason, err := ctx.downloadReplayPurged(tx, handle.JobID, now)
			if err != nil {
				return migrationBlocker(jobMigrationScheduledDownload, id, "canonical-replay-unavailable")
			}
			if purged {
				at := now
				mapping.Status, mapping.PurgedAt, mapping.PurgeReason = models.JobSourceMappingPurged, &at, reason
			} else if err := ctx.verifyScheduledDownloadReplay(tx, handle.JobID, row); err != nil {
				mapping.Status, mapping.BlockerCode = models.JobSourceMappingQuarantined, "canonical-replay-mismatch"
				if err := tx.Create(&mapping).Error; err != nil {
					return err
				}
				return nil
			}
			mapping.VerifiedAt = &now
			if mapping.Status == models.JobSourceMappingCopied {
				mapping.Status = models.JobSourceMappingVerified
			}
			return tx.Create(&mapping).Error
		}
		if !errors.Is(handleErr, gorm.ErrRecordNotFound) {
			return errors.New("scheduled download Job handle lookup failed")
		}
		creator, err := scheduledDownloadCreator(row)
		if err != nil {
			return err
		}
		input, err := remoteDownloadInputJSON(creator, row.PluginName)
		if err != nil {
			return migrationBlocker(jobMigrationScheduledDownload, id, "input-not-encodable")
		}
		state, failure, finished, err := scheduledDownloadOutcome(row)
		if err != nil {
			return err
		}
		owner := copyUintPtr(row.CreatedByUserId)
		acceptance := jobs.Acceptance{
			Kind: JobKindDeferredDownload, KindVersion: jobDownloadKindVersion, State: jobs.StateScheduled,
			OwnerUserID: owner, ActorUserID: copyUintPtr(owner), Origin: "schedule",
			Title: downloadJobTitle(input), Replay: jobs.ReplayInput{Input: input},
			ScheduledFor: &row.DueAt, LegacyRefs: []jobs.LegacyRef{{Namespace: ScheduledDownloadHandleNamespace, Handle: id}},
		}
		deps := ctx.jobDepsWithDB(tx)
		deps.Now = func() time.Time { return row.CreatedAt.UTC() }
		if _, err := ctx.JobService().ImportLegacy(deps, jobs.LegacyImport{Acceptance: acceptance, State: state,
			AcceptedAt: row.CreatedAt.UTC(), FinishedAt: finished, Failure: failure}); err != nil {
			return migrationBlocker(jobMigrationScheduledDownload, id, "canonical-job-import-failed")
		}
		var created models.JobLegacyHandle
		if err := tx.Where("namespace = ? AND handle = ?", ScheduledDownloadHandleNamespace, id).First(&created).Error; err != nil {
			return errors.New("scheduled download Job handle was not recorded")
		}
		mapping.JobID = created.JobID
		return tx.Create(&mapping).Error
	})
}

func scheduledDownloadOutcome(row models.ScheduledDownload) (jobs.State, *jobs.Failure, *time.Time, error) {
	switch row.Status {
	case models.ScheduledDownloadStatusPending:
		return jobs.StateScheduled, nil, nil, nil
	case models.ScheduledDownloadStatusFailed:
		if row.UpdatedAt.IsZero() || row.UpdatedAt.Before(row.CreatedAt) {
			return "", nil, nil, migrationBlocker(jobMigrationScheduledDownload, strconv.FormatUint(uint64(row.ID), 10), "outcome-time-unproven")
		}
		finished := row.UpdatedAt.UTC()
		return jobs.StateFailed, &jobs.Failure{Code: "legacy-scheduled-submit-failed", Class: jobs.FailureClassInternal, Message: "scheduled download submission failed"}, &finished, nil
	case models.ScheduledDownloadStatusCancelled:
		if row.UpdatedAt.IsZero() || row.UpdatedAt.Before(row.CreatedAt) {
			return "", nil, nil, migrationBlocker(jobMigrationScheduledDownload, strconv.FormatUint(uint64(row.ID), 10), "outcome-time-unproven")
		}
		finished := row.UpdatedAt.UTC()
		return jobs.StateCancelled, nil, &finished, nil
	case models.ScheduledDownloadStatusSubmitted:
		return "", nil, nil, migrationBlocker(jobMigrationScheduledDownload, strconv.FormatUint(uint64(row.ID), 10), "submitted-job-unmapped")
	default:
		return "", nil, nil, migrationBlocker(jobMigrationScheduledDownload, strconv.FormatUint(uint64(row.ID), 10), "outcome-unknown")
	}
}

func (ctx *MahresourcesContext) verifyScheduledDownloadReplay(db *gorm.DB, jobID string, row models.ScheduledDownload) error {
	creator, err := scheduledDownloadCreator(row)
	if err != nil {
		return err
	}
	expected, err := remoteDownloadInputJSON(creator, row.PluginName)
	if err != nil {
		return err
	}
	return ctx.verifyCanonicalDownloadInput(db, jobID, expected)
}

func (ctx *MahresourcesContext) verifyCanonicalDownloadInput(db *gorm.DB, jobID string, expected json.RawMessage) error {
	opened, err := ctx.JobService().OpenReplay(ctx.jobDepsWithDB(db), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return err
	}
	var got, want downloadJobInput
	if err := json.Unmarshal(opened.Input, &got); err != nil {
		return err
	}
	if err := json.Unmarshal(expected, &want); err != nil {
		return err
	}
	if !sameDownloadJobInput(got, want) {
		return fmt.Errorf("decrypted input does not match source fields")
	}
	return nil
}

func (ctx *MahresourcesContext) quarantineScheduledDownload(row models.ScheduledDownload, code string, now time.Time) error {
	id := strconv.FormatUint(uint64(row.ID), 10)
	mapping := models.JobSourceMapping{SourceKind: jobMigrationScheduledDownload, SourceID: id, SourceRevision: 1,
		SourceHash: hashScheduledDownload(row), Status: models.JobSourceMappingQuarantined, BlockerCode: code,
		Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now}
	return ctx.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_kind"}, {Name: "source_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "blocker_code", "source_hash", "updated_at"})}).Create(&mapping).Error
}

type downloadHistorySourceHash struct {
	ID              uint
	JobID           string
	URL             string
	Name            string
	Status          string
	Error           string
	ResourceID      *uint
	TotalSize       int64
	Progress        int64
	Attempts        int
	CreatedAt       time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	CreatedByUserID *uint
	PluginName      string
	Payload         []byte
	LastRetryJobID  string
	LastRetryAt     *time.Time
}

func hashDownloadHistory(row models.DownloadHistoryEntry) string {
	projection := downloadHistorySourceHash{
		ID: row.ID, JobID: row.JobID, URL: row.URL, Name: row.Name, Status: row.Status, Error: row.Error,
		ResourceID: row.ResourceID, TotalSize: row.TotalSize, Progress: row.Progress, Attempts: row.Attempts,
		CreatedAt: row.CreatedAt.UTC(), StartedAt: utcTimePtr(row.StartedAt), CompletedAt: utcTimePtr(row.CompletedAt),
		CreatedByUserID: row.CreatedByUserId, PluginName: row.PluginName, Payload: append([]byte(nil), row.Payload...),
		LastRetryJobID: row.LastRetryJobID, LastRetryAt: utcTimePtr(row.LastRetryAt),
	}
	encoded, _ := json.Marshal(projection)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func hashRetiredDownloadHistory(row models.DownloadHistoryEntry) string {
	return hashJobMigrationProjection(struct {
		ID      uint
		JobID   string
		URL     string
		Payload []byte
	}{row.ID, row.JobID, row.URL, append([]byte(nil), row.Payload...)})
}

func hashRetiredScheduledDownload(row models.ScheduledDownload) string {
	return hashJobMigrationProjection(struct {
		ID         uint
		PluginName string
		JobID      string
		URL        string
		Payload    []byte
	}{row.ID, row.PluginName, row.JobID, row.URL, append([]byte(nil), row.Payload...)})
}

func hashJobMigrationProjection(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (ctx *MahresourcesContext) copyDownloadHistory(row models.DownloadHistoryEntry, now time.Time) error {
	hash := hashDownloadHistory(row)
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		var prior models.JobSourceMapping
		err := tx.Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, strconv.FormatUint(uint64(row.ID), 10)).First(&prior).Error
		if err == nil {
			if prior.Status == models.JobSourceMappingPurged || prior.Status == models.JobSourceMappingScrubbed {
				return nil
			}
			if prior.SourceHash != hash {
				return ctx.refreshChangedDownloadHistoryMapping(tx, &prior, now)
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		mapping := models.JobSourceMapping{
			SourceKind: jobMigrationDownloadHistory, SourceID: strconv.FormatUint(uint64(row.ID), 10),
			SourceRevision: 1, SourceHash: hash, Status: models.JobSourceMappingCopied,
			Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		var handle models.JobLegacyHandle
		handleErr := tx.Where("namespace = ? AND handle = ?", DownloadHandleNamespace, row.JobID).First(&handle).Error
		if handleErr == nil {
			mapping.JobID = handle.JobID
			mapping.Origin = models.JobSourceOriginDualPublished
			purged, reason, err := ctx.downloadReplayPurged(tx, handle.JobID, now)
			if err != nil {
				return err
			}
			if purged {
				at := now
				mapping.Status, mapping.PurgedAt, mapping.PurgeReason = models.JobSourceMappingPurged, &at, reason
			} else if err := ctx.verifyDownloadReplay(tx, handle.JobID, row); err != nil {
				mapping.Status, mapping.BlockerCode = models.JobSourceMappingQuarantined, "canonical-replay-mismatch"
				if createErr := tx.Create(&mapping).Error; createErr != nil {
					return createErr
				}
				return nil
			}
			mapping.VerifiedAt = &now
			if mapping.Status == models.JobSourceMappingCopied {
				mapping.Status = models.JobSourceMappingVerified
			}
			return tx.Create(&mapping).Error
		}
		if !errors.Is(handleErr, gorm.ErrRecordNotFound) {
			return handleErr
		}
		creator, err := downloadHistoryCreator(row)
		if err != nil {
			return err
		}
		input, err := remoteDownloadInputJSON(creator, row.PluginName)
		if err != nil {
			return migrationBlocker(jobMigrationDownloadHistory, mapping.SourceID, "input-not-encodable")
		}
		state, failure, err := downloadHistoryJobOutcome(row)
		if err != nil {
			return err
		}
		owner := copyUintPtr(row.CreatedByUserId)
		origin := "api"
		if row.PluginName != "" {
			origin = "plugin"
		}
		acceptance := jobs.Acceptance{
			Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion, State: jobs.StateQueued,
			OwnerUserID: owner, ActorUserID: copyUintPtr(owner), Origin: origin,
			Title: downloadJobTitle(input), Replay: jobs.ReplayInput{Input: input},
			LegacyRefs: []jobs.LegacyRef{{Namespace: DownloadHandleNamespace, Handle: row.JobID}},
		}
		var note json.RawMessage
		if row.Attempts > 1 {
			note, _ = json.Marshal(map[string]any{"legacyAttempts": row.Attempts, "source": "download-history"})
		}
		deps := ctx.jobDeps()
		deps.DB = tx
		deps.Now = func() time.Time { return row.CreatedAt.UTC() }
		purgeReplay := legacyReplayExpired(row.CompletedAt, ctx.JobReplayRetention(), now)
		purgeReason := ""
		if purgeReplay {
			purgeReason = models.JobReplayPurgeExpired
		}
		if _, err := ctx.JobService().ImportLegacy(deps, jobs.LegacyImport{
			Acceptance: acceptance, State: state, AcceptedAt: row.CreatedAt.UTC(), StartedAt: row.StartedAt,
			FinishedAt: row.CompletedAt, Failure: failure, MigrationNote: note, PurgeReplayReason: purgeReason,
		}); err != nil {
			return migrationBlocker(jobMigrationDownloadHistory, mapping.SourceID, "canonical-job-import-failed")
		}
		var createdHandle models.JobLegacyHandle
		if err := tx.Where("namespace = ? AND handle = ?", DownloadHandleNamespace, row.JobID).First(&createdHandle).Error; err != nil {
			return err
		}
		mapping.JobID = createdHandle.JobID
		if purgeReplay {
			at := now
			mapping.Status, mapping.PurgedAt, mapping.PurgeReason = models.JobSourceMappingPurged, &at, models.JobReplayPurgeExpired
		}
		if err := tx.Create(&mapping).Error; err != nil {
			return err
		}
		return nil
	})
}

func downloadHistoryCreator(row models.DownloadHistoryEntry) (*query_models.ResourceFromRemoteCreator, error) {
	creator := &query_models.ResourceFromRemoteCreator{}
	if len(row.Payload) > 0 {
		if err := json.Unmarshal(row.Payload, creator); err != nil {
			return nil, migrationBlocker(jobMigrationDownloadHistory, strconv.FormatUint(uint64(row.ID), 10), "payload-unreadable")
		}
	}
	if creator.URL == "" {
		creator.URL = row.URL
	}
	if creator.Name == "" {
		creator.Name = row.Name
	}
	if creator.URL == "" {
		return nil, migrationBlocker(jobMigrationDownloadHistory, strconv.FormatUint(uint64(row.ID), 10), "execution-url-missing")
	}
	return creator, nil
}

func downloadHistoryJobOutcome(row models.DownloadHistoryEntry) (jobs.State, *jobs.Failure, error) {
	if row.CompletedAt == nil {
		return "", nil, migrationBlocker(jobMigrationDownloadHistory, strconv.FormatUint(uint64(row.ID), 10), "completion-time-missing")
	}
	switch row.Status {
	case models.DownloadHistoryStatusCompleted:
		return jobs.StateSucceeded, nil, nil
	case models.DownloadHistoryStatusCancelled:
		return jobs.StateCancelled, nil, nil
	case models.DownloadHistoryStatusFailed:
		return jobs.StateFailed, &jobs.Failure{Code: "legacy-download-failed", Class: jobs.FailureClassInternal, Message: "legacy download failed"}, nil
	default:
		return "", nil, migrationBlocker(jobMigrationDownloadHistory, strconv.FormatUint(uint64(row.ID), 10), "outcome-unknown")
	}
}

func (ctx *MahresourcesContext) verifyDownloadReplay(db *gorm.DB, jobID string, row models.DownloadHistoryEntry) error {
	creator, err := downloadHistoryCreator(row)
	if err != nil {
		return err
	}
	expected, err := remoteDownloadInputJSON(creator, row.PluginName)
	if err != nil {
		return err
	}
	opened, err := ctx.JobService().OpenReplay(ctx.jobDepsWithDB(db), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return err
	}
	var got, want downloadJobInput
	if err := json.Unmarshal(opened.Input, &got); err != nil {
		return err
	}
	if err := json.Unmarshal(expected, &want); err != nil {
		return err
	}
	if !sameDownloadJobInput(got, want) {
		return fmt.Errorf("decrypted input does not match source fields")
	}
	return nil
}

func (ctx *MahresourcesContext) jobDepsWithDB(db *gorm.DB) jobs.Deps {
	deps := ctx.jobDeps()
	deps.DB = db
	return deps
}

func sameDownloadJobInput(a, b downloadJobInput) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func (ctx *MahresourcesContext) downloadReplayPurged(db *gorm.DB, jobID string, now time.Time) (bool, string, error) {
	var envelope models.JobReplayEnvelope
	err := db.Where("job_id = ?", jobID).First(&envelope).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, "", fmt.Errorf("canonical download Job %s has no replay envelope", jobID)
	}
	if err != nil {
		return false, "", err
	}
	if envelope.PurgedAt != nil || len(envelope.Ciphertext) == 0 {
		reason := envelope.PurgeReason
		if reason == "" {
			reason = "canonical-replay-purged"
		}
		return true, reason, nil
	}
	if envelope.ExpiresAt != nil && !envelope.ExpiresAt.After(now) {
		return true, models.JobReplayPurgeExpired, nil
	}
	return false, "", nil
}

func legacyReplayExpired(finishedAt *time.Time, retention time.Duration, now time.Time) bool {
	return finishedAt != nil && retention > 0 && !finishedAt.UTC().Add(retention).After(now.UTC())
}

func downloadURLProjection(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func utcTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func quarantineJobSource(tx *gorm.DB, mapping *models.JobSourceMapping, message string) error {
	mapping.Status = models.JobSourceMappingQuarantined
	mapping.UpdatedAt = time.Now().UTC()
	if err := tx.Save(mapping).Error; err != nil {
		return errors.Join(fmt.Errorf("job source %s/%s: %s", mapping.SourceKind, mapping.SourceID, message), err)
	}
	return fmt.Errorf("job source %s/%s quarantined: %s", mapping.SourceKind, mapping.SourceID, message)
}

func (ctx *MahresourcesContext) verifyDownloadHistoryBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	var mappings []models.JobSourceMapping
	query := ctx.db.Where("source_kind = ? AND status IN ?", jobMigrationDownloadHistory,
		[]string{models.JobSourceMappingCopied, models.JobSourceMappingVerified}).Order("source_id ASC").Limit(limit)
	if cursor != "" {
		query = query.Where("source_id > ?", cursor)
	}
	if err := query.Find(&mappings).Error; err != nil {
		return false, cursor, err
	}
	for _, mapping := range mappings {
		id, err := strconv.ParseUint(mapping.SourceID, 10, 64)
		if err != nil {
			return false, cursor, err
		}
		var row models.DownloadHistoryEntry
		if err := ctx.db.First(&row, uint(id)).Error; err != nil {
			return false, cursor, fmt.Errorf("download history source %s disappeared before verification: %w", mapping.SourceID, err)
		}
		if hashDownloadHistory(row) != mapping.SourceHash {
			if err := ctx.db.Transaction(func(tx *gorm.DB) error {
				return ctx.refreshChangedDownloadHistoryMapping(tx, &mapping, now)
			}); err != nil {
				var blocker *jobMigrationBlockerError
				if !errors.As(err, &blocker) {
					return false, mapping.SourceID, errors.New("download history source reconciliation failed")
				}
				if err := ctx.quarantineDownloadHistory(row, blocker.code, now); err != nil {
					return false, mapping.SourceID, errors.New("changed download history quarantine could not be persisted")
				}
				if err := ctx.recordSafeSourceBlocker(blocker, now); err != nil {
					return false, mapping.SourceID, err
				}
				continue
			}
			if err := ctx.db.First(&row, uint(id)).Error; err != nil {
				return false, mapping.SourceID, errors.New("download history source could not be reread after reconciliation")
			}
			if mapping.Status == models.JobSourceMappingPurged || mapping.Status == models.JobSourceMappingScrubbed {
				continue
			}
		}
		if mapping.Status != models.JobSourceMappingPurged {
			if err := ctx.verifyDownloadReplay(ctx.db, mapping.JobID, row); err != nil {
				return false, mapping.SourceID, quarantineJobSource(ctx.db, &mapping, "canonical replay did not verify")
			}
		}
		mapping.Status = models.JobSourceMappingVerified
		mapping.VerifiedAt = &now
		mapping.UpdatedAt = now
		if err := ctx.db.Save(&mapping).Error; err != nil {
			return false, mapping.SourceID, err
		}
	}
	if len(mappings) == limit {
		return true, mappings[len(mappings)-1].SourceID, nil
	}
	return false, "", nil
}

func (ctx *MahresourcesContext) verifyJobMigrationSourceBatch(kind, cursor string, limit int, now time.Time, requireTerminal bool) (bool, string, error) {
	switch kind {
	case jobMigrationDownloadHistory:
		return ctx.verifyDownloadHistoryBatch(cursor, limit, now)
	case jobMigrationScheduledDownload:
		return ctx.verifyScheduledDownloadBatch(cursor, limit, now)
	case jobMigrationPluginCommandRun:
		return ctx.verifyPluginCommandRunsBatch(cursor, limit, now)
	case jobMigrationPluginCommandImport:
		return ctx.verifyPluginCommandImportsBatch(cursor, limit, now)
	case jobMigrationReduction:
		return ctx.verifyReductionsBatch(cursor, limit, now, requireTerminal)
	default:
		return false, cursor, errors.New("unknown job migration verification source")
	}
}

func (ctx *MahresourcesContext) verifyScheduledDownloadBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	query := ctx.db.Where("source_kind = ? AND status IN ?", jobMigrationScheduledDownload,
		[]string{models.JobSourceMappingCopied, models.JobSourceMappingVerified}).Order("source_id ASC").Limit(limit)
	if cursor != "" {
		query = query.Where("source_id > ?", cursor)
	}
	var mappings []models.JobSourceMapping
	if err := query.Find(&mappings).Error; err != nil {
		return false, cursor, errors.New("scheduled download mappings could not be read")
	}
	for _, mapping := range mappings {
		id, err := strconv.ParseUint(mapping.SourceID, 10, 64)
		if err != nil {
			return false, cursor, errors.New("scheduled download mapping id is invalid")
		}
		var row models.ScheduledDownload
		if err := ctx.db.First(&row, uint(id)).Error; err != nil {
			return false, cursor, migrationBlocker(jobMigrationScheduledDownload, mapping.SourceID, "source-row-missing")
		}
		if hashScheduledDownload(row) != mapping.SourceHash {
			if err := ctx.db.Transaction(func(tx *gorm.DB) error {
				return ctx.refreshChangedScheduledDownloadMapping(tx, &mapping, now)
			}); err != nil {
				var blocker *jobMigrationBlockerError
				if !errors.As(err, &blocker) {
					return false, mapping.SourceID, errors.New("scheduled download source reconciliation failed")
				}
				if err := ctx.quarantineScheduledDownload(row, blocker.code, now); err != nil {
					return false, mapping.SourceID, errors.New("changed scheduled download quarantine could not be persisted")
				}
				if err := ctx.recordSafeSourceBlocker(blocker, now); err != nil {
					return false, mapping.SourceID, err
				}
				continue
			}
			if err := ctx.db.First(&row, uint(id)).Error; err != nil {
				return false, mapping.SourceID, errors.New("scheduled download source could not be reread after reconciliation")
			}
			if mapping.Status == models.JobSourceMappingPurged || mapping.Status == models.JobSourceMappingScrubbed {
				continue
			}
		}
		if err := ctx.verifyScheduledDownloadReplay(ctx.db, mapping.JobID, row); err != nil {
			return false, mapping.SourceID, quarantineJobSource(ctx.db, &mapping, "canonical replay did not verify")
		}
		mapping.Status, mapping.VerifiedAt, mapping.UpdatedAt = models.JobSourceMappingVerified, &now, now
		if err := ctx.db.Save(&mapping).Error; err != nil {
			return false, mapping.SourceID, errors.New("scheduled download verification marker could not be stored")
		}
	}
	if len(mappings) == limit {
		return true, mappings[len(mappings)-1].SourceID, nil
	}
	return false, "", nil
}

func (ctx *MahresourcesContext) hasUnmappedDownloadHistory() (bool, error) {
	var unmapped int64
	err := ctx.db.Model(&models.DownloadHistoryEntry{}).
		Where("NOT EXISTS (SELECT 1 FROM job_source_mappings WHERE job_source_mappings.source_kind = ? AND job_source_mappings.source_id = CAST(download_history_entries.id AS TEXT))", jobMigrationDownloadHistory).
		Count(&unmapped).Error
	return unmapped > 0, err
}

func (ctx *MahresourcesContext) hasUnmappedJobMigrationSources() (bool, error) {
	for _, source := range []struct {
		model any
		table string
		kind  string
	}{
		{model: &models.DownloadHistoryEntry{}, table: "download_history_entries", kind: jobMigrationDownloadHistory},
		{model: &models.ScheduledDownload{}, table: "scheduled_downloads", kind: jobMigrationScheduledDownload},
		{model: &models.PluginCommandRun{}, table: "plugin_command_runs", kind: jobMigrationPluginCommandRun},
		{model: &models.PluginCommandImport{}, table: "plugin_command_imports", kind: jobMigrationPluginCommandImport},
		{model: &models.ResourceReduction{}, table: "resource_reductions", kind: jobMigrationReduction},
	} {
		query := ctx.db.Table(source.table).
			Joins("LEFT JOIN job_source_mappings AS mapping ON mapping.source_kind = ? AND mapping.source_id = CAST("+source.table+".id AS TEXT)", source.kind).
			Where("mapping.source_id IS NULL")
		if source.kind == jobMigrationReduction {
			query = query.Where("resource_reductions.status = ? AND resource_reductions.compute_job_id <> '' AND resource_reductions.computed_at IS NOT NULL AND EXISTS (SELECT 1 FROM job_legacy_handles WHERE job_legacy_handles.namespace = ? AND job_legacy_handles.handle = resource_reductions.compute_job_id)", models.ReductionStatusReady, ReductionComputeHandleNamespace)
		}
		var match int
		result := query.Select("1").Limit(1).Scan(&match)
		if result.Error != nil {
			return false, result.Error
		}
		if result.RowsAffected > 0 {
			return true, nil
		}
	}
	return false, nil
}

func (ctx *MahresourcesContext) scrubDownloadHistoryBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	var mappings []models.JobSourceMapping
	query := ctx.db.Where("source_kind = ? AND (status = ? OR (status = ? AND scrubbed_at IS NULL))",
		jobMigrationDownloadHistory, models.JobSourceMappingVerified, models.JobSourceMappingPurged).Order("source_id ASC").Limit(limit)
	if cursor != "" {
		query = query.Where("source_id > ?", cursor)
	}
	if err := query.Find(&mappings).Error; err != nil {
		return false, cursor, err
	}
	for _, mapping := range mappings {
		var changedAfterVerification bool
		err := ctx.db.Transaction(func(tx *gorm.DB) error {
			var current models.JobSourceMapping
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("source_kind = ? AND source_id = ?", jobMigrationDownloadHistory, mapping.SourceID).First(&current).Error; err != nil {
				return errors.New("download history scrub mapping disappeared")
			}
			if current.Status != models.JobSourceMappingVerified &&
				!(current.Status == models.JobSourceMappingPurged && current.ScrubbedAt == nil) {
				return nil
			}
			id, err := strconv.ParseUint(current.SourceID, 10, 64)
			if err != nil {
				return errors.New("download history mapping id is invalid")
			}
			var row models.DownloadHistoryEntry
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, uint(id)).Error; err != nil {
				return errors.New("download history disappeared before scrub")
			}
			if current.Status != models.JobSourceMappingPurged && hashDownloadHistory(row) != current.SourceHash {
				if err := quarantineJobSource(tx, &current, "source changed after verification"); err != nil {
					return err
				}
				changedAfterVerification = true
				return nil
			}
			projection := downloadURLProjection(row.URL)
			if err := tx.Model(&models.DownloadHistoryEntry{}).Where("id = ?", row.ID).
				Updates(map[string]any{"payload": nil, "url": projection}).Error; err != nil {
				return errors.New("download history plaintext scrub failed")
			}
			var scrubbed models.DownloadHistoryEntry
			if err := tx.First(&scrubbed, row.ID).Error; err != nil {
				return errors.New("download history could not be reread after scrub")
			}
			at := now
			if current.Status != models.JobSourceMappingPurged {
				current.Status = models.JobSourceMappingScrubbed
			}
			current.ScrubbedAt, current.PostScrubHash, current.UpdatedAt = &at, hashRetiredDownloadHistory(scrubbed), now
			if err := tx.Save(&current).Error; err != nil {
				return errors.New("download history scrub marker could not be stored")
			}
			return nil
		})
		if err != nil {
			return false, mapping.SourceID, err
		}
		if changedAfterVerification {
			return false, mapping.SourceID, fmt.Errorf("job source %s/%s changed after verification and was quarantined", mapping.SourceKind, mapping.SourceID)
		}
	}
	if len(mappings) == limit {
		return true, mappings[len(mappings)-1].SourceID, nil
	}
	return false, "", nil
}

func (ctx *MahresourcesContext) scrubJobMigrationSourceBatch(kind, cursor string, limit int, now time.Time) (bool, string, error) {
	switch kind {
	case jobMigrationDownloadHistory:
		return ctx.scrubDownloadHistoryBatch(cursor, limit, now)
	case jobMigrationScheduledDownload:
		return ctx.scrubScheduledDownloadBatch(cursor, limit, now)
	case jobMigrationPluginCommandRun:
		return ctx.scrubPluginCommandRunsBatch(cursor, limit, now)
	case jobMigrationPluginCommandImport:
		return ctx.scrubPluginCommandImportsBatch(cursor, limit, now)
	case jobMigrationReduction:
		return ctx.scrubReductionMappingsBatch(cursor, limit, now)
	default:
		return false, cursor, errors.New("unknown job migration scrub source")
	}
}

func (ctx *MahresourcesContext) scrubScheduledDownloadBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	query := ctx.db.Where("source_kind = ? AND (status = ? OR (status = ? AND scrubbed_at IS NULL))",
		jobMigrationScheduledDownload, models.JobSourceMappingVerified, models.JobSourceMappingPurged).Order("source_id ASC").Limit(limit)
	if cursor != "" {
		query = query.Where("source_id > ?", cursor)
	}
	var mappings []models.JobSourceMapping
	if err := query.Find(&mappings).Error; err != nil {
		return false, cursor, errors.New("scheduled download scrub mappings could not be read")
	}
	for _, mapping := range mappings {
		var changedAfterVerification bool
		err := ctx.db.Transaction(func(tx *gorm.DB) error {
			var current models.JobSourceMapping
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("source_kind = ? AND source_id = ?", jobMigrationScheduledDownload, mapping.SourceID).First(&current).Error; err != nil {
				return errors.New("scheduled download scrub mapping disappeared")
			}
			if current.Status != models.JobSourceMappingVerified &&
				!(current.Status == models.JobSourceMappingPurged && current.ScrubbedAt == nil) {
				return nil
			}
			id, err := strconv.ParseUint(current.SourceID, 10, 64)
			if err != nil {
				return errors.New("scheduled download mapping id is invalid")
			}
			var row models.ScheduledDownload
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, uint(id)).Error; err != nil {
				return errors.New("scheduled download disappeared before scrub")
			}
			if current.Status != models.JobSourceMappingPurged && hashScheduledDownload(row) != current.SourceHash {
				if err := quarantineJobSource(tx, &current, "source changed after verification"); err != nil {
					return err
				}
				changedAfterVerification = true
				return nil
			}
			projection := downloadURLProjection(row.URL)
			if err := tx.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).
				Updates(map[string]any{"payload": nil, "url": projection}).Error; err != nil {
				return errors.New("scheduled download plaintext scrub failed")
			}
			var scrubbed models.ScheduledDownload
			if err := tx.First(&scrubbed, row.ID).Error; err != nil {
				return errors.New("scheduled download could not be reread after scrub")
			}
			at := now
			if current.Status != models.JobSourceMappingPurged {
				current.Status = models.JobSourceMappingScrubbed
			}
			current.ScrubbedAt, current.PostScrubHash, current.UpdatedAt = &at, hashRetiredScheduledDownload(scrubbed), now
			if err := tx.Save(&current).Error; err != nil {
				return errors.New("scheduled download scrub marker could not be stored")
			}
			return nil
		})
		if err != nil {
			return false, mapping.SourceID, err
		}
		if changedAfterVerification {
			return false, mapping.SourceID, fmt.Errorf("job source %s/%s changed after verification and was quarantined", mapping.SourceKind, mapping.SourceID)
		}
	}
	if len(mappings) == limit {
		return true, mappings[len(mappings)-1].SourceID, nil
	}
	return false, "", nil
}

func (ctx *MahresourcesContext) installRetiredPlaintextBarrier(now time.Time) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		var epoch models.JobWriterEpoch
		if err := tx.Clauses().Where("id = ?", models.JobWriterEpochRowID).First(&epoch).Error; err != nil {
			return err
		}
		if epoch.MinimumEpoch > models.JobWriterEpochRetiredPlaintext {
			return fmt.Errorf("database job writer epoch %d is unsupported", epoch.MinimumEpoch)
		}
		if err := installLegacySourceBarriers(tx); err != nil {
			return err
		}
		if epoch.MinimumEpoch == models.JobWriterEpochRetiredPlaintext {
			return nil
		}
		result := tx.Model(&models.JobWriterEpoch{}).
			Where("id = ? AND minimum_epoch = ?", models.JobWriterEpochRowID, models.JobWriterEpochDualPublisher).
			Updates(map[string]any{"minimum_epoch": models.JobWriterEpochRetiredPlaintext, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("job writer epoch changed while installing plaintext barrier")
		}
		return nil
	})
}

func installLegacySourceBarriers(tx *gorm.DB) error {
	if tx.Dialector.Name() == "postgres" {
		return installPostgresLegacySourceBarriers(tx)
	}
	statements := []string{
		`CREATE TRIGGER IF NOT EXISTS job_barrier_download_history_insert BEFORE INSERT ON download_history_entries WHEN COALESCE(NEW.payload, '') <> '' OR (NEW.url <> '' AND (instr(NEW.url, '?') > 0 OR instr(NEW.url, '#') > 0 OR instr(NEW.url, '@') > 0 OR instr(NEW.url, '%') > 0 OR instr(substr(NEW.url, instr(NEW.url, '://') + 3), '/') > 0)) BEGIN SELECT RAISE(ABORT, 'legacy download replay fields are retired'); END`,
		`CREATE TRIGGER IF NOT EXISTS job_barrier_download_history_update BEFORE UPDATE OF payload, url ON download_history_entries WHEN COALESCE(NEW.payload, '') <> '' OR (NEW.url <> '' AND (instr(NEW.url, '?') > 0 OR instr(NEW.url, '#') > 0 OR instr(NEW.url, '@') > 0 OR instr(NEW.url, '%') > 0 OR instr(substr(NEW.url, instr(NEW.url, '://') + 3), '/') > 0)) BEGIN SELECT RAISE(ABORT, 'legacy download replay fields are retired'); END`,
		`CREATE TRIGGER IF NOT EXISTS job_barrier_scheduled_download_insert BEFORE INSERT ON scheduled_downloads WHEN COALESCE(NEW.payload, '') <> '' OR (NEW.url <> '' AND (instr(NEW.url, '?') > 0 OR instr(NEW.url, '#') > 0 OR instr(NEW.url, '@') > 0 OR instr(NEW.url, '%') > 0 OR instr(substr(NEW.url, instr(NEW.url, '://') + 3), '/') > 0)) BEGIN SELECT RAISE(ABORT, 'legacy scheduled replay fields are retired'); END`,
		`CREATE TRIGGER IF NOT EXISTS job_barrier_scheduled_download_update BEFORE UPDATE OF payload, url ON scheduled_downloads WHEN COALESCE(NEW.payload, '') <> '' OR (NEW.url <> '' AND (instr(NEW.url, '?') > 0 OR instr(NEW.url, '#') > 0 OR instr(NEW.url, '@') > 0 OR instr(NEW.url, '%') > 0 OR instr(substr(NEW.url, instr(NEW.url, '://') + 3), '/') > 0)) BEGIN SELECT RAISE(ABORT, 'legacy scheduled replay fields are retired'); END`,
		`CREATE TRIGGER IF NOT EXISTS job_barrier_command_run_insert BEFORE INSERT ON plugin_command_runs WHEN COALESCE(NEW.params_json, '') <> '' OR COALESCE(NEW.inputs_json, '') <> '' BEGIN SELECT RAISE(ABORT, 'legacy command input fields are retired'); END`,
		`CREATE TRIGGER IF NOT EXISTS job_barrier_command_run_update BEFORE UPDATE OF params_json, inputs_json ON plugin_command_runs WHEN COALESCE(NEW.params_json, '') <> '' OR COALESCE(NEW.inputs_json, '') <> '' BEGIN SELECT RAISE(ABORT, 'legacy command input fields are retired'); END`,
		`CREATE TRIGGER IF NOT EXISTS job_barrier_command_import_insert BEFORE INSERT ON plugin_command_imports WHEN COALESCE(NEW.fields_json, '') <> '' BEGIN SELECT RAISE(ABORT, 'legacy import fields are retired'); END`,
		`CREATE TRIGGER IF NOT EXISTS job_barrier_command_import_update BEFORE UPDATE OF fields_json ON plugin_command_imports WHEN COALESCE(NEW.fields_json, '') <> '' BEGIN SELECT RAISE(ABORT, 'legacy import fields are retired'); END`,
	}
	for _, statement := range statements {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func installPostgresLegacySourceBarriers(tx *gorm.DB) error {
	function := `CREATE OR REPLACE FUNCTION reject_retired_job_source_plaintext() RETURNS trigger AS $$
DECLARE
  current_row jsonb := to_jsonb(NEW);
	raw_url text;
BEGIN
  IF TG_TABLE_NAME IN ('download_history_entries', 'scheduled_downloads') THEN
    IF COALESCE(current_row->>'payload', '') <> '' THEN
      RAISE EXCEPTION 'legacy download replay fields are retired';
    END IF;
    raw_url := COALESCE(current_row->>'url', '');
	    IF raw_url <> '' AND raw_url !~ '^[A-Za-z][A-Za-z0-9+.-]*://[^/?#@%]+$' THEN
      RAISE EXCEPTION 'legacy download URL projection is not safe';
    END IF;
  ELSIF TG_TABLE_NAME = 'plugin_command_runs' THEN
    IF COALESCE(current_row->>'params_json', '') <> '' OR COALESCE(current_row->>'inputs_json', '') <> '' THEN
      RAISE EXCEPTION 'legacy command input fields are retired';
    END IF;
  ELSIF TG_TABLE_NAME = 'plugin_command_imports' THEN
    IF COALESCE(current_row->>'fields_json', '') <> '' THEN
      RAISE EXCEPTION 'legacy import fields are retired';
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql`
	if err := tx.Exec(function).Error; err != nil {
		return err
	}
	updates := map[string]string{
		"download_history_entries": "payload, url",
		"scheduled_downloads":      "payload, url",
		"plugin_command_runs":      "params_json, inputs_json",
		"plugin_command_imports":   "fields_json",
	}
	for _, table := range []string{"download_history_entries", "scheduled_downloads", "plugin_command_runs", "plugin_command_imports"} {
		if err := tx.Exec("DROP TRIGGER IF EXISTS retired_job_source_plaintext_" + table + " ON " + table).Error; err != nil {
			return err
		}
		statement := "CREATE TRIGGER retired_job_source_plaintext_" + table + " BEFORE INSERT OR UPDATE OF " + updates[table] + " ON " + table +
			" FOR EACH ROW EXECUTE FUNCTION reject_retired_job_source_plaintext()"
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
