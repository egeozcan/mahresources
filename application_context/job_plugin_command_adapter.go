package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_commands"
)

const (
	JobKindPluginCommand               = "plugin-command"
	JobKindPluginCommandImport         = "plugin-command-import"
	jobPluginCommandVersion            = 1
	pluginCommandHandleNamespace       = "plugin-command-run"
	pluginCommandImportHandleNamespace = "plugin-command-import"
)

type pluginCommandJobAdapter struct {
	ctx  *MahresourcesContext
	kind string
}

func (a *pluginCommandJobAdapter) Definition() jobs.Definition {
	return jobs.Definition{Kind: a.kind, KindVersion: jobPluginCommandVersion, Restorable: false,
		Visibility: jobs.VisibilityAdmin, CapacityGroup: "plugin-command", MaxConcurrent: 6, Lease: 2 * time.Minute}
}

// RuntimeClaimEnabled is false because the fenced plugin dispatcher directly
// claims its canonical Job when it admits work to the managed live lane.
func (*pluginCommandJobAdapter) RuntimeClaimEnabled() bool { return false }
func (a *pluginCommandJobAdapter) Dispatch(context.Context, jobs.Execution) error {
	return fmt.Errorf("plugin command jobs are dispatched by the fenced command runtime")
}
func (*pluginCommandJobAdapter) Reconcile(context.Context, jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
	return jobs.ReconcileExternalWorkUnproven, nil
}
func (*pluginCommandJobAdapter) CleanupArtifacts(context.Context, jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
	return jobs.ArtifactCleanupResult{}, nil
}
func (a *pluginCommandJobAdapter) Commands(_ context.Context, command jobs.CommandContext) ([]jobs.Command, error) {
	commands := []jobs.Command{{Key: jobs.CommandCancel, Label: "Cancel", Destructive: true, Confirmation: "Cancel this plugin command?"},
		{Key: "inspect", Label: "Inspect command history"}}
	if a.kind == JobKindPluginCommandImport {
		if a.importRetryable(command.Deps.DB, command.Snapshot.ID) && a.importExchangeFilePresent(command.Deps.DB, command.Snapshot.ID) {
			commands = append(commands, jobs.Command{Key: "retry-import", Label: "Retry import", Destructive: true,
				Confirmation: "Retry this import from its admitted exchange file?"})
		}
	}
	return commands, nil
}
func (a *pluginCommandJobAdapter) ExecuteCommand(_ context.Context, execution jobs.CommandExecution) (jobs.CommandOutcome, error) {
	switch execution.Key {
	case jobs.CommandCancel:
		if a.kind == JobKindPluginCommand {
			var source models.PluginCommandRun
			if err := a.ctx.db.Where("job_id = ?", execution.JobID).First(&source).Error; err != nil {
				return jobs.CommandOutcome{}, fmt.Errorf("plugin command source is unavailable")
			}
			if err := a.ctx.CancelPluginCommandRun(source.ID); err != nil {
				return jobs.CommandOutcome{}, err
			}
		} else {
			var source models.PluginCommandImport
			if err := a.ctx.db.Where("job_id = ?", execution.JobID).First(&source).Error; err != nil {
				return jobs.CommandOutcome{}, fmt.Errorf("plugin command import source is unavailable")
			}
			active, err := a.ctx.pluginCommandActive()
			if err != nil {
				return jobs.CommandOutcome{}, err
			}
			if err := active.dispatcher.CancelImport(source.ID, "operator cancelled"); err != nil {
				return jobs.CommandOutcome{}, err
			}
		}
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "cancellation requested"}, nil
	case "inspect":
		var detail map[string]string
		if a.kind == JobKindPluginCommand {
			var source models.PluginCommandRun
			if err := a.ctx.db.Where("job_id = ?", execution.JobID).First(&source).Error; err != nil {
				return jobs.CommandOutcome{}, fmt.Errorf("plugin command source is unavailable")
			}
			detail = map[string]string{"runId": source.ID, "status": source.Status}
		} else {
			var source models.PluginCommandImport
			if err := a.ctx.db.Where("job_id = ?", execution.JobID).First(&source).Error; err != nil {
				return jobs.CommandOutcome{}, fmt.Errorf("plugin command import source is unavailable")
			}
			detail = map[string]string{"importId": source.ID, "runId": source.RunID, "status": source.Status}
		}
		encoded, _ := json.Marshal(detail)
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "command history reference resolved", Detail: encoded}, nil
	case "retry-import":
		return a.retryImport(execution)
	default:
		return jobs.CommandOutcome{}, fmt.Errorf("plugin command Job control is unavailable")
	}
}

func (a *pluginCommandJobAdapter) ApplyHostTransition(_ context.Context, deps jobs.Deps, snapshot jobs.Snapshot, key string, to jobs.State) error {
	if key != jobs.CommandCancel || to != jobs.StateCancelled {
		return nil
	}
	if !a.ctx.pluginCommandFenceOwned() {
		return fmt.Errorf("plugin command runtime fence is not owned")
	}
	if err := a.ctx.requirePluginCommandFenceTx(deps.DB); err != nil {
		return err
	}
	finished := time.Now().UTC()
	if a.kind == JobKindPluginCommand {
		var source models.PluginCommandRun
		if err := deps.DB.Where("job_id = ?", snapshot.ID).First(&source).Error; err != nil {
			return err
		}
		if source.Status != plugin_commands.RunStatusQueued {
			return fmt.Errorf("plugin command source is not queued")
		}
		if err := deps.DB.Model(&models.PluginCommandRun{}).Where("id = ? AND status = ?", source.ID, plugin_commands.RunStatusQueued).
			Updates(map[string]any{"status": plugin_commands.RunStatusCancelled, "cancel_requested": true, "error": "operator cancelled", "finished_at": finished}).Error; err != nil {
			return err
		}
		return nil
	}
	var source models.PluginCommandImport
	if err := deps.DB.Where("job_id = ?", snapshot.ID).First(&source).Error; err != nil {
		return err
	}
	if source.Status != plugin_commands.ImportStatusPending {
		return fmt.Errorf("plugin command import source is not pending")
	}
	if err := deps.DB.Model(&models.PluginCommandImport{}).Where("id = ? AND status = ?", source.ID, plugin_commands.ImportStatusPending).
		Updates(map[string]any{"status": plugin_commands.ImportStatusCancelled, "error": "operator cancelled", "finished_at": finished}).Error; err != nil {
		return err
	}
	if err := deps.DB.Model(&models.PluginCommandImportMap{}).Where("import_id = ? AND status = ?", source.ID, plugin_commands.ImportStatusPending).
		Updates(map[string]any{"status": plugin_commands.ImportStatusCancelled, "error": "operator cancelled"}).Error; err != nil {
		return err
	}
	return nil
}

func (a *pluginCommandJobAdapter) AfterHostTransition(_ context.Context, snapshot jobs.Snapshot, key string, to jobs.State) {
	if key != jobs.CommandCancel || to != jobs.StateCancelled {
		return
	}
	active, err := a.ctx.pluginCommandActive()
	if err != nil {
		a.ctx.Logger().Warning(models.LogActionSystem, "plugin_command", nil, snapshot.ID, "queued cancellation committed but dispatcher notification was unavailable", nil)
		return
	}
	if a.kind == JobKindPluginCommand {
		var source models.PluginCommandRun
		if err := a.ctx.db.Where("job_id = ?", snapshot.ID).First(&source).Error; err != nil {
			return
		}
		if err := active.dispatcher.Cancel(source.ID, "operator cancelled"); err != nil {
			a.ctx.Logger().Warning(models.LogActionSystem, "plugin_command", nil, source.ID, "queued cancellation dispatcher notification failed", nil)
		}
		return
	}
	var source models.PluginCommandImport
	if err := a.ctx.db.Where("job_id = ?", snapshot.ID).First(&source).Error; err != nil {
		return
	}
	if err := active.dispatcher.CancelImport(source.ID, "operator cancelled"); err != nil {
		a.ctx.Logger().Warning(models.LogActionSystem, "plugin_command", nil, source.ID, "queued import cancellation dispatcher notification failed", nil)
	}
}

func (a *pluginCommandJobAdapter) importRetryable(db *gorm.DB, jobID string) bool {
	if db == nil || !a.ctx.pluginCommandFenceOwned() {
		return false
	}
	var fenceCount int64
	a.ctx.pluginCommandController.mu.Lock()
	token := a.ctx.pluginCommandController.dbFence
	a.ctx.pluginCommandController.mu.Unlock()
	if db.Model(&models.JobRuntimeFence{}).Where("key = ? AND token = ?", pluginCommandRuntimeFenceKey, token).Count(&fenceCount).Error != nil || fenceCount != 1 {
		return false
	}
	var source models.PluginCommandImport
	if db.Where("job_id = ?", jobID).First(&source).Error != nil || source.FieldsJSON == "" || source.CreatedByUserId == nil {
		return false
	}
	if source.Status != plugin_commands.ImportStatusFailed && source.Status != plugin_commands.ImportStatusCancelled && source.Status != plugin_commands.ImportStatusInterrupted {
		return false
	}
	var mapped models.PluginCommandImportMap
	if db.Where("run_id = ? AND file_name = ?", source.RunID, source.FileName).First(&mapped).Error != nil || mapped.ImportID != source.ID || mapped.Status != source.Status || mapped.ResourceID != nil {
		return false
	}
	var run models.PluginCommandRun
	if db.Where("id = ? AND status = ? AND output_unverified = ?", source.RunID, plugin_commands.RunStatusSucceeded, false).First(&run).Error != nil {
		return false
	}
	var fields plugin_commands.ResourceFields
	if json.Unmarshal([]byte(source.FieldsJSON), &fields) != nil {
		return false
	}
	return true
}

func (a *pluginCommandJobAdapter) importExchangeFilePresent(db *gorm.DB, jobID string) bool {
	if db == nil {
		return false
	}
	var source models.PluginCommandImport
	if db.Where("job_id = ?", jobID).First(&source).Error != nil {
		return false
	}
	active, err := a.ctx.pluginCommandActive()
	if err != nil || active.exchange == nil {
		return false
	}
	listing, err := active.exchange.List(plugin_commands.Access{Administrator: true}, source.RunID)
	if err != nil {
		return false
	}
	for _, entry := range listing.Entries {
		if entry.Name == source.FileName {
			return true
		}
	}
	return false
}

func (a *pluginCommandJobAdapter) retryImport(execution jobs.CommandExecution) (jobs.CommandOutcome, error) {
	if !a.importRetryable(a.ctx.db, execution.JobID) {
		return jobs.CommandOutcome{}, fmt.Errorf("plugin command import is no longer safely retryable")
	}
	var source models.PluginCommandImport
	if err := a.ctx.db.Where("job_id = ?", execution.JobID).First(&source).Error; err != nil {
		return jobs.CommandOutcome{}, fmt.Errorf("plugin command import source is unavailable")
	}
	var fields plugin_commands.ResourceFields
	if err := json.Unmarshal([]byte(source.FieldsJSON), &fields); err != nil {
		return jobs.CommandOutcome{}, fmt.Errorf("plugin command import fields are invalid")
	}
	plugins := a.ctx.PluginManager()
	if plugins == nil {
		return jobs.CommandOutcome{}, fmt.Errorf("plugin command runtime is unavailable")
	}
	var generation uint64
	var run models.PluginCommandRun
	if err := a.ctx.db.Where("id = ?", source.RunID).First(&run).Error; err != nil {
		return jobs.CommandOutcome{}, fmt.Errorf("plugin command run is unavailable")
	}
	pluginName := run.PluginName
	for _, plugin := range plugins.Plugins() {
		if plugin.Name == pluginName {
			generation = plugin.Generation
			break
		}
	}
	if generation == 0 {
		return jobs.CommandOutcome{}, fmt.Errorf("plugin command generation is unavailable")
	}
	active, err := a.ctx.pluginCommandActive()
	if err != nil {
		return jobs.CommandOutcome{}, err
	}
	listing, err := active.exchange.List(plugin_commands.Access{Administrator: true}, source.RunID)
	if err != nil {
		return jobs.CommandOutcome{}, fmt.Errorf("the admitted exchange file is unavailable")
	}
	filePresent := false
	for _, entry := range listing.Entries {
		filePresent = filePresent || entry.Name == source.FileName
	}
	if !filePresent {
		return jobs.CommandOutcome{}, fmt.Errorf("the admitted exchange file is unavailable")
	}
	result, err := active.dispatcher.SubmitImport(plugin_commands.ImportSubmission{
		Access: plugin_commands.Access{PluginName: pluginName, ActorUserID: copyCommandUint(source.CreatedByUserId), Administrator: true},
		RunID:  source.RunID, Name: source.FileName, Fields: fields, PluginGeneration: generation,
		ActorUserID: copyCommandUint(source.CreatedByUserId),
	})
	if err != nil {
		return jobs.CommandOutcome{}, err
	}
	detail, _ := json.Marshal(map[string]string{"importId": result.ImportID})
	return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "a new import job was admitted", Detail: detail}, nil
}

func (ctx *MahresourcesContext) registerPluginCommandJobKinds(service *jobs.Service) error {
	for _, kind := range []string{JobKindPluginCommand, JobKindPluginCommandImport} {
		if _, ok := service.AdapterFor(kind, jobPluginCommandVersion); ok {
			continue
		}
		if err := service.RegisterAdapter(&pluginCommandJobAdapter{ctx: ctx, kind: kind}); err != nil {
			return err
		}
	}
	return nil
}

func (ctx *MahresourcesContext) claimPluginCommandJob(jobID, kind, sourceID string) (jobs.Execution, bool, error) {
	if jobID == "" || ctx == nil || ctx.JobService() == nil {
		return jobs.Execution{}, false, nil
	}
	if sourceID == "" {
		return jobs.Execution{}, false, fmt.Errorf("plugin command Job %s has no durable source id", jobID)
	}
	if !ctx.pluginCommandFenceOwned() {
		return jobs.Execution{}, false, fmt.Errorf("plugin command runtime fence is not owned")
	}
	controller := ctx.pluginCommandController
	controller.mu.Lock()
	token := controller.dbFence
	controller.mu.Unlock()
	var execution jobs.Execution
	var claimed bool
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		deps := ctx.jobDeps()
		deps.DB = tx
		var err error
		execution, claimed, err = ctx.JobService().Claim(context.Background(), deps, jobs.ClaimRequest{
			Kind: kind, KindVersion: jobPluginCommandVersion, JobID: jobID,
			Claimant: "plugin-command:" + token, Capacity: ctx.hostClaimCapacityBudget(),
		})
		if err != nil || !claimed {
			return err
		}
		model := any(&models.PluginCommandRun{})
		status := plugin_commands.RunStatusQueued
		if kind == JobKindPluginCommandImport {
			model = &models.PluginCommandImport{}
			status = plugin_commands.ImportStatusPending
		}
		result := tx.Model(model).Where("id = ? AND job_id = ? AND status = ?", sourceID, jobID, status).
			Update("job_execution_token", execution.ExecutionToken)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("plugin command source %s was not queued for Job %s", sourceID, jobID)
		}
		return nil
	})
	return execution, claimed, err
}

func (ctx *MahresourcesContext) releasePluginCommandJob(execution jobs.Execution) error {
	if execution.JobID == "" || ctx == nil || ctx.JobService() == nil {
		return nil
	}
	_, err := ctx.JobService().ReleaseClaim(ctx.jobDeps(), jobs.ReleaseRequest{
		ExecutionRef: jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		Reason:       jobs.ReleaseReasonExecutionEnded, To: jobs.StateQueued,
	})
	return err
}

func (ctx *MahresourcesContext) heartbeatPluginCommandJob(execution jobs.Execution) error {
	if ctx == nil || execution.JobID == "" || execution.ExecutionToken == "" {
		return fmt.Errorf("plugin command execution identity is incomplete")
	}
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.requirePluginCommandFenceTx(tx); err != nil {
			return err
		}
		deps := ctx.jobDeps()
		deps.DB = tx
		return ctx.JobService().Heartbeat(deps, jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken}, 2*time.Minute)
	})
}

func (ctx *MahresourcesContext) acceptPluginCommandRunJob(tx *gorm.DB, record plugin_commands.RunRecord) (string, error) {
	service := ctx.JobService()
	if service == nil {
		if ctx.pluginCommandController != nil {
			ctx.pluginCommandController.mu.Lock()
			// A controller with the real staging lease is the production path and
			// must never commit source-only acceptance. Lease-less active dispatchers
			// are retained for the legacy application test seam.
			started := ctx.pluginCommandController.lease != nil
			ctx.pluginCommandController.mu.Unlock()
			if started {
				return "", fmt.Errorf("plugin command Job service is unavailable")
			}
		}
		return "", nil
	}
	summary, err := json.Marshal(map[string]string{"plugin": record.PluginName, "command": record.CommandName})
	if err != nil {
		return "", err
	}
	deps := ctx.jobDeps()
	deps.DB = tx
	snapshot, err := service.Accept(deps, jobs.Acceptance{
		Kind: JobKindPluginCommand, KindVersion: jobPluginCommandVersion,
		State: jobs.StateQueued, ActorUserID: copyCommandUint(record.CreatedByUserID),
		Origin: "plugin", Visibility: jobs.VisibilityAdmin,
		Title: record.CommandName, Summary: summary,
		Replay:     jobs.ReplayInput{NonReplayable: true},
		LegacyRefs: []jobs.LegacyRef{{Namespace: pluginCommandHandleNamespace, Handle: record.ID}},
	})
	if err != nil {
		return "", err
	}
	return snapshot.ID, nil
}

func (ctx *MahresourcesContext) acceptPluginCommandImportJob(tx *gorm.DB, run models.PluginCommandRun, claim models.PluginCommandImport, retryOfJobID string) (string, error) {
	service := ctx.JobService()
	if service == nil {
		if ctx.pluginCommandController != nil {
			ctx.pluginCommandController.mu.Lock()
			started := ctx.pluginCommandController.lease != nil
			ctx.pluginCommandController.mu.Unlock()
			if started {
				return "", fmt.Errorf("plugin command Job service is unavailable")
			}
		}
		return "", nil
	}
	parents := []string(nil)
	if run.JobID != "" {
		parents = append(parents, run.JobID)
	}
	deps := ctx.jobDeps()
	deps.DB = tx
	snapshot, err := service.Accept(deps, jobs.Acceptance{
		Kind: JobKindPluginCommandImport, KindVersion: jobPluginCommandVersion,
		State: jobs.StateQueued, ActorUserID: copyCommandUint(claim.CreatedByUserId),
		Origin: "plugin", Visibility: jobs.VisibilityAdmin,
		Title: "Import " + claim.FileName, Summary: json.RawMessage(`{"source":"plugin-command"}`),
		Replay:     jobs.ReplayInput{NonReplayable: true},
		LegacyRefs: []jobs.LegacyRef{{Namespace: pluginCommandImportHandleNamespace, Handle: claim.ID}}, Parents: parents,
	})
	if err != nil {
		return "", err
	}
	if retryOfJobID != "" {
		if err := service.Link(deps, jobs.LinkRequest{Type: jobs.LinkRetryOf, FromJobID: snapshot.ID, ToJobID: retryOfJobID}); err != nil {
			return "", err
		}
	}
	return snapshot.ID, nil
}

func (ctx *MahresourcesContext) finishPluginCommandJobTx(tx *gorm.DB, jobID, token, sourceStatus, runID string) error {
	service := ctx.JobService()
	if service == nil {
		return fmt.Errorf("plugin command Job service is unavailable")
	}
	var current models.Job
	if err := tx.Where("id = ?", jobID).First(&current).Error; err != nil {
		return err
	}
	outcome := pluginCommandJobState(sourceStatus)
	deps := ctx.jobDeps()
	deps.DB = tx
	ref := jobs.ExecutionRef{JobID: jobID, ExecutionToken: token}
	if outcome == jobs.StateSucceeded {
		reference, err := json.Marshal(map[string]string{"runId": runID})
		if err != nil {
			return err
		}
		if _, err := service.PublishOutput(deps, ref, jobs.OutputInput{Key: "command-history", Type: jobs.OutputTypeLog,
			Label: "Command history and output", Reference: reference}); err != nil {
			return err
		}
	}
	state := jobs.State(current.State)
	if state == outcome {
		return nil
	}
	if state == jobs.StateRunning || state == jobs.StateBlocked {
		if token == "" {
			return fmt.Errorf("plugin command Job %s is owned without an execution token", jobID)
		}
		failure := pluginCommandFailure(sourceStatus)
		_, err := service.Finish(deps, jobs.FinishRequest{ExecutionRef: ref, ExpectedVersion: uint64(current.Version), Outcome: outcome, Failure: failure,
			RequiredOutputs: []string{"command-history"}})
		return err
	}
	if state.Terminal() {
		return fmt.Errorf("plugin command Job %s is already terminal", jobID)
	}
	_, err := service.Transition(deps, jobs.Transition{JobID: jobID, ExpectedVersion: uint64(current.Version), To: outcome, Failure: pluginCommandFailure(sourceStatus)})
	return err
}

func (ctx *MahresourcesContext) finishPluginCommandImportJobTx(tx *gorm.DB, jobID, token, sourceStatus, importID string, resourceID *uint) error {
	service := ctx.JobService()
	if service == nil {
		return fmt.Errorf("plugin command Job service is unavailable")
	}
	var current models.Job
	if err := tx.Where("id = ?", jobID).First(&current).Error; err != nil {
		return err
	}
	state := pluginCommandJobState(sourceStatus)
	deps := ctx.jobDeps()
	deps.DB = tx
	ref := jobs.ExecutionRef{JobID: jobID, ExecutionToken: token}
	if state == jobs.StateSucceeded {
		if resourceID == nil || *resourceID == 0 {
			return fmt.Errorf("successful plugin import %s has no Resource", importID)
		}
		reference, err := json.Marshal(map[string]uint{"resourceId": *resourceID})
		if err != nil {
			return err
		}
		if _, err := service.PublishOutput(deps, ref, jobs.OutputInput{Key: "imported-resource", Type: jobs.OutputTypeEntity,
			Label: "Imported Resource", Reference: reference, Required: true}); err != nil {
			return err
		}
	}
	if jobs.State(current.State) == jobs.StateRunning {
		if token == "" {
			return fmt.Errorf("plugin import Job %s is running without an execution token", jobID)
		}
		_, err := service.Finish(deps, jobs.FinishRequest{ExecutionRef: ref, ExpectedVersion: uint64(current.Version), Outcome: state,
			Failure: pluginCommandFailure(sourceStatus), RequiredOutputs: []string{"imported-resource"}})
		return err
	}
	if jobs.State(current.State) == state {
		return nil
	}
	if jobs.State(current.State).Terminal() {
		return fmt.Errorf("plugin import Job %s is already terminal", jobID)
	}
	_, err := service.Transition(deps, jobs.Transition{JobID: jobID, ExpectedVersion: uint64(current.Version), To: state, Failure: pluginCommandFailure(sourceStatus)})
	return err
}

func pluginCommandJobState(status string) jobs.State {
	switch status {
	case plugin_commands.RunStatusSucceeded:
		return jobs.StateSucceeded
	case plugin_commands.RunStatusCancelled:
		return jobs.StateCancelled
	case plugin_commands.RunStatusInterrupted:
		return jobs.StateInterrupted
	default:
		return jobs.StateFailed
	}
}

func pluginCommandFailure(status string) *jobs.Failure {
	if status != plugin_commands.RunStatusFailed {
		return nil
	}
	return &jobs.Failure{Code: "plugin-command-failed", Class: "internal", Message: "the plugin command did not complete successfully"}
}
