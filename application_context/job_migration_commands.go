package application_context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/plugin_commands"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	jobMigrationPluginCommandRun    = "plugin-command-run"
	jobMigrationPluginCommandImport = "plugin-command-import"
)

type pluginCommandRunMigrationHash struct {
	ID              string
	PluginName      string
	CommandName     string
	ParamsJSON      string
	InputsJSON      string
	CreatedByUserID *uint
	Actorless       bool
	CreatedAt       time.Time
}

func hashPluginCommandRun(row models.PluginCommandRun) string {
	encoded, _ := json.Marshal(pluginCommandRunMigrationHash{
		ID: row.ID, PluginName: row.PluginName, CommandName: row.CommandName,
		ParamsJSON: row.ParamsJSON, InputsJSON: row.InputsJSON,
		CreatedByUserID: row.CreatedByUserId, Actorless: row.ActorlessAtSubmission,
		CreatedAt: row.CreatedAt.UTC(),
	})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func hashRetiredPluginCommandRun(row models.PluginCommandRun) string {
	return hashJobMigrationProjection(struct {
		ID         string
		JobID      string
		ParamsJSON string
		InputsJSON string
	}{row.ID, row.JobID, row.ParamsJSON, row.InputsJSON})
}

type pluginCommandImportMigrationHash struct {
	ID               string
	RunID            string
	FileName         string
	FieldsJSON       string
	PluginGeneration uint64
	CreatedByUserID  *uint
	CreatedAt        time.Time
}

func hashPluginCommandImport(row models.PluginCommandImport) string {
	encoded, _ := json.Marshal(pluginCommandImportMigrationHash{
		ID: row.ID, RunID: row.RunID, FileName: row.FileName, FieldsJSON: row.FieldsJSON,
		PluginGeneration: row.PluginGeneration, CreatedByUserID: row.CreatedByUserId,
		CreatedAt: row.CreatedAt.UTC(),
	})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func hashRetiredPluginCommandImport(row models.PluginCommandImport) string {
	return hashJobMigrationProjection(struct {
		ID         string
		JobID      string
		RunID      string
		FileName   string
		FieldsJSON string
	}{row.ID, row.JobID, row.RunID, row.FileName, row.FieldsJSON})
}

func (ctx *MahresourcesContext) recordDualPublishedPluginCommandRunTx(tx *gorm.DB, row models.PluginCommandRun, scrubbed bool, now time.Time) error {
	if row.JobID == "" {
		return errors.New("plugin command run has no canonical Job")
	}
	return ctx.recordDualPublishedSourceTx(tx, jobMigrationPluginCommandRun, row.ID, row.JobID,
		hashPluginCommandRun(row), hashRetiredPluginCommandRun(row), scrubbed, now)
}

func (ctx *MahresourcesContext) recordDualPublishedPluginCommandImportTx(tx *gorm.DB, row models.PluginCommandImport, scrubbed bool, now time.Time) error {
	if row.JobID == "" {
		return errors.New("plugin command import has no canonical Job")
	}
	return ctx.recordDualPublishedSourceTx(tx, jobMigrationPluginCommandImport, row.ID, row.JobID,
		hashPluginCommandImport(row), hashRetiredPluginCommandImport(row), scrubbed, now)
}

func validateCommandRunSource(row models.PluginCommandRun) error {
	if row.ID == "" || row.PluginName == "" || row.CommandName == "" || row.CreatedAt.IsZero() || row.ParamsJSON == "" || !json.Valid([]byte(row.ParamsJSON)) {
		return migrationBlocker(jobMigrationPluginCommandRun, row.ID, "source-input-unreadable")
	}
	if row.InputsJSON != "" && !json.Valid([]byte(row.InputsJSON)) {
		return migrationBlocker(jobMigrationPluginCommandRun, row.ID, "source-input-unreadable")
	}
	return nil
}

func commandRunReplay(row models.PluginCommandRun) json.RawMessage {
	input, _ := json.Marshal(pluginCommandRunReplayInput{
		PluginName: row.PluginName, CommandName: row.CommandName,
		ParamsJSON: row.ParamsJSON, InputsJSON: row.InputsJSON,
	})
	return input
}

func commandImportReplay(row models.PluginCommandImport) json.RawMessage {
	input, _ := json.Marshal(pluginCommandImportReplayInput{
		RunID: row.RunID, FileName: row.FileName, FieldsJSON: row.FieldsJSON,
		PluginGeneration: row.PluginGeneration,
	})
	return input
}

func (ctx *MahresourcesContext) copyPluginCommandRunsBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	query := ctx.db.Order("id ASC").Limit(limit)
	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}
	var rows []models.PluginCommandRun
	if err := query.Find(&rows).Error; err != nil {
		return false, cursor, errors.New("plugin command run source scan failed")
	}
	for _, row := range rows {
		if err := ctx.copyPluginCommandRun(row, now); err != nil {
			var blocker *jobMigrationBlockerError
			if !errors.As(err, &blocker) {
				return false, row.ID, errors.New("plugin command run copy failed; details are redacted")
			}
			if err := ctx.quarantinePluginCommandRun(row, blocker.code, now); err != nil {
				return false, row.ID, errors.New("plugin command run quarantine could not be persisted")
			}
			if err := ctx.recordSafeSourceBlocker(blocker, now); err != nil {
				return false, row.ID, err
			}
		}
	}
	if len(rows) == limit {
		return true, rows[len(rows)-1].ID, nil
	}
	return false, "", nil
}

func (ctx *MahresourcesContext) copyPluginCommandRun(row models.PluginCommandRun, now time.Time) error {
	var existing models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandRun, row.ID).First(&existing).Error; err == nil {
		if existing.Status == models.JobSourceMappingPurged || existing.Status == models.JobSourceMappingScrubbed {
			return nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("plugin command run mapping lookup failed")
	}
	if err := validateCommandRunSource(row); err != nil {
		return err
	}
	hash := hashPluginCommandRun(row)
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		var mapping models.JobSourceMapping
		err := tx.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandRun, row.ID).First(&mapping).Error
		if err == nil {
			if mapping.Status == models.JobSourceMappingPurged || mapping.Status == models.JobSourceMappingScrubbed {
				return nil
			}
			if mapping.SourceHash != hash {
				return migrationBlocker(jobMigrationPluginCommandRun, row.ID, "source-input-changed-after-copy")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("plugin command run mapping lookup failed")
		}
		mapping = models.JobSourceMapping{SourceKind: jobMigrationPluginCommandRun, SourceID: row.ID,
			SourceRevision: 1, SourceHash: hash, Status: models.JobSourceMappingCopied,
			Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now}

		jobID, err := findPluginCommandJob(tx, pluginCommandHandleNamespace, row.ID, row.JobID)
		if err == nil {
			mapping.JobID, mapping.Origin = jobID, models.JobSourceOriginDualPublished
			purged, reason, err := ctx.ensureCommandReplay(tx, jobID, commandRunReplay(row), row.FinishedAt, isTerminalPluginRun(row.Status), now)
			if err != nil {
				return migrationBlocker(jobMigrationPluginCommandRun, row.ID, "canonical-replay-unavailable")
			}
			if purged {
				at := now
				mapping.Status, mapping.PurgedAt, mapping.PurgeReason = models.JobSourceMappingPurged, &at, reason
			} else if err := ctx.verifyPluginCommandRunReplay(tx, jobID, row); err != nil {
				return migrationBlocker(jobMigrationPluginCommandRun, row.ID, "canonical-replay-mismatch")
			}
			if err := persistCommandSourceJobID(tx, &row, jobID); err != nil {
				return errors.New("plugin command canonical Job identity could not be backfilled")
			}
			return tx.Create(&mapping).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("plugin command canonical handle lookup failed")
		}
		state, failure, finished, err := pluginRunLegacyOutcome(row)
		if err != nil {
			return err
		}
		acceptance := jobs.Acceptance{Kind: JobKindPluginCommand, KindVersion: jobPluginCommandVersion,
			State: state, ActorUserID: copyUintPtr(row.CreatedByUserId), Origin: "plugin",
			Title: row.CommandName, Replay: jobs.ReplayInput{Input: commandRunReplay(row)},
			LegacyRefs: []jobs.LegacyRef{{Namespace: pluginCommandHandleNamespace, Handle: row.ID}}}
		deps := ctx.jobDepsWithDB(tx)
		deps.Now = func() time.Time { return row.CreatedAt.UTC() }
		purgeReason := ""
		if terminal := isTerminalPluginRun(row.Status); terminal && finished != nil && ctx.JobReplayRetention() > 0 && !finished.Add(ctx.JobReplayRetention()).After(now) {
			purgeReason = models.JobReplayPurgeExpired
		}
		imported, err := ctx.JobService().ImportLegacy(deps, jobs.LegacyImport{Acceptance: acceptance,
			State: state, AcceptedAt: row.CreatedAt.UTC(), StartedAt: row.StartedAt, FinishedAt: finished,
			Failure: failure, PurgeReplayReason: purgeReason})
		if err != nil {
			return migrationBlocker(jobMigrationPluginCommandRun, row.ID, "canonical-job-import-failed")
		}
		mapping.JobID = imported.ID
		if purgeReason != "" {
			at := now
			mapping.Status, mapping.PurgedAt, mapping.PurgeReason = models.JobSourceMappingPurged, &at, purgeReason
		}
		if err := persistCommandSourceJobID(tx, &row, imported.ID); err != nil {
			return errors.New("plugin command canonical Job identity could not be backfilled")
		}
		return tx.Create(&mapping).Error
	})
}

func pluginRunLegacyOutcome(row models.PluginCommandRun) (jobs.State, *jobs.Failure, *time.Time, error) {
	switch row.Status {
	case plugin_commands.RunStatusQueued:
		return jobs.StateQueued, nil, nil, nil
	case plugin_commands.RunStatusSucceeded:
		return jobs.StateSucceeded, nil, row.FinishedAt, requireCommandFinish(row.ID, row.FinishedAt)
	case plugin_commands.RunStatusFailed:
		return jobs.StateFailed, &jobs.Failure{Code: "plugin-command-failed", Class: jobs.FailureClassInternal, Message: "the plugin command did not complete successfully"}, row.FinishedAt, requireCommandFinish(row.ID, row.FinishedAt)
	case plugin_commands.RunStatusCancelled:
		return jobs.StateCancelled, nil, row.FinishedAt, requireCommandFinish(row.ID, row.FinishedAt)
	case plugin_commands.RunStatusInterrupted:
		return jobs.StateInterrupted, nil, row.FinishedAt, requireCommandFinish(row.ID, row.FinishedAt)
	case plugin_commands.RunStatusRunning:
		return "", nil, nil, migrationBlocker(jobMigrationPluginCommandRun, row.ID, "running-source-without-canonical-job")
	default:
		return "", nil, nil, migrationBlocker(jobMigrationPluginCommandRun, row.ID, "outcome-unknown")
	}
}

func requireCommandFinish(sourceID string, finished *time.Time) error {
	if finished == nil || finished.IsZero() {
		return migrationBlocker(jobMigrationPluginCommandRun, sourceID, "outcome-time-unproven")
	}
	return nil
}

func isTerminalPluginRun(status string) bool {
	return status == plugin_commands.RunStatusSucceeded || status == plugin_commands.RunStatusFailed ||
		status == plugin_commands.RunStatusCancelled || status == plugin_commands.RunStatusInterrupted
}

func findPluginCommandJob(tx *gorm.DB, namespace, handle, directJobID string) (string, error) {
	var existing models.JobLegacyHandle
	err := tx.Where("namespace = ? AND handle = ?", namespace, handle).First(&existing).Error
	if err == nil {
		if directJobID != "" && directJobID != existing.JobID {
			return "", fmt.Errorf("source Job identity disagrees with its legacy handle")
		}
		return existing.JobID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if directJobID == "" {
		return "", gorm.ErrRecordNotFound
	}
	var job models.Job
	if err := tx.Where("id = ?", directJobID).First(&job).Error; err != nil {
		return "", err
	}
	created := models.JobLegacyHandle{Namespace: namespace, Handle: handle, JobID: directJobID, CreatedAt: time.Now().UTC()}
	if err := tx.Create(&created).Error; err != nil {
		return "", err
	}
	return directJobID, nil
}

func (ctx *MahresourcesContext) ensureCommandReplay(tx *gorm.DB, jobID string, expected json.RawMessage, finished *time.Time, terminal bool, now time.Time) (bool, string, error) {
	var job models.Job
	if err := tx.Where("id = ?", jobID).First(&job).Error; err != nil {
		return false, "", err
	}
	var envelope models.JobReplayEnvelope
	err := tx.Where("job_id = ?", jobID).First(&envelope).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, "", err
	}
	if err == nil && (envelope.PurgedAt != nil || (envelope.ExpiresAt != nil && !envelope.ExpiresAt.After(now))) {
		if !terminal {
			return false, "", fmt.Errorf("nonterminal command replay is purged")
		}
		reason := envelope.PurgeReason
		if reason == "" {
			reason = models.JobReplayPurgeExpired
		}
		return true, reason, nil
	}
	if err != nil {
		if jobs.ReplayClass(job.ReplayClass) != jobs.ReplayClassNonReplayable {
			return false, "", fmt.Errorf("replayable command Job is missing its envelope")
		}
		if terminal && finished != nil && ctx.JobReplayRetention() > 0 && !finished.Add(ctx.JobReplayRetention()).After(now) {
			return true, models.JobReplayPurgeExpired, nil
		}
		if installErr := ctx.JobService().InstallLegacyReplay(ctx.jobDepsWithDB(tx), jobID, jobs.ReplayInput{Input: expected}); installErr != nil {
			return false, "", installErr
		}
	}
	return false, "", nil
}

func (ctx *MahresourcesContext) verifyPluginCommandRunReplay(tx *gorm.DB, jobID string, row models.PluginCommandRun) error {
	opened, err := ctx.JobService().OpenReplay(ctx.jobDepsWithDB(tx), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return err
	}
	var got pluginCommandRunReplayInput
	if err := json.Unmarshal(opened.Input, &got); err != nil {
		return err
	}
	want := pluginCommandRunReplayInput{PluginName: row.PluginName, CommandName: row.CommandName, ParamsJSON: row.ParamsJSON, InputsJSON: row.InputsJSON}
	if got != want {
		return fmt.Errorf("canonical replay differs from source")
	}
	var job models.Job
	if err := tx.Where("id = ?", jobID).First(&job).Error; err != nil {
		return err
	}
	wantState, _, _, stateErr := pluginRunLegacyOutcome(row)
	if stateErr != nil && row.Status != plugin_commands.RunStatusRunning {
		return stateErr
	}
	if row.Status == plugin_commands.RunStatusRunning {
		if job.State != string(jobs.StateRunning) && job.State != string(jobs.StateBlocked) {
			return fmt.Errorf("running source and canonical Job state disagree")
		}
	} else if job.State != string(wantState) {
		return fmt.Errorf("source and canonical Job outcomes disagree")
	}
	return nil
}

func (ctx *MahresourcesContext) quarantinePluginCommandRun(row models.PluginCommandRun, code string, now time.Time) error {
	mapping := models.JobSourceMapping{SourceKind: jobMigrationPluginCommandRun, SourceID: row.ID,
		SourceRevision: 1, SourceHash: hashPluginCommandRun(row), Status: models.JobSourceMappingQuarantined,
		BlockerCode: code, Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now}
	if row.JobID != "" {
		mapping.JobID = row.JobID
	}
	return ctx.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_kind"}, {Name: "source_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "blocker_code", "source_hash", "updated_at"})}).Create(&mapping).Error
}

func (ctx *MahresourcesContext) copyPluginCommandImportsBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	query := ctx.db.Order("id ASC").Limit(limit)
	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}
	var rows []models.PluginCommandImport
	if err := query.Find(&rows).Error; err != nil {
		return false, cursor, errors.New("plugin command import source scan failed")
	}
	for _, row := range rows {
		if err := ctx.copyPluginCommandImport(row, now); err != nil {
			var blocker *jobMigrationBlockerError
			if !errors.As(err, &blocker) {
				return false, row.ID, errors.New("plugin command import copy failed; details are redacted")
			}
			if err := ctx.quarantinePluginCommandImport(row, blocker.code, now); err != nil {
				return false, row.ID, errors.New("plugin command import quarantine could not be persisted")
			}
			if err := ctx.recordSafeSourceBlocker(blocker, now); err != nil {
				return false, row.ID, err
			}
		}
	}
	if len(rows) == limit {
		return true, rows[len(rows)-1].ID, nil
	}
	return false, "", nil
}

func (ctx *MahresourcesContext) copyPluginCommandImport(row models.PluginCommandImport, now time.Time) error {
	var existing models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandImport, row.ID).First(&existing).Error; err == nil {
		if existing.Status == models.JobSourceMappingPurged || existing.Status == models.JobSourceMappingScrubbed {
			return nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("plugin command import mapping lookup failed")
	}
	if row.ID == "" || row.RunID == "" || row.FileName == "" || row.CreatedAt.IsZero() || (row.FieldsJSON != "" && !json.Valid([]byte(row.FieldsJSON))) {
		return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-unreadable")
	}
	hash := hashPluginCommandImport(row)
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		var mapping models.JobSourceMapping
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandImport, row.ID).First(&mapping).Error
		resumeQuarantine := false
		if err == nil {
			if mapping.Status == models.JobSourceMappingPurged || mapping.Status == models.JobSourceMappingScrubbed {
				return nil
			}
			if mapping.SourceHash != hash {
				return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-changed-after-copy")
			}
			if mapping.Status != models.JobSourceMappingQuarantined || mapping.BlockerCode != "source-input-unreadable" || mapping.JobID != "" {
				return nil
			}
			resumeQuarantine = true
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("plugin command import mapping lookup failed")
		}
		var current models.PluginCommandImport
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", row.ID).First(&current).Error; err != nil {
			return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-row-missing")
		}
		if hashPluginCommandImport(current) != hash {
			return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-changed-after-copy")
		}
		row = current
		historicalEvidence, err := ctx.proveBlankPluginCommandImport(tx, row)
		if err != nil {
			return err
		}
		if !resumeQuarantine {
			mapping = models.JobSourceMapping{SourceKind: jobMigrationPluginCommandImport, SourceID: row.ID,
				SourceRevision: 1, SourceHash: hash, Status: models.JobSourceMappingCopied,
				Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now}
		}
		if historicalEvidence != nil {
			state, failure, finished, err := pluginImportLegacyOutcome(row)
			if err != nil {
				return err
			}
			if state != jobs.StateSucceeded || failure != nil || finished == nil {
				return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-unreadable")
			}
			return ctx.copyProvenBlankPluginCommandImport(tx, &mapping, &row, *historicalEvidence, now, resumeQuarantine)
		}

		jobID, err := findPluginCommandJob(tx, pluginCommandImportHandleNamespace, row.ID, row.JobID)
		if err == nil {
			mapping.JobID, mapping.Origin = jobID, models.JobSourceOriginDualPublished
			purged, reason, err := ctx.ensureCommandReplay(tx, jobID, commandImportReplay(row), row.FinishedAt, isTerminalPluginImport(row.Status), now)
			if err != nil {
				return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "canonical-replay-unavailable")
			}
			if purged {
				at := now
				mapping.Status, mapping.PurgedAt, mapping.PurgeReason = models.JobSourceMappingPurged, &at, reason
			} else if err := ctx.verifyPluginCommandImportReplay(tx, jobID, row); err != nil {
				return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "canonical-replay-mismatch")
			}
			if err := persistCommandImportJobID(tx, &row, jobID); err != nil {
				return errors.New("plugin command import canonical Job identity could not be backfilled")
			}
			return tx.Create(&mapping).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("plugin command import handle lookup failed")
		}
		state, failure, finished, err := pluginImportLegacyOutcome(row)
		if err != nil {
			return err
		}
		var run models.PluginCommandRun
		if err := tx.Where("id = ?", row.RunID).First(&run).Error; err != nil {
			return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "parent-run-missing")
		}
		parentJobID := run.JobID
		if parentJobID == "" {
			var parentMapping models.JobSourceMapping
			if err := tx.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandRun, run.ID).First(&parentMapping).Error; err == nil && parentMapping.Status != models.JobSourceMappingQuarantined {
				parentJobID = parentMapping.JobID
			} else {
				var parentHandle models.JobLegacyHandle
				if err := tx.Where("namespace = ? AND handle = ?", pluginCommandHandleNamespace, run.ID).First(&parentHandle).Error; err == nil {
					parentJobID = parentHandle.JobID
				}
			}
		}
		parents := []string(nil)
		if parentJobID != "" {
			var parent models.Job
			if err := tx.Where("id = ?", parentJobID).First(&parent).Error; err != nil {
				return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "parent-job-missing")
			}
			parents = append(parents, parentJobID)
		}
		acceptance := jobs.Acceptance{Kind: JobKindPluginCommandImport, KindVersion: jobPluginCommandVersion,
			State: state, ActorUserID: copyUintPtr(row.CreatedByUserId), Origin: "plugin",
			Title: pluginCommandImportJobTitle(row.FileName), Replay: jobs.ReplayInput{Input: commandImportReplay(row)}, Parents: parents,
			LegacyRefs: []jobs.LegacyRef{{Namespace: pluginCommandImportHandleNamespace, Handle: row.ID}}}
		deps := ctx.jobDepsWithDB(tx)
		deps.Now = func() time.Time { return row.CreatedAt.UTC() }
		purgeReason := ""
		if terminal := isTerminalPluginImport(row.Status); terminal && finished != nil && ctx.JobReplayRetention() > 0 && !finished.Add(ctx.JobReplayRetention()).After(now) {
			purgeReason = models.JobReplayPurgeExpired
		}
		imported, err := ctx.JobService().ImportLegacy(deps, jobs.LegacyImport{Acceptance: acceptance,
			State: state, AcceptedAt: row.CreatedAt.UTC(), StartedAt: row.StartedAt, FinishedAt: finished,
			Failure: failure, PurgeReplayReason: purgeReason})
		if err != nil {
			return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "canonical-job-import-failed")
		}
		mapping.JobID = imported.ID
		if purgeReason != "" {
			at := now
			mapping.Status, mapping.PurgedAt, mapping.PurgeReason = models.JobSourceMappingPurged, &at, purgeReason
		}
		if err := persistCommandImportJobID(tx, &row, imported.ID); err != nil {
			return errors.New("plugin command import canonical Job identity could not be backfilled")
		}
		if resumeQuarantine {
			mapping.Status, mapping.BlockerCode = models.JobSourceMappingCopied, ""
			mapping.VerifiedAt, mapping.ScrubbedAt, mapping.PostScrubHash = nil, nil, ""
			mapping.PurgedAt, mapping.PurgeReason = nil, ""
			mapping.UpdatedAt = now
			return tx.Save(&mapping).Error
		}
		return tx.Create(&mapping).Error
	})
}

func pluginCommandImportJobTitle(fileName string) string {
	title := "Import " + fileName
	for len(title) > jobs.MaxTitleBytes || !utf8.ValidString(title) {
		title = title[:len(title)-1]
	}
	return title
}

const jobMigrationPluginCommandImportResourceOutput = "imported-resource"

type blankPluginCommandImportEvidence struct {
	ParentJobID string
	ResourceID  uint
}

// proveBlankPluginCommandImport accepts only the narrow history shape whose
// successful result can be established without its originally accepted fields.
// The import row still owns its lifecycle timestamps; the parent run, import
// map and Resource prove the successful outcome and its lineage.
func (ctx *MahresourcesContext) proveBlankPluginCommandImport(tx *gorm.DB, row models.PluginCommandImport) (*blankPluginCommandImportEvidence, error) {
	if row.FieldsJSON != "" {
		return nil, nil
	}
	if row.Status != plugin_commands.ImportStatusSucceeded || row.FinishedAt == nil || row.FinishedAt.IsZero() {
		return nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-unreadable")
	}
	state, failure, finished, err := pluginImportLegacyOutcome(row)
	if err != nil {
		return nil, err
	}
	if state != jobs.StateSucceeded || failure != nil || finished == nil {
		return nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-unreadable")
	}
	var run models.PluginCommandRun
	if err := tx.Where("id = ?", row.RunID).First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "parent-run-missing")
		}
		return nil, errors.New("plugin command import parent run could not be read")
	}
	if run.Status != plugin_commands.RunStatusSucceeded || run.FinishedAt == nil || run.FinishedAt.IsZero() {
		return nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-unreadable")
	}
	parentJobID, err := pluginCommandRunJobIDForImport(tx, run)
	if err != nil {
		return nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "parent-job-missing")
	}
	var parentJob models.Job
	if err := tx.Where("id = ?", parentJobID).First(&parentJob).Error; err != nil || !pluginCommandRunJobMatches(parentJob, run) {
		return nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "parent-job-missing")
	}
	if !tx.Migrator().HasTable(&models.PluginCommandImportMap{}) || !tx.Migrator().HasTable(&models.Resource{}) {
		return nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-unreadable")
	}
	var importMap models.PluginCommandImportMap
	err = tx.Where("run_id = ? AND file_name = ?", row.RunID, row.FileName).First(&importMap).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-unreadable")
	}
	if err != nil {
		return nil, errors.New("plugin command import result map could not be read")
	}
	if importMap.ImportID != row.ID || importMap.Status != plugin_commands.ImportStatusSucceeded || importMap.Error != "" || importMap.ResourceID == nil || *importMap.ResourceID == 0 {
		return nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-unreadable")
	}
	var resource models.Resource
	if err := tx.Select("id").Where("id = ?", *importMap.ResourceID).First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-unreadable")
		}
		return nil, errors.New("plugin command imported Resource could not be read")
	}
	return &blankPluginCommandImportEvidence{ParentJobID: parentJobID, ResourceID: *importMap.ResourceID}, nil
}

func pluginCommandRunJobIDForImport(tx *gorm.DB, run models.PluginCommandRun) (string, error) {
	if run.JobID == "" {
		return "", gorm.ErrRecordNotFound
	}
	var mapping models.JobSourceMapping
	err := tx.Where("source_kind = ? AND source_id = ?", jobMigrationPluginCommandRun, run.ID).First(&mapping).Error
	if err == nil {
		if mapping.Status == models.JobSourceMappingQuarantined || mapping.JobID != run.JobID {
			return "", errors.New("parent run mapping does not prove a canonical Job")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	var handle models.JobLegacyHandle
	err = tx.Where("namespace = ? AND handle = ?", pluginCommandHandleNamespace, run.ID).First(&handle).Error
	if err == nil {
		if handle.JobID != run.JobID {
			return "", errors.New("parent run handle disagrees with its source")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	return run.JobID, nil
}

func pluginCommandRunJobMatches(job models.Job, run models.PluginCommandRun) bool {
	return job.ID != "" && job.Kind == JobKindPluginCommand && job.KindVersion == jobPluginCommandVersion &&
		job.State == string(jobs.StateSucceeded) && sameJobTime(&job.AcceptedAt, &run.CreatedAt) &&
		sameJobTime(job.StartedAt, run.StartedAt) && sameJobTime(job.FinishedAt, run.FinishedAt)
}

func sameJobTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.UTC().Equal(right.UTC())
}

func (ctx *MahresourcesContext) copyProvenBlankPluginCommandImport(tx *gorm.DB, mapping *models.JobSourceMapping, row *models.PluginCommandImport, evidence blankPluginCommandImportEvidence, now time.Time, resumeQuarantine bool) error {
	jobID, err := findPluginCommandJob(tx, pluginCommandImportHandleNamespace, row.ID, row.JobID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "plugin command import handle lookup failed")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		acceptance := jobs.Acceptance{Kind: JobKindPluginCommandImport, KindVersion: jobPluginCommandVersion,
			State: jobs.StateSucceeded, ActorUserID: copyUintPtr(row.CreatedByUserId), Origin: "plugin",
			Title: pluginCommandImportJobTitle(row.FileName), Replay: jobs.ReplayInput{NonReplayable: true},
			Parents:    []string{evidence.ParentJobID},
			LegacyRefs: []jobs.LegacyRef{{Namespace: pluginCommandImportHandleNamespace, Handle: row.ID}}}
		deps := ctx.jobDepsWithDB(tx)
		deps.Now = func() time.Time { return row.CreatedAt.UTC() }
		imported, importErr := ctx.JobService().ImportLegacy(deps, jobs.LegacyImport{
			Acceptance: acceptance, State: jobs.StateSucceeded, AcceptedAt: row.CreatedAt.UTC(), StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
		})
		if importErr != nil {
			return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "canonical-job-import-failed")
		}
		jobID = imported.ID
	}
	if err := persistCommandImportJobID(tx, row, jobID); err != nil {
		return errors.New("plugin command import canonical Job identity could not be backfilled")
	}
	mapping.JobID = jobID
	if err := ensureProvenPluginCommandImportOutput(tx, jobID, row.FinishedAt.UTC(), evidence.ResourceID); err != nil {
		return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "canonical-replay-mismatch")
	}
	if err := ctx.verifyProvenBlankPluginCommandImport(tx, jobID, *row, evidence); err != nil {
		return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "canonical-replay-mismatch")
	}
	if resumeQuarantine {
		mapping.Status, mapping.BlockerCode = models.JobSourceMappingCopied, ""
		mapping.VerifiedAt, mapping.ScrubbedAt, mapping.PostScrubHash = nil, nil, ""
		mapping.PurgedAt, mapping.PurgeReason = nil, ""
		mapping.UpdatedAt = now
		return tx.Save(mapping).Error
	}
	return tx.Create(mapping).Error
}

func ensureProvenPluginCommandImportOutput(tx *gorm.DB, jobID string, finished time.Time, resourceID uint) error {
	if jobID == "" || resourceID == 0 {
		return errors.New("proven import output is incomplete")
	}
	reference, err := json.Marshal(map[string]uint{"resourceId": resourceID})
	if err != nil {
		return err
	}
	var existing models.JobOutput
	err = tx.Where("job_id = ? AND key = ?", jobID, jobMigrationPluginCommandImportResourceOutput).First(&existing).Error
	createdAt := finished.UTC()
	if err == nil {
		var got map[string]uint
		if existing.Type != string(jobs.OutputTypeEntity) || existing.Label != "Imported Resource" || !existing.Required ||
			existing.Availability != string(jobs.OutputAvailable) || existing.RemovedAt != nil ||
			!existing.CreatedAt.UTC().Equal(createdAt) || json.Unmarshal(existing.Reference, &got) != nil || got["resourceId"] != resourceID {
			return errors.New("canonical import output does not match result evidence")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	} else if err := tx.Create(&models.JobOutput{ID: types.NewUUIDv7(), JobID: jobID, Key: jobMigrationPluginCommandImportResourceOutput,
		Type: string(jobs.OutputTypeEntity), Label: "Imported Resource", Reference: types.JSON(reference), Required: true,
		Availability: string(jobs.OutputAvailable), Version: 1, CreatedAt: createdAt, UpdatedAt: createdAt}).Error; err != nil {
		return err
	}
	return ensureHistoricalPluginCommandImportOutputEvent(tx, jobID, createdAt)
}

func historicalPluginCommandImportOutputDetail() ([]byte, error) {
	return json.Marshal(map[string]string{
		"key":   jobMigrationPluginCommandImportResourceOutput,
		"type":  string(jobs.OutputTypeEntity),
		"state": string(jobs.OutputAvailable),
	})
}

func sameHistoricalPluginCommandImportOutputDetail(actual, expected []byte) bool {
	var actualFields, expectedFields map[string]string
	if json.Unmarshal(actual, &actualFields) != nil || json.Unmarshal(expected, &expectedFields) != nil || len(actualFields) != len(expectedFields) {
		return false
	}
	for key, expectedValue := range expectedFields {
		if actualValue, ok := actualFields[key]; !ok || actualValue != expectedValue {
			return false
		}
	}
	return true
}

func ensureHistoricalPluginCommandImportOutputEvent(tx *gorm.DB, jobID string, publishedAt time.Time) error {
	if jobID == "" {
		return errors.New("historical import output event has no Job")
	}
	detail, err := historicalPluginCommandImportOutputDetail()
	if err != nil {
		return err
	}
	var job models.Job
	if err := tx.Where("id = ?", jobID).First(&job).Error; err != nil {
		return err
	}
	var existing []models.JobEvent
	if err := tx.Where("job_id = ? AND type = ?", jobID, jobs.EventOutputPublished).Find(&existing).Error; err != nil {
		return err
	}
	for _, event := range existing {
		var got struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(event.Detail, &got); err != nil {
			return errors.New("canonical import output event is unreadable")
		}
		if got.Key != jobMigrationPluginCommandImportResourceOutput {
			continue
		}
		if !sameHistoricalPluginCommandImportOutputDetail(event.Detail, detail) || !event.CreatedAt.UTC().Equal(publishedAt.UTC()) {
			return errors.New("canonical import output event does not match result evidence")
		}
		return verifyHistoricalPluginCommandImportOutputEvent(tx, job, event, publishedAt)
	}
	var terminal models.JobEvent
	if err := tx.Where("job_id = ? AND type = ?", jobID, jobs.EventSucceeded).Order("sequence DESC").First(&terminal).Error; err != nil {
		return errors.New("canonical import success event is missing")
	}
	if terminal.JobVersion != job.Version || !terminal.CreatedAt.UTC().Equal(publishedAt.UTC()) || terminal.Sequence == 0 {
		return errors.New("canonical import success event does not match source outcome")
	}
	var last *uint64
	if err := tx.Model(&models.JobEvent{}).Where("job_id = ?", jobID).Select("MAX(sequence)").Scan(&last).Error; err != nil {
		return err
	}
	if last == nil || *last == ^uint64(0) {
		return errors.New("canonical import timeline has no appendable event sequence")
	}
	if *last < terminal.Sequence {
		return errors.New("canonical import success event is beyond its timeline")
	}
	return tx.Create(&models.JobEvent{ID: types.NewUUIDv7(), JobID: jobID, Sequence: *last + 1, JobVersion: job.Version,
		Type: jobs.EventOutputPublished, Detail: types.JSON(detail), CreatedAt: publishedAt.UTC()}).Error
}

func verifyHistoricalPluginCommandImportOutputEvent(tx *gorm.DB, job models.Job, publication models.JobEvent, finished time.Time) error {
	var terminal models.JobEvent
	if err := tx.Where("job_id = ? AND type = ?", job.ID, jobs.EventSucceeded).Order("sequence DESC").First(&terminal).Error; err != nil {
		return errors.New("canonical import success event is missing")
	}
	if terminal.JobVersion == 0 || terminal.JobVersion != job.Version || terminal.Sequence == 0 ||
		!terminal.CreatedAt.UTC().Equal(finished.UTC()) || !publication.CreatedAt.UTC().Equal(finished.UTC()) {
		return errors.New("canonical import output event does not match its terminal outcome")
	}
	// A publication already recorded by the runtime may precede success in its
	// original timeline. Keep accepting that immutable history. New recovery
	// events are appended after the current tail with the terminal Job version.
	preTerminal := publication.Sequence < terminal.Sequence && publication.JobVersion == terminal.JobVersion-1
	postTerminal := publication.Sequence > terminal.Sequence && publication.JobVersion == job.Version
	if !preTerminal && !postTerminal {
		return errors.New("canonical import output event is inconsistent with its success event")
	}
	return nil
}

func verifyHistoricalPluginCommandImportOutput(tx *gorm.DB, jobID string, finished time.Time, resourceID uint) error {
	if jobID == "" || resourceID == 0 {
		return errors.New("proven import output is incomplete")
	}
	var output models.JobOutput
	if err := tx.Where("job_id = ? AND key = ?", jobID, jobMigrationPluginCommandImportResourceOutput).First(&output).Error; err != nil {
		return err
	}
	var got map[string]uint
	if output.Type != string(jobs.OutputTypeEntity) || output.Label != "Imported Resource" || !output.Required ||
		output.Availability != string(jobs.OutputAvailable) || output.RemovedAt != nil ||
		!output.CreatedAt.UTC().Equal(finished.UTC()) || json.Unmarshal(output.Reference, &got) != nil || got["resourceId"] != resourceID {
		return errors.New("canonical import output does not match result evidence")
	}
	var job models.Job
	if err := tx.Where("id = ?", jobID).First(&job).Error; err != nil {
		return err
	}
	detail, err := historicalPluginCommandImportOutputDetail()
	if err != nil {
		return err
	}
	var events []models.JobEvent
	if err := tx.Where("job_id = ? AND type = ?", jobID, jobs.EventOutputPublished).Find(&events).Error; err != nil {
		return err
	}
	for _, event := range events {
		var got struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(event.Detail, &got); err != nil {
			return errors.New("canonical import output event is unreadable")
		}
		if got.Key != jobMigrationPluginCommandImportResourceOutput {
			continue
		}
		if sameHistoricalPluginCommandImportOutputDetail(event.Detail, detail) && event.CreatedAt.UTC().Equal(finished.UTC()) {
			return verifyHistoricalPluginCommandImportOutputEvent(tx, job, event, finished)
		}
		return errors.New("canonical import output event does not match result evidence")
	}
	return errors.New("canonical import output publication is missing from the Job timeline")
}

func (ctx *MahresourcesContext) verifyProvenBlankPluginCommandImport(tx *gorm.DB, jobID string, row models.PluginCommandImport, evidence blankPluginCommandImportEvidence) error {
	if jobID == "" || (row.JobID != "" && row.JobID != jobID) {
		return errors.New("import source and canonical Job identity disagree")
	}
	var job models.Job
	if err := tx.Where("id = ?", jobID).First(&job).Error; err != nil {
		return err
	}
	if job.Kind != JobKindPluginCommandImport || job.KindVersion != jobPluginCommandVersion || job.State != string(jobs.StateSucceeded) || jobs.ReplayClass(job.ReplayClass) != jobs.ReplayClassNonReplayable || !sameJobTime(&job.AcceptedAt, &row.CreatedAt) || !sameJobTime(job.StartedAt, row.StartedAt) || !sameJobTime(job.FinishedAt, row.FinishedAt) || !sameOptionalUint(job.ActorUserID, row.CreatedByUserId) {
		return errors.New("canonical import outcome differs from source")
	}
	var envelope models.JobReplayEnvelope
	err := tx.Where("job_id = ?", jobID).First(&envelope).Error
	if err == nil {
		return errors.New("non-replayable historical import has a replay envelope")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var link models.JobLink
	if err := tx.Where("type = ? AND from_job_id = ? AND to_job_id = ?", models.JobLinkParentChild, evidence.ParentJobID, jobID).First(&link).Error; err != nil {
		return err
	}
	return verifyHistoricalPluginCommandImportOutput(tx, jobID, row.FinishedAt.UTC(), evidence.ResourceID)
}

func persistCommandSourceJobID(tx *gorm.DB, row *models.PluginCommandRun, jobID string) error {
	if row.JobID != "" || jobID == "" {
		return nil
	}
	result := tx.Model(&models.PluginCommandRun{}).Where("id = ? AND (job_id = '' OR job_id IS NULL)", row.ID).Update("job_id", jobID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("source Job identity changed during migration")
	}
	row.JobID = jobID
	return nil
}

func persistCommandImportJobID(tx *gorm.DB, row *models.PluginCommandImport, jobID string) error {
	if row.JobID != "" || jobID == "" {
		return nil
	}
	result := tx.Model(&models.PluginCommandImport{}).Where("id = ? AND (job_id = '' OR job_id IS NULL)", row.ID).Update("job_id", jobID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("source Job identity changed during migration")
	}
	row.JobID = jobID
	return nil
}

func pluginImportLegacyOutcome(row models.PluginCommandImport) (jobs.State, *jobs.Failure, *time.Time, error) {
	switch row.Status {
	case plugin_commands.ImportStatusPending:
		return jobs.StateQueued, nil, nil, nil
	case plugin_commands.ImportStatusSucceeded:
		return jobs.StateSucceeded, nil, row.FinishedAt, requireImportFinish(row.ID, row.FinishedAt)
	case plugin_commands.ImportStatusFailed:
		return jobs.StateFailed, &jobs.Failure{Code: "plugin-command-import-failed", Class: jobs.FailureClassInternal, Message: "the plugin command import did not complete successfully"}, row.FinishedAt, requireImportFinish(row.ID, row.FinishedAt)
	case plugin_commands.ImportStatusCancelled:
		return jobs.StateCancelled, nil, row.FinishedAt, requireImportFinish(row.ID, row.FinishedAt)
	case plugin_commands.ImportStatusInterrupted:
		return jobs.StateInterrupted, nil, row.FinishedAt, requireImportFinish(row.ID, row.FinishedAt)
	case plugin_commands.ImportStatusRunning:
		return "", nil, nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "running-source-without-canonical-job")
	default:
		return "", nil, nil, migrationBlocker(jobMigrationPluginCommandImport, row.ID, "outcome-unknown")
	}
}

func requireImportFinish(sourceID string, finished *time.Time) error {
	if finished == nil || finished.IsZero() {
		return migrationBlocker(jobMigrationPluginCommandImport, sourceID, "outcome-time-unproven")
	}
	return nil
}

func isTerminalPluginImport(status string) bool {
	return status == plugin_commands.ImportStatusSucceeded || status == plugin_commands.ImportStatusFailed ||
		status == plugin_commands.ImportStatusCancelled || status == plugin_commands.ImportStatusInterrupted
}

func (ctx *MahresourcesContext) verifyPluginCommandImportReplay(tx *gorm.DB, jobID string, row models.PluginCommandImport) error {
	opened, err := ctx.JobService().OpenReplay(ctx.jobDepsWithDB(tx), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return err
	}
	var got pluginCommandImportReplayInput
	if err := json.Unmarshal(opened.Input, &got); err != nil {
		return err
	}
	want := pluginCommandImportReplayInput{RunID: row.RunID, FileName: row.FileName, FieldsJSON: row.FieldsJSON, PluginGeneration: row.PluginGeneration}
	if got != want {
		return fmt.Errorf("canonical replay differs from source")
	}
	var job models.Job
	if err := tx.Where("id = ?", jobID).First(&job).Error; err != nil {
		return err
	}
	wantState, _, _, stateErr := pluginImportLegacyOutcome(row)
	if stateErr != nil && row.Status != plugin_commands.ImportStatusRunning {
		return stateErr
	}
	if row.Status == plugin_commands.ImportStatusRunning {
		if job.State != string(jobs.StateRunning) && job.State != string(jobs.StateBlocked) {
			return fmt.Errorf("running source and canonical Job state disagree")
		}
	} else if job.State != string(wantState) {
		return fmt.Errorf("source and canonical Job outcomes disagree")
	}
	return nil
}

func (ctx *MahresourcesContext) verifyPluginCommandRunsBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	return ctx.verifyCommandSourceBatch(jobMigrationPluginCommandRun, cursor, limit, now)
}

func (ctx *MahresourcesContext) verifyPluginCommandImportsBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	return ctx.verifyCommandSourceBatch(jobMigrationPluginCommandImport, cursor, limit, now)
}

func (ctx *MahresourcesContext) verifyPluginCommandImportSource(tx *gorm.DB, jobID string, row models.PluginCommandImport) error {
	if row.FieldsJSON != "" {
		return ctx.verifyPluginCommandImportReplay(tx, jobID, row)
	}
	evidence, err := ctx.proveBlankPluginCommandImport(tx, row)
	if err != nil {
		return err
	}
	if evidence == nil {
		return migrationBlocker(jobMigrationPluginCommandImport, row.ID, "source-input-unreadable")
	}
	return ctx.verifyProvenBlankPluginCommandImport(tx, jobID, row, *evidence)
}

func (ctx *MahresourcesContext) verifyCommandSourceBatch(kind, cursor string, limit int, now time.Time) (bool, string, error) {
	query := ctx.db.Where("source_kind = ? AND status IN ?", kind,
		[]string{models.JobSourceMappingCopied, models.JobSourceMappingVerified}).Order("source_id ASC").Limit(limit)
	if cursor != "" {
		query = query.Where("source_id > ?", cursor)
	}
	var mappings []models.JobSourceMapping
	if err := query.Find(&mappings).Error; err != nil {
		return false, cursor, errors.New("plugin command mappings could not be read for verification")
	}
	for _, mapping := range mappings {
		switch kind {
		case jobMigrationPluginCommandRun:
			var row models.PluginCommandRun
			if err := ctx.db.Where("id = ?", mapping.SourceID).First(&row).Error; err != nil {
				return false, mapping.SourceID, migrationBlocker(kind, mapping.SourceID, "source-row-missing")
			}
			if hashPluginCommandRun(row) != mapping.SourceHash {
				return false, mapping.SourceID, quarantineJobSource(ctx.db, &mapping, "plugin command input hash changed before verification")
			}
			if err := ctx.verifyPluginCommandRunReplay(ctx.db, mapping.JobID, row); err != nil {
				return false, mapping.SourceID, quarantineJobSource(ctx.db, &mapping, "plugin command replay did not verify")
			}
		case jobMigrationPluginCommandImport:
			var row models.PluginCommandImport
			if err := ctx.db.Where("id = ?", mapping.SourceID).First(&row).Error; err != nil {
				return false, mapping.SourceID, migrationBlocker(kind, mapping.SourceID, "source-row-missing")
			}
			if hashPluginCommandImport(row) != mapping.SourceHash {
				return false, mapping.SourceID, quarantineJobSource(ctx.db, &mapping, "plugin command import hash changed before verification")
			}
			if err := ctx.verifyPluginCommandImportSource(ctx.db, mapping.JobID, row); err != nil {
				return false, mapping.SourceID, quarantineJobSource(ctx.db, &mapping, "plugin command import outcome did not verify")
			}
		}
		mapping.Status, mapping.VerifiedAt, mapping.UpdatedAt = models.JobSourceMappingVerified, &now, now
		if err := ctx.db.Save(&mapping).Error; err != nil {
			return false, mapping.SourceID, errors.New("plugin command verification marker could not be stored")
		}
	}
	if len(mappings) == limit {
		return true, mappings[len(mappings)-1].SourceID, nil
	}
	return false, "", nil
}

func (ctx *MahresourcesContext) scrubPluginCommandRunsBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	return ctx.scrubCommandSourceBatch(jobMigrationPluginCommandRun, cursor, limit, now)
}

func (ctx *MahresourcesContext) scrubPluginCommandImportsBatch(cursor string, limit int, now time.Time) (bool, string, error) {
	return ctx.scrubCommandSourceBatch(jobMigrationPluginCommandImport, cursor, limit, now)
}

func (ctx *MahresourcesContext) scrubCommandSourceBatch(kind, cursor string, limit int, now time.Time) (bool, string, error) {
	query := ctx.db.Where("source_kind = ? AND (status = ? OR (status = ? AND scrubbed_at IS NULL))",
		kind, models.JobSourceMappingVerified, models.JobSourceMappingPurged).Order("source_id ASC").Limit(limit)
	if cursor != "" {
		query = query.Where("source_id > ?", cursor)
	}
	var mappings []models.JobSourceMapping
	if err := query.Find(&mappings).Error; err != nil {
		return false, cursor, errors.New("plugin command scrub mappings could not be read")
	}
	for _, mapping := range mappings {
		var changedAfterVerification bool
		err := ctx.db.Transaction(func(tx *gorm.DB) error {
			var current models.JobSourceMapping
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("source_kind = ? AND source_id = ?", kind, mapping.SourceID).First(&current).Error; err != nil {
				return errors.New("plugin command scrub mapping disappeared")
			}
			if current.Status != models.JobSourceMappingVerified &&
				!(current.Status == models.JobSourceMappingPurged && current.ScrubbedAt == nil) {
				return nil
			}
			var afterHash string
			switch kind {
			case jobMigrationPluginCommandRun:
				var row models.PluginCommandRun
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", current.SourceID).First(&row).Error; err != nil {
					return errors.New("plugin command run disappeared before scrub")
				}
				if current.Status != models.JobSourceMappingPurged && hashPluginCommandRun(row) != current.SourceHash {
					if err := quarantineJobSource(tx, &current, "plugin command input changed after verification"); err != nil {
						return err
					}
					changedAfterVerification = true
					return nil
				}
				if err := tx.Model(&models.PluginCommandRun{}).Where("id = ?", row.ID).
					Updates(map[string]any{"params_json": "", "inputs_json": ""}).Error; err != nil {
					return errors.New("plugin command run plaintext scrub failed")
				}
				var after models.PluginCommandRun
				if err := tx.Where("id = ?", row.ID).First(&after).Error; err != nil {
					return errors.New("plugin command run could not be reread after scrub")
				}
				afterHash = hashRetiredPluginCommandRun(after)
			case jobMigrationPluginCommandImport:
				var row models.PluginCommandImport
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", current.SourceID).First(&row).Error; err != nil {
					return errors.New("plugin command import disappeared before scrub")
				}
				if current.Status != models.JobSourceMappingPurged && hashPluginCommandImport(row) != current.SourceHash {
					if err := quarantineJobSource(tx, &current, "plugin command import changed after verification"); err != nil {
						return err
					}
					changedAfterVerification = true
					return nil
				}
				if err := tx.Model(&models.PluginCommandImport{}).Where("id = ?", row.ID).
					Update("fields_json", "").Error; err != nil {
					return errors.New("plugin command import plaintext scrub failed")
				}
				var after models.PluginCommandImport
				if err := tx.Where("id = ?", row.ID).First(&after).Error; err != nil {
					return errors.New("plugin command import could not be reread after scrub")
				}
				afterHash = hashRetiredPluginCommandImport(after)
			default:
				return errors.New("unknown plugin command migration source")
			}
			at := now
			if current.Status != models.JobSourceMappingPurged {
				current.Status = models.JobSourceMappingScrubbed
			}
			current.ScrubbedAt, current.PostScrubHash, current.UpdatedAt = &at, afterHash, now
			if err := tx.Save(&current).Error; err != nil {
				return errors.New("plugin command scrub marker could not be stored")
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

func (ctx *MahresourcesContext) quarantinePluginCommandImport(row models.PluginCommandImport, code string, now time.Time) error {
	mapping := models.JobSourceMapping{SourceKind: jobMigrationPluginCommandImport, SourceID: row.ID,
		SourceRevision: 1, SourceHash: hashPluginCommandImport(row), Status: models.JobSourceMappingQuarantined,
		BlockerCode: code, Origin: models.JobSourceOriginBackfilled, CopiedAt: now, CreatedAt: now, UpdatedAt: now}
	if row.JobID != "" {
		mapping.JobID = row.JobID
	}
	return ctx.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_kind"}, {Name: "source_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "blocker_code", "source_hash", "updated_at"})}).Create(&mapping).Error
}

func commandMappingID(kind string, sourceID string) string {
	return kind + ":" + strconv.Quote(sourceID)
}
