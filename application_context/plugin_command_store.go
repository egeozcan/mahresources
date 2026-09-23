package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"mahresources/constants"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_commands"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errPluginCommandTransitionLost = errors.New("plugin command transition lost")

var _ plugin_commands.Store = (*MahresourcesContext)(nil)
var _ plugin_commands.PendingImportCanceller = (*MahresourcesContext)(nil)

func (ctx *MahresourcesContext) requestRunCancelTx(id, reason string) error {
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		err = ctx.db.Transaction(func(tx *gorm.DB) error {
			if fenceErr := ctx.requirePluginCommandFenceTx(tx); fenceErr != nil {
				return fenceErr
			}
			res := tx.Model(&models.PluginCommandRun{}).
				Where("id = ? AND status IN ? AND cancel_requested = ?", id,
					[]string{plugin_commands.RunStatusQueued, plugin_commands.RunStatusRunning}, false).
				Updates(map[string]any{"cancel_requested": true, "error": reason})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 1 {
				return nil
			}
			var count int64
			if queryErr := tx.Model(&models.PluginCommandRun{}).Where("id = ?", id).Count(&count).Error; queryErr != nil {
				return queryErr
			}
			if count == 0 {
				return plugin_commands.ErrRunNotFound
			}
			return plugin_commands.ErrRunNotCancellable
		})
		if err == nil || ctx.db.Dialector.Name() != "sqlite" || !sqliteLockContention(err) {
			return err
		}
		// SQLite can reject a transaction that read before another writer
		// committed instead of honoring busy_timeout. Retry after rollback so the
		// cancellation CAS is evaluated against the latest source state.
		time.Sleep(time.Duration(5*(1<<attempt)) * time.Millisecond)
	}
	return err
}

func sqliteLockContention(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database is busy")
}

func retryPluginCommandSQLiteWrite(ctx *MahresourcesContext, write func() error) error {
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		err = write()
		if err == nil || ctx.db.Dialector.Name() != "sqlite" || !sqliteLockContention(err) {
			return err
		}
		time.Sleep(time.Duration(5*(1<<attempt)) * time.Millisecond)
	}
	return err
}

func copyCommandUint(value *uint) *uint {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func runModel(record plugin_commands.RunRecord) models.PluginCommandRun {
	return models.PluginCommandRun{
		ID: record.ID, JobID: record.JobID, JobExecutionToken: record.JobExecutionToken, PluginName: record.PluginName, CommandName: record.CommandName,
		ParamsJSON: record.ParamsJSON, InputsJSON: encodeSuppliedInputs(record.Inputs), Status: record.Status, ExitCode: record.ExitCode,
		Error: record.Error, ProcessGroupID: record.ProcessGroupID,
		CancelRequested: record.CancelRequested, OutputUnverified: record.OutputUnverified,
		ActorlessAtSubmission: record.ActorlessAtSubmission,
		CreatedByUserId:       copyCommandUint(record.CreatedByUserID), CreatedAt: record.CreatedAt,
		StartedAt: record.StartedAt, FinishedAt: record.FinishedAt, ExchangeRemovedAt: record.ExchangeRemovedAt,
	}
}

func runRecord(row models.PluginCommandRun) plugin_commands.RunRecord {
	return plugin_commands.RunRecord{
		ID: row.ID, JobID: row.JobID, JobExecutionToken: row.JobExecutionToken, PluginName: row.PluginName, CommandName: row.CommandName,
		ParamsJSON: row.ParamsJSON, Inputs: decodeSuppliedInputs(row.ID, row.InputsJSON), Status: row.Status, ExitCode: row.ExitCode,
		Error: row.Error, ProcessGroupID: row.ProcessGroupID,
		CancelRequested: row.CancelRequested, OutputUnverified: row.OutputUnverified,
		ActorlessAtSubmission: row.ActorlessAtSubmission,
		CreatedByUserID:       copyCommandUint(row.CreatedByUserId), CreatedAt: row.CreatedAt,
		StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, ExchangeRemovedAt: row.ExchangeRemovedAt,
	}
}

func outputModel(output plugin_commands.RunOutput) models.PluginCommandRunOutput {
	return models.PluginCommandRunOutput{RunID: output.RunID, ArgvJSON: output.ArgvJSON, OutputTail: output.OutputTail, CreatedAt: output.CreatedAt}
}

// encodeSuppliedInputs stores the names and sizes of a run's input files. A run
// with none stores nothing at all rather than an empty array, and metadata that
// somehow cannot be encoded (it is a name and a count) is dropped rather than
// failing the run's admission.
func encodeSuppliedInputs(inputs []plugin_commands.SuppliedInput) string {
	if len(inputs) == 0 {
		return ""
	}
	encoded, err := json.Marshal(inputs)
	if err != nil {
		log.Printf("plugin command: encode supplied inputs: %v", err)
		return ""
	}
	return string(encoded)
}

// decodeSuppliedInputs reads the record back. A column that cannot be read is
// metadata trouble, not a reason to fail a history page, so it reads as empty
// with one log line.
func decodeSuppliedInputs(runID, encoded string) []plugin_commands.SuppliedInput {
	if encoded == "" {
		return nil
	}
	var inputs []plugin_commands.SuppliedInput
	if err := json.Unmarshal([]byte(encoded), &inputs); err != nil {
		log.Printf("plugin command %s: unreadable supplied inputs record: %v", runID, err)
		return nil
	}
	return inputs
}

func pluginCommandInputsRetired(db *gorm.DB) (bool, error) {
	if err := models.EnsureJobWriterEpoch(db); err != nil {
		return false, err
	}
	epoch, err := models.JobWriterEpochMinimum(db)
	if err != nil {
		return false, err
	}
	return epoch >= models.JobWriterEpochRetiredPlaintext, nil
}

func (ctx *MahresourcesContext) hydratePluginCommandRun(row *models.PluginCommandRun, db *gorm.DB) error {
	if row == nil {
		return nil
	}
	retired, err := pluginCommandInputsRetired(db)
	if err != nil {
		return err
	}
	if !retired {
		return nil
	}
	if row.JobID == "" || ctx.JobService() == nil {
		return fmt.Errorf("plugin command replay input is unavailable")
	}
	opened, err := ctx.JobService().OpenReplay(ctx.jobDepsWithDB(db), jobs.Access{Administrator: true}, row.JobID)
	if err != nil {
		return fmt.Errorf("plugin command replay input is unavailable")
	}
	var input pluginCommandRunReplayInput
	if err := json.Unmarshal(opened.Input, &input); err != nil || input.PluginName != row.PluginName || input.CommandName != row.CommandName {
		return fmt.Errorf("plugin command replay input does not match its source")
	}
	row.ParamsJSON, row.InputsJSON = input.ParamsJSON, input.InputsJSON
	return nil
}

func (ctx *MahresourcesContext) hydratePluginCommandImport(row *models.PluginCommandImport, db *gorm.DB) error {
	if row == nil {
		return nil
	}
	retired, err := pluginCommandInputsRetired(db)
	if err != nil {
		return err
	}
	if !retired {
		return nil
	}
	if row.JobID == "" || ctx.JobService() == nil {
		return fmt.Errorf("plugin command import replay input is unavailable")
	}
	opened, err := ctx.JobService().OpenReplay(ctx.jobDepsWithDB(db), jobs.Access{Administrator: true}, row.JobID)
	if err != nil {
		return fmt.Errorf("plugin command import replay input is unavailable")
	}
	var input pluginCommandImportReplayInput
	if err := json.Unmarshal(opened.Input, &input); err != nil || input.RunID != row.RunID || input.FileName != row.FileName {
		return fmt.Errorf("plugin command import replay input does not match its source")
	}
	row.FieldsJSON = input.FieldsJSON
	return nil
}

func (ctx *MahresourcesContext) acceptAndPersistPluginCommandImportTx(tx *gorm.DB, run models.PluginCommandRun, claim models.PluginCommandImport, retryOfJobID string) (string, error) {
	jobID, err := ctx.acceptPluginCommandImportJob(tx, run, claim, retryOfJobID)
	if err != nil {
		return "", err
	}
	retired, err := pluginCommandInputsRetired(tx)
	if err != nil {
		return "", err
	}
	claim.JobID = jobID
	if retired {
		claim.FieldsJSON = ""
	}
	if err := tx.Create(&claim).Error; err != nil {
		return "", err
	}
	if jobID != "" {
		if err := ctx.recordDualPublishedPluginCommandImportTx(tx, claim, retired, claim.CreatedAt.UTC()); err != nil {
			return "", err
		}
	}
	return jobID, nil
}

func outputRecord(row models.PluginCommandRunOutput) plugin_commands.RunOutput {
	return plugin_commands.RunOutput{RunID: row.RunID, ArgvJSON: row.ArgvJSON, OutputTail: row.OutputTail, CreatedAt: row.CreatedAt}
}

func importRecord(row models.PluginCommandImport) plugin_commands.ImportRecord {
	return plugin_commands.ImportRecord{
		ID: row.ID, JobID: row.JobID, JobExecutionToken: row.JobExecutionToken, RunID: row.RunID, FileName: row.FileName, FieldsJSON: row.FieldsJSON,
		PluginGeneration: row.PluginGeneration, CreatedByUserID: copyCommandUint(row.CreatedByUserId),
		Status: row.Status, Error: row.Error, SourceDeletePending: row.SourceDeletePending,
		CreatedAt: row.CreatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
	}
}

func importMapEntry(row models.PluginCommandImportMap) plugin_commands.ImportMapEntry {
	return plugin_commands.ImportMapEntry{
		RunID: row.RunID, FileName: row.FileName, ImportID: row.ImportID,
		ResourceID: copyCommandUint(row.ResourceID), Status: row.Status, Error: row.Error,
		SourceDeletePending: row.SourceDeletePending,
	}
}

func (ctx *MahresourcesContext) CreateRun(record plugin_commands.RunRecord, output plugin_commands.RunOutput) error {
	if record.ID == "" || output.RunID != record.ID {
		return fmt.Errorf("plugin command run and output ids must match")
	}
	if record.Status != plugin_commands.RunStatusQueued {
		return fmt.Errorf("new plugin command run must be %q", plugin_commands.RunStatusQueued)
	}
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		retired, err := pluginCommandInputsRetired(tx)
		if err != nil {
			return err
		}
		row := runModel(record)
		if retired {
			row.ParamsJSON, row.InputsJSON = "", ""
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		// Intentional actorlessness is durable provenance. In no-auth mode the
		// global create callback attributes context-less rows to root, so clear
		// that automatic stamp inside this same transaction only for a request
		// that explicitly records actorless provenance.
		if record.ActorlessAtSubmission {
			if err := tx.Model(&models.PluginCommandRun{}).Where("id = ?", record.ID).
				Update("created_by_user_id", nil).Error; err != nil {
				return err
			}
		}
		out := outputModel(output)
		if err := tx.Create(&out).Error; err != nil {
			return err
		}
		jobID, err := ctx.acceptPluginCommandRunJob(tx, record)
		if err != nil {
			return err
		}
		if jobID != "" {
			if err := tx.Model(&models.PluginCommandRun{}).Where("id = ?", record.ID).Update("job_id", jobID).Error; err != nil {
				return err
			}
			row.JobID = jobID
			return ctx.recordDualPublishedPluginCommandRunTx(tx, row, retired, record.CreatedAt.UTC())
		}
		return nil
	})
}

func (ctx *MahresourcesContext) MarkRunRunning(id string, started time.Time) (bool, error) {
	var won bool
	err := retryPluginCommandSQLiteWrite(ctx, func() error {
		return ctx.db.Transaction(func(tx *gorm.DB) error {
			if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
				return err
			}
			updates := map[string]any{"status": plugin_commands.RunStatusRunning, "started_at": started}
			if ctx.JobService() != nil {
				updates["job_execution_token"] = tx.Model(&models.Job{}).Select("execution_token").Where("jobs.id = plugin_command_runs.job_id")
			}
			res := tx.Model(&models.PluginCommandRun{}).Where("id = ? AND status = ?", id, plugin_commands.RunStatusQueued).Updates(updates)
			if res.Error != nil {
				return res.Error
			}
			won = res.RowsAffected == 1
			return nil
		})
	})
	return won, err
}

func (ctx *MahresourcesContext) SetRunProcessGroup(id string, pgid int, bootSessionID string) error {
	if pgid <= 0 {
		return fmt.Errorf("plugin command process group must be positive")
	}
	err := retryPluginCommandSQLiteWrite(ctx, func() error {
		return ctx.db.Transaction(func(tx *gorm.DB) error {
			if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
				return err
			}
			res := tx.Model(&models.PluginCommandRun{}).
				Where("id = ? AND status = ? AND process_group_id IS NULL", id, plugin_commands.RunStatusRunning).
				Updates(map[string]any{
					"process_group_id": pgid,
					"boot_session_id":  bootSessionID,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return fmt.Errorf("plugin command run %q is not awaiting a process group", id)
			}
			return nil
		})
	})
	return err
}

func (ctx *MahresourcesContext) RequestRunCancel(id, reason string) error {
	return ctx.requestRunCancelTx(id, reason)
}

func (ctx *MahresourcesContext) FinishRun(id string, finish plugin_commands.RunFinish) (bool, error) {
	if !plugin_commands.RunStatusTerminal(finish.Status) {
		return false, fmt.Errorf("invalid terminal plugin command status %q", finish.Status)
	}
	priorStatuses := []string{plugin_commands.RunStatusRunning}
	if finish.Status == plugin_commands.RunStatusCancelled || finish.Status == plugin_commands.RunStatusInterrupted {
		priorStatuses = append(priorStatuses, plugin_commands.RunStatusQueued)
	}
	err := retryPluginCommandSQLiteWrite(ctx, func() error {
		return ctx.db.Transaction(func(tx *gorm.DB) error {
			if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
				return err
			}
			var prior models.PluginCommandRun
			if err := tx.Where("id = ?", id).First(&prior).Error; err != nil {
				return err
			}
			query := tx.Model(&models.PluginCommandRun{}).
				Where("id = ? AND status IN ?", id, priorStatuses)
			if finish.Status == plugin_commands.RunStatusSucceeded || finish.Status == plugin_commands.RunStatusFailed {
				query = query.Where("cancel_requested = ?", false)
			}
			res := query.Updates(map[string]any{
				"status": finish.Status, "exit_code": finish.ExitCode, "error": finish.Error,
				"output_unverified": finish.OutputUnverified, "finished_at": finish.FinishedAt,
			})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return errPluginCommandTransitionLost
			}
			out := tx.Model(&models.PluginCommandRunOutput{}).Where("run_id = ?", id).
				Update("output_tail", finish.OutputTail)
			if out.Error != nil {
				return out.Error
			}
			if out.RowsAffected != 1 {
				return fmt.Errorf("plugin command run %q has no output row", id)
			}
			if prior.JobID != "" && ctx.JobService() != nil {
				if err := ctx.finishPluginCommandJobTx(tx, prior.JobID, prior.JobExecutionToken, finish.Status, prior.ID); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if errors.Is(err, errPluginCommandTransitionLost) {
		return false, nil
	}
	return err == nil, err
}

func (ctx *MahresourcesContext) Run(id string) (plugin_commands.RunRecord, plugin_commands.RunOutput, error) {
	var row models.PluginCommandRun
	if err := ctx.db.Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return plugin_commands.RunRecord{}, plugin_commands.RunOutput{}, plugin_commands.ErrRunNotFound
		}
		return plugin_commands.RunRecord{}, plugin_commands.RunOutput{}, err
	}
	if err := ctx.hydratePluginCommandRun(&row, ctx.db); err != nil {
		return plugin_commands.RunRecord{}, plugin_commands.RunOutput{}, err
	}
	var out models.PluginCommandRunOutput
	err := ctx.db.Where("run_id = ?", id).First(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return runRecord(row), plugin_commands.RunOutput{}, nil
	}
	if err != nil {
		return plugin_commands.RunRecord{}, plugin_commands.RunOutput{}, err
	}
	return runRecord(row), outputRecord(out), nil
}

func (ctx *MahresourcesContext) Runs(access plugin_commands.Access) ([]plugin_commands.RunView, error) {
	var rows []models.PluginCommandRun
	query := ctx.db.Order("created_at desc, id desc")
	if !access.Administrator {
		if access.PluginName == "" {
			return []plugin_commands.RunView{}, nil
		}
		// Do not reject a nil actor here. Access.AllowsRun distinguishes the
		// intentional actorless provenance used by auth-off from an owned run
		// whose creator was nulled by deletion. Applying that predicate after the
		// plugin-name query keeps deleted-user rows fail-closed while preserving
		// auth-off reconciliation.
		query = query.Where("plugin_name = ?", access.PluginName)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	visible := make([]models.PluginCommandRun, 0, len(rows))
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		record := runRecord(row)
		if access.AllowsRun(record) {
			if err := ctx.hydratePluginCommandRun(&row, ctx.db); err != nil {
				return nil, err
			}
			visible = append(visible, row)
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return []plugin_commands.RunView{}, nil
	}
	var outputs []models.PluginCommandRunOutput
	if err := ctx.db.Where("run_id IN ?", ids).Find(&outputs).Error; err != nil {
		return nil, err
	}
	var imports []models.PluginCommandImportMap
	if err := ctx.db.Where("run_id IN ?", ids).Find(&imports).Error; err != nil {
		return nil, err
	}
	outputByRun := make(map[string]plugin_commands.RunOutput, len(outputs))
	for _, row := range outputs {
		outputByRun[row.RunID] = outputRecord(row)
	}
	importsByRun := make(map[string][]plugin_commands.ImportMapEntry)
	for _, row := range imports {
		importsByRun[row.RunID] = append(importsByRun[row.RunID], importMapEntry(row))
	}
	result := make([]plugin_commands.RunView, 0, len(visible))
	for _, row := range visible {
		result = append(result, plugin_commands.RunView{RunRecord: runRecord(row), Output: outputByRun[row.ID], Imports: importsByRun[row.ID]})
	}
	return result, nil
}

func (ctx *MahresourcesContext) NonterminalRuns() ([]plugin_commands.RecoveryRun, error) {
	knownStatuses := []string{
		plugin_commands.RunStatusQueued, plugin_commands.RunStatusRunning,
		plugin_commands.RunStatusSucceeded, plugin_commands.RunStatusFailed,
		plugin_commands.RunStatusCancelled, plugin_commands.RunStatusInterrupted,
	}
	var malformed []models.PluginCommandRun
	if err := ctx.db.Where("status NOT IN ?", knownStatuses).
		Order("created_at asc, id asc").Limit(1).Find(&malformed).Error; err != nil {
		return nil, err
	}
	if len(malformed) != 0 {
		return nil, fmt.Errorf("plugin command run %q has unknown status %q", malformed[0].ID, malformed[0].Status)
	}

	var rows []models.PluginCommandRun
	if err := ctx.db.Where("status IN ?", []string{plugin_commands.RunStatusQueued, plugin_commands.RunStatusRunning}).
		Order("created_at asc, id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]plugin_commands.RecoveryRun, len(rows))
	for i, row := range rows {
		if err := ctx.hydratePluginCommandRun(&row, ctx.db); err != nil {
			return nil, err
		}
		result[i] = plugin_commands.RecoveryRun{RunRecord: runRecord(row), BootSessionID: row.BootSessionID}
	}
	return result, nil
}

// QuarantineRun publishes an unproven process-group recovery as a canonical
// blocked Job while retaining its token, claim and capacity. The controller
// calls this only after it acquired the staging lease and database fence.
func (ctx *MahresourcesContext) QuarantineRun(blocker plugin_commands.RecoveryBlocker) error {
	if blocker.RunID == "" {
		return fmt.Errorf("plugin command recovery blocker has no run id")
	}
	return retryPluginCommandSQLiteWrite(ctx, func() error {
		return ctx.db.Transaction(func(tx *gorm.DB) error {
			if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
				return err
			}
			var source models.PluginCommandRun
			if err := tx.Where("id = ?", blocker.RunID).First(&source).Error; err != nil {
				return err
			}
			// Recovery blockers are reported only for running source rows. If the
			// durable outcome already changed, that source no longer owns live work.
			if source.Status != plugin_commands.RunStatusRunning || source.JobID == "" {
				return nil
			}
			if source.JobExecutionToken == "" {
				return fmt.Errorf("plugin command run %s has no canonical execution token", source.ID)
			}
			service := ctx.JobService()
			if service == nil {
				return fmt.Errorf("plugin command Job service is unavailable")
			}
			deps := ctx.jobDeps()
			deps.DB = tx
			_, err := service.QuarantineExternalWork(deps, jobs.ExecutionRef{
				JobID: source.JobID, ExecutionToken: source.JobExecutionToken,
			}, "plugin-command-recovery-unproven")
			return err
		})
	})
}

func terminalCommandStatuses() []string {
	return []string{
		plugin_commands.RunStatusSucceeded, plugin_commands.RunStatusFailed,
		plugin_commands.RunStatusCancelled, plugin_commands.RunStatusInterrupted,
	}
}

func (ctx *MahresourcesContext) expiredTerminalRunsQuery(before time.Time) *gorm.DB {
	return ctx.db.Where("status IN ? AND finished_at IS NOT NULL AND finished_at < ? AND exchange_removed_at IS NULL",
		terminalCommandStatuses(), before)
}

func (ctx *MahresourcesContext) ExpiredTerminalRunBoundary(before time.Time) (*plugin_commands.RetentionCursor, error) {
	var row models.PluginCommandRun
	err := ctx.expiredTerminalRunsQuery(before).Order("finished_at desc, id desc").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &plugin_commands.RetentionCursor{FinishedAt: *row.FinishedAt, RunID: row.ID}, nil
}

func (ctx *MahresourcesContext) ExpiredTerminalRuns(before time.Time, after, through *plugin_commands.RetentionCursor, limit int) ([]plugin_commands.RunRecord, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("plugin command exchange sweep limit must be positive")
	}
	query := ctx.expiredTerminalRunsQuery(before)
	if after != nil {
		query = query.Where("finished_at > ? OR (finished_at = ? AND id > ?)", after.FinishedAt, after.FinishedAt, after.RunID)
	}
	if through != nil {
		query = query.Where("finished_at < ? OR (finished_at = ? AND id <= ?)", through.FinishedAt, through.FinishedAt, through.RunID)
	}
	var rows []models.PluginCommandRun
	if err := query.Order("finished_at asc, id asc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]plugin_commands.RunRecord, len(rows))
	for i, row := range rows {
		result[i] = runRecord(row)
	}
	return result, nil
}

func (ctx *MahresourcesContext) MarkRunExchangeRemoved(runID string, removedAt time.Time) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		result := tx.Model(&models.PluginCommandRun{}).
			Where("id = ? AND status IN ? AND exchange_removed_at IS NULL", runID, terminalCommandStatuses()).
			Update("exchange_removed_at", removedAt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("plugin command run %s exchange removal transition lost", runID)
		}
		if err := tx.Model(&models.PluginCommandImport{}).
			Where("run_id = ? AND status = ? AND source_delete_pending = ?", runID, plugin_commands.ImportStatusSucceeded, true).
			Update("source_delete_pending", false).Error; err != nil {
			return err
		}
		return tx.Model(&models.PluginCommandImportMap{}).
			Where("run_id = ? AND status = ? AND source_delete_pending = ?", runID, plugin_commands.ImportStatusSucceeded, true).
			Update("source_delete_pending", false).Error
	})
}

func (ctx *MahresourcesContext) PruneRunOutputs(before time.Time) (int64, error) {
	var removed int64
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		terminal := terminalCommandStatuses()
		subquery := tx.Model(&models.PluginCommandRun{}).Select("id").Where("status IN ?", terminal)
		res := tx.Where("created_at < ? AND run_id IN (?)", before, subquery).Delete(&models.PluginCommandRunOutput{})
		removed = res.RowsAffected
		return res.Error
	})
	return removed, err
}

func (ctx *MahresourcesContext) ImportMap(runID, name string) (plugin_commands.ImportMapEntry, bool, error) {
	var row models.PluginCommandImportMap
	err := ctx.db.Where("run_id = ? AND file_name = ?", runID, name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return plugin_commands.ImportMapEntry{}, false, nil
	}
	if err != nil {
		return plugin_commands.ImportMapEntry{}, false, err
	}
	return importMapEntry(row), true, nil
}

func commandClaimResult(row models.PluginCommandImportMap, created, enqueue bool) plugin_commands.ImportClaimResult {
	return plugin_commands.ImportClaimResult{
		ImportID: row.ImportID, ResourceID: copyCommandUint(row.ResourceID), Status: row.Status,
		Created: created, Enqueue: enqueue,
	}
}

func (ctx *MahresourcesContext) lockCommandImportMap(tx *gorm.DB, runID, name string) (models.PluginCommandImportMap, error) {
	var row models.PluginCommandImportMap
	query := tx.Where("run_id = ? AND file_name = ?", runID, name)
	if ctx.Config != nil && ctx.Config.DbType == constants.DbTypePosgres {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return row, query.First(&row).Error
}

func (ctx *MahresourcesContext) ClaimImport(req plugin_commands.ImportClaimRequest) (plugin_commands.ImportClaimResult, error) {
	if req.ImportID == "" || req.RunID == "" || req.FileName == "" {
		return plugin_commands.ImportClaimResult{}, fmt.Errorf("plugin command import claim requires id, run and file name")
	}
	if req.CreatedByUserID == nil || *req.CreatedByUserID == 0 {
		return plugin_commands.ImportClaimResult{}, fmt.Errorf("plugin command import claim requires an acting user")
	}
	// An omitted field selection means the empty ResourceFields value. Persist
	// its explicit JSON form so the canonical import replay codec can validate
	// the same input that the importer executes.
	if req.FieldsJSON == "" {
		req.FieldsJSON = "{}"
	}
	// The dispatcher is process-lifetime and intentionally unscoped, but the
	// global create callback must stamp the current claim actor rather than the
	// singleton/default actor. Scope and role are revalidated separately when
	// the worker starts.
	claimContext := context.WithValue(ctx.db.Statement.Context, actingUserCtxKey{}, *req.CreatedByUserID)
	claimDB := ctx.db.WithContext(claimContext)
	const attempts = 20
	for attempt := 0; attempt < attempts; attempt++ {
		var result plugin_commands.ImportClaimResult
		err := claimDB.Transaction(func(tx *gorm.DB) error {
			if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
				return err
			}
			var run models.PluginCommandRun
			if err := tx.Where("id = ?", req.RunID).First(&run).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return plugin_commands.ErrRunNotFound
				}
				return err
			}
			if !(plugin_commands.Access{PluginName: run.PluginName, ActorUserID: req.CreatedByUserID}).AllowsRun(runRecord(run)) {
				return fmt.Errorf("plugin command run is not accessible to this actor")
			}

			mapped, err := ctx.lockCommandImportMap(tx, req.RunID, req.FileName)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				claim := models.PluginCommandImport{
					ID: req.ImportID, RunID: req.RunID, FileName: req.FileName,
					FieldsJSON:       req.FieldsJSON,
					PluginGeneration: req.PluginGeneration, CreatedByUserId: copyCommandUint(req.CreatedByUserID),
					Status: plugin_commands.ImportStatusPending, CreatedAt: req.CreatedAt,
				}
				jobID, err := ctx.acceptAndPersistPluginCommandImportTx(tx, run, claim, "")
				if err != nil {
					return err
				}
				mapped = models.PluginCommandImportMap{RunID: req.RunID, FileName: req.FileName, ImportID: req.ImportID, Status: plugin_commands.ImportStatusPending}
				if err := tx.Create(&mapped).Error; err != nil {
					return err
				}
				result = commandClaimResult(mapped, true, true)
				result.JobID = jobID
				return nil
			}
			if err != nil {
				return err
			}

			switch mapped.Status {
			case plugin_commands.ImportStatusPending, plugin_commands.ImportStatusRunning, plugin_commands.ImportStatusSucceeded:
				result = commandClaimResult(mapped, false, false)
				return nil
			case plugin_commands.ImportStatusInterrupted:
				var predecessor models.PluginCommandImport
				if err := tx.Where("id = ?", mapped.ImportID).First(&predecessor).Error; err != nil {
					return err
				}
				claim := models.PluginCommandImport{ID: req.ImportID, RunID: req.RunID, FileName: req.FileName,
					FieldsJSON:       req.FieldsJSON,
					PluginGeneration: req.PluginGeneration, CreatedByUserId: copyCommandUint(req.CreatedByUserID),
					Status: plugin_commands.ImportStatusPending, CreatedAt: req.CreatedAt}
				jobID, err := ctx.acceptAndPersistPluginCommandImportTx(tx, run, claim, predecessor.JobID)
				if err != nil {
					return err
				}
				if err := tx.Model(&models.PluginCommandImportMap{}).Where("id = ?", mapped.ID).
					Updates(map[string]any{"import_id": req.ImportID, "status": plugin_commands.ImportStatusPending, "resource_id": nil, "error": "", "source_delete_pending": false}).Error; err != nil {
					return err
				}
				mapped.ImportID, mapped.Status, mapped.ResourceID, mapped.Error, mapped.SourceDeletePending = req.ImportID, plugin_commands.ImportStatusPending, nil, "", false
				result = commandClaimResult(mapped, true, true)
				result.JobID = jobID
				return nil
			case plugin_commands.ImportStatusFailed, plugin_commands.ImportStatusCancelled:
				var predecessor models.PluginCommandImport
				if err := tx.Where("id = ?", mapped.ImportID).First(&predecessor).Error; err != nil {
					return err
				}
				claim := models.PluginCommandImport{
					ID: req.ImportID, RunID: req.RunID, FileName: req.FileName,
					FieldsJSON:       req.FieldsJSON,
					PluginGeneration: req.PluginGeneration, CreatedByUserId: copyCommandUint(req.CreatedByUserID),
					Status: plugin_commands.ImportStatusPending, CreatedAt: req.CreatedAt,
				}
				jobID, err := ctx.acceptAndPersistPluginCommandImportTx(tx, run, claim, predecessor.JobID)
				if err != nil {
					return err
				}
				if err := tx.Model(&models.PluginCommandImportMap{}).Where("id = ?", mapped.ID).
					Updates(map[string]any{"import_id": req.ImportID, "status": plugin_commands.ImportStatusPending, "resource_id": nil, "error": "", "source_delete_pending": false}).Error; err != nil {
					return err
				}
				mapped.ImportID, mapped.Status, mapped.ResourceID, mapped.Error, mapped.SourceDeletePending = req.ImportID, plugin_commands.ImportStatusPending, nil, "", false
				result = commandClaimResult(mapped, true, true)
				result.JobID = jobID
				return nil
			default:
				return fmt.Errorf("invalid plugin command import map status %q", mapped.Status)
			}
		})
		if err == nil {
			return result, nil
		}
		if errors.Is(err, errPluginCommandTransitionLost) || isUniqueConstraintError(err) || isLockContentionError(err) || isDeadlockError(err) {
			time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
			continue
		}
		return plugin_commands.ImportClaimResult{}, err
	}
	return plugin_commands.ImportClaimResult{}, fmt.Errorf("claiming plugin command import: contention did not settle")
}

func (ctx *MahresourcesContext) MarkImportRunning(importID string, started time.Time) (bool, error) {
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		updates := map[string]any{"status": plugin_commands.ImportStatusRunning, "started_at": started}
		if ctx.JobService() != nil {
			updates["job_execution_token"] = tx.Model(&models.Job{}).Select("execution_token").Where("jobs.id = plugin_command_imports.job_id")
		}
		res := tx.Model(&models.PluginCommandImport{}).
			Where("id = ? AND status = ? AND created_by_user_id IS NOT NULL", importID, plugin_commands.ImportStatusPending).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errPluginCommandTransitionLost
		}
		mapped := tx.Model(&models.PluginCommandImportMap{}).Where("import_id = ?", importID).
			Update("status", plugin_commands.ImportStatusRunning)
		if mapped.Error != nil {
			return mapped.Error
		}
		if mapped.RowsAffected != 1 {
			return fmt.Errorf("plugin command import %q has no active map entry", importID)
		}
		return nil
	})
	if errors.Is(err, errPluginCommandTransitionLost) {
		return false, nil
	}
	return err == nil, err
}

// CancelPendingImport is plugin disable's half of the pending/running race.
// Its status predicate must remain narrower than FinishImport's: a worker whose
// MarkImportRunning committed first owns the import and is allowed to finish.
func (ctx *MahresourcesContext) CancelPendingImport(importID, reason string, finished time.Time) (bool, error) {
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		var prior models.PluginCommandImport
		if err := tx.Where("id = ?", importID).First(&prior).Error; err != nil {
			return err
		}
		claim := tx.Model(&models.PluginCommandImport{}).
			Where("id = ? AND status = ?", importID, plugin_commands.ImportStatusPending).
			Updates(map[string]any{
				"status": plugin_commands.ImportStatusCancelled, "error": reason, "finished_at": finished,
			})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return errPluginCommandTransitionLost
		}
		mapped := tx.Model(&models.PluginCommandImportMap{}).
			Where("import_id = ? AND status = ?", importID, plugin_commands.ImportStatusPending).
			Updates(map[string]any{
				"status": plugin_commands.ImportStatusCancelled, "error": reason,
			})
		if mapped.Error != nil {
			return mapped.Error
		}
		if mapped.RowsAffected != 1 {
			return fmt.Errorf("plugin command import %q has no pending map entry", importID)
		}
		if prior.JobID != "" && ctx.JobService() != nil {
			if err := ctx.finishPluginCommandImportJobTx(tx, prior.JobID, prior.JobExecutionToken, plugin_commands.ImportStatusCancelled, prior.ID, nil); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errPluginCommandTransitionLost) {
		return false, nil
	}
	return err == nil, err
}

func (ctx *MahresourcesContext) FinishImport(importID string, finish plugin_commands.ImportFinish) (bool, error) {
	if !plugin_commands.ImportStatusTerminal(finish.Status) {
		return false, fmt.Errorf("invalid terminal plugin command import status %q", finish.Status)
	}
	if finish.Status == plugin_commands.ImportStatusSucceeded {
		if finish.ResourceID == nil || *finish.ResourceID == 0 {
			return false, fmt.Errorf("a successful plugin command import requires a resource id")
		}
	} else if finish.ResourceID != nil {
		return false, fmt.Errorf("plugin command import status %q cannot carry a resource id", finish.Status)
	}
	priorStatuses := []string{plugin_commands.ImportStatusRunning}
	if finish.Status != plugin_commands.ImportStatusSucceeded {
		priorStatuses = append(priorStatuses, plugin_commands.ImportStatusPending)
	}
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		var prior models.PluginCommandImport
		if err := tx.Where("id = ?", importID).First(&prior).Error; err != nil {
			return err
		}
		res := tx.Model(&models.PluginCommandImport{}).
			Where("id = ? AND status IN ?", importID, priorStatuses).
			Updates(map[string]any{
				"status": finish.Status, "error": finish.Error, "finished_at": finish.FinishedAt,
				"source_delete_pending": finish.SourceDeletePending,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errPluginCommandTransitionLost
		}
		mapped := tx.Model(&models.PluginCommandImportMap{}).Where("import_id = ?", importID).
			Updates(map[string]any{
				"status": finish.Status, "error": finish.Error, "resource_id": finish.ResourceID,
				"source_delete_pending": finish.SourceDeletePending,
			})
		if mapped.Error != nil {
			return mapped.Error
		}
		if mapped.RowsAffected != 1 {
			return fmt.Errorf("plugin command import %q has no active map entry", importID)
		}
		if prior.JobID != "" && ctx.JobService() != nil {
			if err := ctx.finishPluginCommandImportJobTx(tx, prior.JobID, prior.JobExecutionToken, finish.Status, prior.ID, finish.ResourceID); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errPluginCommandTransitionLost) {
		return false, nil
	}
	return err == nil, err
}

func (ctx *MahresourcesContext) SetImportSourceDeletePending(importID string, pending bool) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		claim := tx.Model(&models.PluginCommandImport{}).
			Where("id = ? AND status = ?", importID, plugin_commands.ImportStatusSucceeded).
			Update("source_delete_pending", pending)
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return fmt.Errorf("plugin command import %q is not a succeeded claim", importID)
		}
		mapped := tx.Model(&models.PluginCommandImportMap{}).
			Where("import_id = ? AND status = ?", importID, plugin_commands.ImportStatusSucceeded).
			Update("source_delete_pending", pending)
		if mapped.Error != nil {
			return mapped.Error
		}
		if mapped.RowsAffected != 1 {
			return fmt.Errorf("plugin command import %q has no succeeded map entry", importID)
		}
		return nil
	})
}

func (ctx *MahresourcesContext) InterruptNonterminalImports(finished time.Time) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		nonterminal := []string{plugin_commands.ImportStatusPending, plugin_commands.ImportStatusRunning}
		var rows []models.PluginCommandImport
		query := tx.Where("status IN ?", nonterminal)
		if ctx.Config != nil && ctx.Config.DbType == constants.DbTypePosgres {
			// FinishImport updates the claim before its map row. Locking the claims
			// keeps recovery from observing the old status and later overwriting the
			// map entry committed by a concurrent successful finish.
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		if ctx.JobService() != nil {
			for _, row := range rows {
				if row.JobID != "" {
					if err := ctx.finishPluginCommandImportJobTx(tx, row.JobID, row.JobExecutionToken, plugin_commands.ImportStatusInterrupted, row.ID, nil); err != nil {
						return err
					}
				}
			}
		}
		ids := make([]string, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		claims := tx.Model(&models.PluginCommandImport{}).Where("id IN ? AND status IN ?", ids, nonterminal).
			Updates(map[string]any{"status": plugin_commands.ImportStatusInterrupted, "error": "server interrupted", "finished_at": finished})
		if claims.Error != nil {
			return claims.Error
		}
		if claims.RowsAffected != int64(len(ids)) {
			return errPluginCommandTransitionLost
		}
		maps := tx.Model(&models.PluginCommandImportMap{}).Where("import_id IN ? AND status IN ?", ids, nonterminal).
			Updates(map[string]any{"status": plugin_commands.ImportStatusInterrupted, "error": "server interrupted"})
		if maps.Error != nil {
			return maps.Error
		}
		if maps.RowsAffected != int64(len(ids)) {
			return fmt.Errorf("plugin command import recovery found %d claims but transitioned %d map entries", len(ids), maps.RowsAffected)
		}
		return nil
	})
}

func (ctx *MahresourcesContext) NonterminalImports() ([]plugin_commands.ImportRecord, error) {
	var rows []models.PluginCommandImport
	if err := ctx.db.Where("status IN ?", []string{plugin_commands.ImportStatusPending, plugin_commands.ImportStatusRunning}).
		Order("created_at asc, id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]plugin_commands.ImportRecord, len(rows))
	for i, row := range rows {
		if err := ctx.hydratePluginCommandImport(&row, ctx.db); err != nil {
			return nil, err
		}
		result[i] = importRecord(row)
	}
	return result, nil
}

func (ctx *MahresourcesContext) HasNonterminalImports(runID string) (bool, error) {
	var count int64
	err := ctx.db.Model(&models.PluginCommandImport{}).
		Where("run_id = ? AND status IN ?", runID, []string{plugin_commands.ImportStatusPending, plugin_commands.ImportStatusRunning}).
		Count(&count).Error
	return count > 0, err
}
