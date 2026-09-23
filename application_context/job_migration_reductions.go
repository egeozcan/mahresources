package application_context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"mahresources/jobs"
	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type reductionMigrationProof struct {
	ID              uint
	ComputeJobID    string
	CreatedAt       time.Time
	ComputedAt      *time.Time
	CreatedByUserID *uint
	Status          string
}

func hashReductionExecution(row models.ResourceReduction) string {
	encoded, _ := json.Marshal(reductionMigrationProof{
		ID: row.ID, ComputeJobID: row.ComputeJobID, CreatedAt: row.CreatedAt.UTC(),
		ComputedAt: utcTimePtr(row.ComputedAt), CreatedByUserID: row.CreatedByUserId,
		Status: row.Status,
	})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (ctx *MahresourcesContext) recordDualPublishedReduction(row models.ResourceReduction, now time.Time) error {
	if row.Status != models.ReductionStatusReady || row.ComputeJobID == "" || row.ComputedAt == nil {
		return nil
	}
	var handle models.JobLegacyHandle
	if err := ctx.db.Where("namespace = ? AND handle = ?", ReductionComputeHandleNamespace, row.ComputeJobID).First(&handle).Error; err != nil {
		return errors.New("Resource Reduction canonical handle is unavailable")
	}
	retired, err := ctx.legacyJobInputsRetired()
	if err != nil {
		return fmt.Errorf("Resource Reduction writer epoch cannot be read: %w", err)
	}
	return ctx.recordDualPublishedSource(jobMigrationReduction, strconv.FormatUint(uint64(row.ID), 10), handle.JobID,
		hashReductionExecution(row), retired, now)
}

func (ctx *MahresourcesContext) copyReductionsBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	var after uint64
	if cursor != "" {
		parsed, err := strconv.ParseUint(cursor, 10, 64)
		if err != nil {
			return false, cursor, errors.New("Resource Reduction migration cursor is invalid")
		}
		after = parsed
	}
	var rows []models.ResourceReduction
	if err := ctx.db.Where("id > ?", after).Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return false, cursor, errors.New("Resource Reduction source scan failed")
	}
	for _, row := range rows {
		if !provableReductionExecution(row) {
			continue
		}
		if err := ctx.copyReduction(row, now); err != nil {
			var blocker *jobMigrationBlockerError
			if !errors.As(err, &blocker) {
				return false, strconv.FormatUint(uint64(row.ID), 10), errors.New("Resource Reduction copy failed; details are redacted")
			}
			if err := ctx.quarantineReduction(row, blocker.code, now); err != nil {
				return false, strconv.FormatUint(uint64(row.ID), 10), errors.New("Resource Reduction quarantine could not be persisted")
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

func provableReductionExecution(row models.ResourceReduction) bool {
	return row.Status == models.ReductionStatusReady && row.ComputeJobID != "" && row.ComputedAt != nil &&
		!row.CreatedAt.IsZero() && !row.ComputedAt.IsZero() && !row.ComputedAt.Before(row.CreatedAt)
}

func (ctx *MahresourcesContext) copyReduction(row models.ResourceReduction, now time.Time) error {
	id := strconv.FormatUint(uint64(row.ID), 10)
	var prior models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationReduction, id).First(&prior).Error; err == nil {
		if prior.Status == models.JobSourceMappingPurged || prior.Status == models.JobSourceMappingScrubbed {
			return nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("Resource Reduction mapping lookup failed")
	}
	hash := hashReductionExecution(row)
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		var mapping models.JobSourceMapping
		err := tx.Where("source_kind = ? AND source_id = ?", jobMigrationReduction, id).First(&mapping).Error
		if err == nil {
			if mapping.Status == models.JobSourceMappingPurged || mapping.Status == models.JobSourceMappingScrubbed {
				return nil
			}
			if mapping.SourceHash == hash {
				return nil
			}
			mapping.SourceRevision++
			mapping.SourceHash, mapping.Status, mapping.VerifiedAt = hash, models.JobSourceMappingCopied, nil
			mapping.CopiedAt, mapping.UpdatedAt, mapping.BlockerCode = now, now, ""
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			mapping = models.JobSourceMapping{SourceKind: jobMigrationReduction, SourceID: id, SourceRevision: 1,
				SourceHash: hash, Status: models.JobSourceMappingCopied, Origin: models.JobSourceOriginBackfilled,
				CopiedAt: now, CreatedAt: now, UpdatedAt: now}
		} else {
			return errors.New("Resource Reduction mapping lookup failed")
		}
		jobID, err := findPluginCommandJob(tx, ReductionComputeHandleNamespace, row.ComputeJobID, "")
		if err == nil {
			mapping.JobID, mapping.Origin = jobID, models.JobSourceOriginDualPublished
			return tx.Save(&mapping).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("Resource Reduction canonical handle lookup failed")
		}
		// ResourceReduction.CreatedAt is the domain object's creation time, not
		// the acceptance time of its compute run. A ready row without a canonical
		// handle is ordinary legacy domain data, not a migration failure; leave it
		// untouched and do not fabricate a Job timestamp from CreatedAt.
		return nil
	})
}

func verifyReductionJob(tx *gorm.DB, service *jobs.Service, deps jobs.Deps, jobID string, row models.ResourceReduction, requireTerminal bool) error {
	var job models.Job
	if err := tx.Where("id = ?", jobID).First(&job).Error; err != nil {
		return err
	}
	if job.Kind != JobKindReductionCompute {
		return fmt.Errorf("canonical reduction outcome differs from source")
	}
	if job.State == string(jobs.StateSucceeded) {
		if job.FinishedAt == nil || !job.FinishedAt.Equal(row.ComputedAt.UTC()) {
			return fmt.Errorf("canonical reduction outcome differs from source")
		}
	} else if requireTerminal || (job.State != string(jobs.StateRunning) && job.State != string(jobs.StateBlocked)) {
		return fmt.Errorf("canonical Reduction Job is not in a verifiable state")
	}
	if job.OwnerUserID != nil && (row.CreatedByUserId == nil || *job.OwnerUserID != *row.CreatedByUserId) {
		return fmt.Errorf("canonical reduction owner differs from source")
	}
	if jobs.ReplayClass(job.ReplayClass) == jobs.ReplayClassNonReplayable {
		return nil
	}
	opened, err := service.OpenReplay(deps, jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return err
	}
	input, err := reductionComputeInputOf(opened.Input)
	if err != nil || input.ReductionID != row.ID {
		return fmt.Errorf("canonical reduction input differs from source")
	}
	return nil
}

func (ctx *MahresourcesContext) quarantineReduction(row models.ResourceReduction, code string, now time.Time) error {
	id := strconv.FormatUint(uint64(row.ID), 10)
	mapping := models.JobSourceMapping{SourceKind: jobMigrationReduction, SourceID: id, SourceRevision: 1,
		SourceHash: hashReductionExecution(row), Status: models.JobSourceMappingQuarantined, BlockerCode: code,
		Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now}
	return ctx.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_kind"}, {Name: "source_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "blocker_code", "source_hash", "updated_at"})}).Create(&mapping).Error
}

func (ctx *MahresourcesContext) verifyReductionsBatch(cursor string, limit int, now time.Time, requireTerminal bool) (bool, string, error) {
	query := ctx.db.Where("source_kind = ? AND status IN ?", jobMigrationReduction,
		[]string{models.JobSourceMappingCopied, models.JobSourceMappingVerified}).Order("source_id ASC").Limit(limit)
	if cursor != "" {
		query = query.Where("source_id > ?", cursor)
	}
	var mappings []models.JobSourceMapping
	if err := query.Find(&mappings).Error; err != nil {
		return false, cursor, errors.New("Resource Reduction mappings could not be read")
	}
	for _, mapping := range mappings {
		id, err := strconv.ParseUint(mapping.SourceID, 10, 64)
		if err != nil {
			return false, cursor, errors.New("Resource Reduction mapping id is invalid")
		}
		var row models.ResourceReduction
		if err := ctx.db.First(&row, uint(id)).Error; err != nil || !provableReductionExecution(row) {
			return false, mapping.SourceID, migrationBlocker(jobMigrationReduction, mapping.SourceID, "source-row-no-longer-provable")
		}
		if hashReductionExecution(row) != mapping.SourceHash {
			return false, mapping.SourceID, quarantineJobSource(ctx.db, &mapping, "Resource Reduction execution proof changed")
		}
		if err := verifyReductionJob(ctx.db, ctx.JobService(), ctx.jobDeps(), mapping.JobID, row, requireTerminal); err != nil {
			return false, mapping.SourceID, quarantineJobSource(ctx.db, &mapping, "Resource Reduction Job did not verify")
		}
		mapping.Status, mapping.VerifiedAt, mapping.UpdatedAt = models.JobSourceMappingVerified, &now, now
		if err := ctx.db.Save(&mapping).Error; err != nil {
			return false, mapping.SourceID, errors.New("Resource Reduction verification marker could not be stored")
		}
	}
	if len(mappings) == limit {
		return true, mappings[len(mappings)-1].SourceID, nil
	}
	return false, "", nil
}

func (ctx *MahresourcesContext) scrubReductionMappingsBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	query := ctx.db.Where("source_kind = ? AND (status = ? OR (status = ? AND scrubbed_at IS NULL))",
		jobMigrationReduction, models.JobSourceMappingVerified, models.JobSourceMappingPurged).Order("source_id ASC").Limit(limit)
	if cursor != "" {
		query = query.Where("source_id > ?", cursor)
	}
	var mappings []models.JobSourceMapping
	if err := query.Find(&mappings).Error; err != nil {
		return false, cursor, errors.New("Resource Reduction scrub mappings could not be read")
	}
	for _, mapping := range mappings {
		id, err := strconv.ParseUint(mapping.SourceID, 10, 64)
		if err != nil {
			return false, cursor, errors.New("Resource Reduction mapping id is invalid")
		}
		var row models.ResourceReduction
		if err := ctx.db.First(&row, uint(id)).Error; err != nil {
			return false, mapping.SourceID, errors.New("Resource Reduction disappeared before retirement marker")
		}
		postHash := hashReductionExecution(row)
		if mapping.Status != models.JobSourceMappingPurged && mapping.SourceHash != postHash {
			return false, mapping.SourceID, quarantineJobSource(ctx.db, &mapping, "Resource Reduction proof changed after verification")
		}
		at := now
		if mapping.Status != models.JobSourceMappingPurged {
			mapping.Status = models.JobSourceMappingScrubbed
		}
		mapping.ScrubbedAt, mapping.PostScrubHash, mapping.UpdatedAt = &at, postHash, now
		if err := ctx.db.Save(&mapping).Error; err != nil {
			return false, mapping.SourceID, errors.New("Resource Reduction retirement marker could not be stored")
		}
	}
	if len(mappings) == limit {
		return true, mappings[len(mappings)-1].SourceID, nil
	}
	return false, "", nil
}
