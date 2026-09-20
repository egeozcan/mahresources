package application_context

import (
	"context"
	"errors"
	"fmt"
	"time"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/plugin_commands"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errPluginCommandTransitionLost = errors.New("plugin command transition lost")

var _ plugin_commands.Store = (*MahresourcesContext)(nil)
var _ plugin_commands.PendingImportCanceller = (*MahresourcesContext)(nil)

func copyCommandUint(value *uint) *uint {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func runModel(record plugin_commands.RunRecord) models.PluginCommandRun {
	return models.PluginCommandRun{
		ID: record.ID, PluginName: record.PluginName, CommandName: record.CommandName,
		ParamsJSON: record.ParamsJSON, Status: record.Status, ExitCode: record.ExitCode,
		Error: record.Error, ProcessGroupID: record.ProcessGroupID,
		CancelRequested: record.CancelRequested, OutputUnverified: record.OutputUnverified,
		ActorlessAtSubmission: record.ActorlessAtSubmission,
		CreatedByUserId:       copyCommandUint(record.CreatedByUserID), CreatedAt: record.CreatedAt,
		StartedAt: record.StartedAt, FinishedAt: record.FinishedAt, ExchangeRemovedAt: record.ExchangeRemovedAt,
	}
}

func runRecord(row models.PluginCommandRun) plugin_commands.RunRecord {
	return plugin_commands.RunRecord{
		ID: row.ID, PluginName: row.PluginName, CommandName: row.CommandName,
		ParamsJSON: row.ParamsJSON, Status: row.Status, ExitCode: row.ExitCode,
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

func outputRecord(row models.PluginCommandRunOutput) plugin_commands.RunOutput {
	return plugin_commands.RunOutput{RunID: row.RunID, ArgvJSON: row.ArgvJSON, OutputTail: row.OutputTail, CreatedAt: row.CreatedAt}
}

func importRecord(row models.PluginCommandImport) plugin_commands.ImportRecord {
	return plugin_commands.ImportRecord{
		ID: row.ID, RunID: row.RunID, FileName: row.FileName,
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
		row := runModel(record)
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
		return tx.Create(&out).Error
	})
}

func (ctx *MahresourcesContext) MarkRunRunning(id string, started time.Time) (bool, error) {
	res := ctx.db.Model(&models.PluginCommandRun{}).
		Where("id = ? AND status = ?", id, plugin_commands.RunStatusQueued).
		Updates(map[string]any{"status": plugin_commands.RunStatusRunning, "started_at": started})
	return res.RowsAffected == 1, res.Error
}

func (ctx *MahresourcesContext) SetRunProcessGroup(id string, pgid int, bootSessionID string) error {
	if pgid <= 0 {
		return fmt.Errorf("plugin command process group must be positive")
	}
	res := ctx.db.Model(&models.PluginCommandRun{}).
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
}

func (ctx *MahresourcesContext) RequestRunCancel(id, reason string) error {
	res := ctx.db.Model(&models.PluginCommandRun{}).
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
	if err := ctx.db.Model(&models.PluginCommandRun{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return plugin_commands.ErrRunNotFound
	}
	return plugin_commands.ErrRunNotCancellable
}

func (ctx *MahresourcesContext) FinishRun(id string, finish plugin_commands.RunFinish) (bool, error) {
	if !plugin_commands.RunStatusTerminal(finish.Status) {
		return false, fmt.Errorf("invalid terminal plugin command status %q", finish.Status)
	}
	priorStatuses := []string{plugin_commands.RunStatusRunning}
	if finish.Status == plugin_commands.RunStatusCancelled || finish.Status == plugin_commands.RunStatusInterrupted {
		priorStatuses = append(priorStatuses, plugin_commands.RunStatusQueued)
	}
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
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
		return nil
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
	var rows []models.PluginCommandRun
	if err := ctx.db.Where("status IN ?", []string{plugin_commands.RunStatusQueued, plugin_commands.RunStatusRunning}).
		Order("created_at asc, id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]plugin_commands.RecoveryRun, len(rows))
	for i, row := range rows {
		result[i] = plugin_commands.RecoveryRun{RunRecord: runRecord(row), BootSessionID: row.BootSessionID}
	}
	return result, nil
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
	terminal := terminalCommandStatuses()
	subquery := ctx.db.Model(&models.PluginCommandRun{}).Select("id").Where("status IN ?", terminal)
	res := ctx.db.Where("created_at < ? AND run_id IN (?)", before, subquery).Delete(&models.PluginCommandRunOutput{})
	return res.RowsAffected, res.Error
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
					PluginGeneration: req.PluginGeneration, CreatedByUserId: copyCommandUint(req.CreatedByUserID),
					Status: plugin_commands.ImportStatusPending, CreatedAt: req.CreatedAt,
				}
				if err := tx.Create(&claim).Error; err != nil {
					return err
				}
				mapped = models.PluginCommandImportMap{RunID: req.RunID, FileName: req.FileName, ImportID: req.ImportID, Status: plugin_commands.ImportStatusPending}
				if err := tx.Create(&mapped).Error; err != nil {
					return err
				}
				result = commandClaimResult(mapped, true, true)
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
				updates := map[string]any{
					"status": plugin_commands.ImportStatusPending, "plugin_generation": req.PluginGeneration,
					"created_by_user_id": *req.CreatedByUserID, "error": "", "started_at": nil,
					"finished_at": nil,
				}
				res := tx.Model(&models.PluginCommandImport{}).
					Where("id = ? AND status = ?", mapped.ImportID, plugin_commands.ImportStatusInterrupted).Updates(updates)
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected != 1 {
					return errPluginCommandTransitionLost
				}
				if err := tx.Model(&models.PluginCommandImportMap{}).Where("id = ?", mapped.ID).
					Updates(map[string]any{"status": plugin_commands.ImportStatusPending, "resource_id": nil, "error": "", "source_delete_pending": false}).Error; err != nil {
					return err
				}
				mapped.Status, mapped.ResourceID, mapped.Error, mapped.SourceDeletePending = plugin_commands.ImportStatusPending, nil, "", false
				result = commandClaimResult(mapped, false, true)
				return nil
			case plugin_commands.ImportStatusFailed, plugin_commands.ImportStatusCancelled:
				claim := models.PluginCommandImport{
					ID: req.ImportID, RunID: req.RunID, FileName: req.FileName,
					PluginGeneration: req.PluginGeneration, CreatedByUserId: copyCommandUint(req.CreatedByUserID),
					Status: plugin_commands.ImportStatusPending, CreatedAt: req.CreatedAt,
				}
				if err := tx.Create(&claim).Error; err != nil {
					return err
				}
				if err := tx.Model(&models.PluginCommandImportMap{}).Where("id = ?", mapped.ID).
					Updates(map[string]any{"import_id": req.ImportID, "status": plugin_commands.ImportStatusPending, "resource_id": nil, "error": "", "source_delete_pending": false}).Error; err != nil {
					return err
				}
				mapped.ImportID, mapped.Status, mapped.ResourceID, mapped.Error, mapped.SourceDeletePending = req.ImportID, plugin_commands.ImportStatusPending, nil, "", false
				result = commandClaimResult(mapped, true, true)
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
		res := tx.Model(&models.PluginCommandImport{}).
			Where("id = ? AND status = ? AND created_by_user_id IS NOT NULL", importID, plugin_commands.ImportStatusPending).
			Updates(map[string]any{"status": plugin_commands.ImportStatusRunning, "started_at": started})
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
		return nil
	})
	if errors.Is(err, errPluginCommandTransitionLost) {
		return false, nil
	}
	return err == nil, err
}

func (ctx *MahresourcesContext) SetImportSourceDeletePending(importID string, pending bool) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
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
