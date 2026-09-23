package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"mahresources/contracts"
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

// pluginCommandRunReplayInput holds the accepted command values in the sealed
// Job envelope. The legacy source row is a compatibility projection only.
type pluginCommandRunReplayInput struct {
	PluginName  string `json:"pluginName"`
	CommandName string `json:"commandName"`
	ParamsJSON  string `json:"paramsJson"`
	InputsJSON  string `json:"inputsJson,omitempty"`
}

type pluginCommandImportReplayInput struct {
	RunID            string `json:"runId"`
	FileName         string `json:"fileName"`
	FieldsJSON       string `json:"fieldsJson"`
	PluginGeneration uint64 `json:"pluginGeneration"`
}

func pluginCommandReplayCodec(kind string) jobs.ReplayCodec {
	return jobs.ReplayCodec{
		Sanitize: func(input json.RawMessage) (json.RawMessage, error) {
			if kind == JobKindPluginCommand {
				var decoded pluginCommandRunReplayInput
				if err := json.Unmarshal(input, &decoded); err != nil || decoded.PluginName == "" || decoded.CommandName == "" || !json.Valid([]byte(decoded.ParamsJSON)) || (decoded.InputsJSON != "" && !json.Valid([]byte(decoded.InputsJSON))) {
					return nil, fmt.Errorf("invalid plugin command replay input")
				}
				return json.Marshal(map[string]string{"plugin": decoded.PluginName, "command": decoded.CommandName})
			}
			var decoded pluginCommandImportReplayInput
			if err := json.Unmarshal(input, &decoded); err != nil || decoded.RunID == "" || decoded.FileName == "" || decoded.FieldsJSON == "" || !json.Valid([]byte(decoded.FieldsJSON)) {
				return nil, fmt.Errorf("invalid plugin command import replay input")
			}
			return json.Marshal(map[string]string{"source": "plugin-command", "runId": decoded.RunID})
		},
		Encode: func(input json.RawMessage) (json.RawMessage, error) {
			if _, err := pluginCommandReplayCodec(kind).Sanitize(input); err != nil {
				return nil, err
			}
			return input, nil
		},
		Decode: func(payload json.RawMessage, version uint) (json.RawMessage, error) {
			if version != jobPluginCommandVersion {
				return nil, fmt.Errorf("%w: plugin command v%d input", jobs.ErrReplayCodecUnregistered, version)
			}
			if _, err := pluginCommandReplayCodec(kind).Sanitize(payload); err != nil {
				return nil, err
			}
			return payload, nil
		},
		Migrate: func(payload json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error) {
			if fromVersion != toVersion {
				return nil, fmt.Errorf("jobs: no plugin command input migration from v%d to v%d", fromVersion, toVersion)
			}
			return payload, nil
		},
	}
}

type pluginCommandJobAdapter struct {
	ctx  *MahresourcesContext
	kind string
}

// AuthorizeJobOutput checks the current administrator role and, for command
// history, binds the stored run reference to this exact canonical Job. The
// history reference alone is never authority to read a run.
func (a *pluginCommandJobAdapter) AuthorizeJobOutput(_ context.Context, request JobOutputOpenRequest) error {
	if a == nil || a.ctx == nil || request.Principal == nil || !request.Principal.IsAdmin() {
		return ErrJobOutputForbidden
	}
	if !request.Principal.SuperUser {
		current := commandActorOn(a.ctx.db, request.Principal.UserID)
		if current == nil || !current.IsAdmin() {
			return ErrJobOutputForbidden
		}
	}
	if a.kind != JobKindPluginCommand {
		return nil
	}
	if request.Output.Key != "command-history" || request.Output.Type != jobs.OutputTypeLog {
		return ErrJobOutputForbidden
	}
	runID, err := pluginCommandOutputRunID(request.Output.Reference)
	if err != nil {
		return ErrJobOutputForbidden
	}
	var source models.PluginCommandRun
	if err := a.ctx.db.Select("job_id").Where("id = ?", runID).First(&source).Error; err != nil || source.JobID != request.Snapshot.ID {
		return ErrJobOutputForbidden
	}
	return nil
}

// OpenJobOutput returns bounded command metadata. A command's retained stdout
// and stderr may contain arbitrary secrets, so this representation never reads
// raw output_tail; it reports only whether a redacted tail is present.
func (a *pluginCommandJobAdapter) OpenJobOutput(requestCtx context.Context, request JobOutputOpenRequest) (contracts.JobOutputContent, error) {
	if err := a.AuthorizeJobOutput(requestCtx, request); err != nil {
		return contracts.JobOutputContent{}, err
	}
	if a.kind != JobKindPluginCommand {
		return a.ctx.openStandardJobOutput(request.Output)
	}
	runID, err := pluginCommandOutputRunID(request.Output.Reference)
	if err != nil {
		return contracts.JobOutputContent{}, ErrJobOutputInvalid
	}
	var source models.PluginCommandRun
	if err := a.ctx.db.Select("id, job_id, plugin_name, command_name, status").Where("id = ? AND job_id = ?", runID, request.Snapshot.ID).
		First(&source).Error; err != nil {
		return contracts.JobOutputContent{}, ErrJobOutputForbidden
	}
	var tailPresent int
	result := a.ctx.db.Raw(`SELECT CASE WHEN output_tail IS NOT NULL AND output_tail <> '' THEN 1 ELSE 0 END
		FROM plugin_command_run_outputs WHERE run_id = ? LIMIT 1`, runID).Scan(&tailPresent)
	if result.Error != nil {
		return contracts.JobOutputContent{}, result.Error
	}
	response := struct {
		RunID      string `json:"runId"`
		Plugin     string `json:"plugin"`
		Command    string `json:"command"`
		Status     string `json:"status"`
		OutputTail string `json:"outputTail,omitempty"`
	}{RunID: runID, Plugin: source.PluginName, Command: source.CommandName, Status: source.Status}
	if tailPresent != 0 {
		response.OutputTail = "[redacted]"
	}
	data, err := json.Marshal(response)
	if err != nil {
		return contracts.JobOutputContent{}, err
	}
	return contracts.JobOutputContent{Data: data, ContentType: "application/json"}, nil
}

func pluginCommandOutputRunID(reference json.RawMessage) (string, error) {
	var stored struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(reference, &stored); err != nil || stored.RunID == "" || len(stored.RunID) > 32 {
		return "", ErrJobOutputInvalid
	}
	return stored.RunID, nil
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
	if db.Where("job_id = ?", jobID).First(&source).Error != nil || source.CreatedByUserId == nil {
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
	fieldsJSON, ok := a.importFieldsJSON(db, source)
	if !ok {
		return false
	}
	var fields plugin_commands.ResourceFields
	if json.Unmarshal([]byte(fieldsJSON), &fields) != nil {
		return false
	}
	return true
}

func (a *pluginCommandJobAdapter) importFieldsJSON(db *gorm.DB, source models.PluginCommandImport) (string, bool) {
	if source.FieldsJSON != "" {
		return source.FieldsJSON, true
	}
	retired, err := pluginCommandInputsRetired(db)
	if err != nil || !retired || source.JobID == "" || a.ctx.JobService() == nil {
		return "", false
	}
	opened, err := a.ctx.JobService().OpenReplay(a.ctx.jobDepsWithDB(db), jobs.Access{Administrator: true}, source.JobID)
	if err != nil {
		return "", false
	}
	var input pluginCommandImportReplayInput
	if json.Unmarshal(opened.Input, &input) != nil || input.RunID != source.RunID || input.FileName != source.FileName || input.FieldsJSON == "" {
		return "", false
	}
	return input.FieldsJSON, true
}

func (a *pluginCommandJobAdapter) importExchangeFilePresent(db *gorm.DB, jobID string) bool {
	if db == nil || a.ctx == nil {
		return false
	}
	var source models.PluginCommandImport
	if db.Where("job_id = ?", jobID).First(&source).Error != nil {
		return false
	}
	var run models.PluginCommandRun
	if db.Where("id = ?", source.RunID).First(&run).Error != nil {
		return false
	}
	controller := a.ctx.pluginCommandController
	if controller == nil {
		return false
	}
	controller.mu.Lock()
	active := controller.active.Load()
	controller.mu.Unlock()
	if active == nil || active.exchange == nil {
		return false
	}
	probe, ok := active.exchange.(interface {
		HasRegularFile(pluginName, runID, name string) bool
	})
	return ok && probe.HasRegularFile(run.PluginName, run.ID, source.FileName)
}

func (a *pluginCommandJobAdapter) retryImport(execution jobs.CommandExecution) (jobs.CommandOutcome, error) {
	if !a.importRetryable(a.ctx.db, execution.JobID) {
		return jobs.CommandOutcome{}, fmt.Errorf("plugin command import is no longer safely retryable")
	}
	var source models.PluginCommandImport
	if err := a.ctx.db.Where("job_id = ?", execution.JobID).First(&source).Error; err != nil {
		return jobs.CommandOutcome{}, fmt.Errorf("plugin command import source is unavailable")
	}
	fieldsJSON, ok := a.importFieldsJSON(a.ctx.db, source)
	if !ok {
		return jobs.CommandOutcome{}, fmt.Errorf("plugin command import fields are unavailable")
	}
	var fields plugin_commands.ResourceFields
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
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
		if !jobs.HasReplayCodec(service, kind, jobPluginCommandVersion) {
			if err := service.RegisterReplayCodec(kind, jobPluginCommandVersion, pluginCommandReplayCodec(kind)); err != nil {
				return err
			}
		}
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
	deps := ctx.jobDeps()
	deps.DB = tx
	input, err := json.Marshal(pluginCommandRunReplayInput{
		PluginName: record.PluginName, CommandName: record.CommandName,
		ParamsJSON: record.ParamsJSON, InputsJSON: encodeSuppliedInputs(record.Inputs),
	})
	if err != nil {
		return "", fmt.Errorf("encode plugin command replay input")
	}
	snapshot, err := service.Accept(deps, jobs.Acceptance{
		Kind: JobKindPluginCommand, KindVersion: jobPluginCommandVersion,
		State: jobs.StateQueued, ActorUserID: copyCommandUint(record.CreatedByUserID),
		Origin: "plugin", Visibility: jobs.VisibilityAdmin,
		Title:      record.CommandName,
		Replay:     jobs.ReplayInput{Input: input},
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
	input, err := json.Marshal(pluginCommandImportReplayInput{
		RunID: claim.RunID, FileName: claim.FileName, FieldsJSON: claim.FieldsJSON,
		PluginGeneration: claim.PluginGeneration,
	})
	if err != nil {
		return "", fmt.Errorf("encode plugin command import replay input")
	}
	snapshot, err := service.Accept(deps, jobs.Acceptance{
		Kind: JobKindPluginCommandImport, KindVersion: jobPluginCommandVersion,
		State: jobs.StateQueued, ActorUserID: copyCommandUint(claim.CreatedByUserId),
		Origin: "plugin", Visibility: jobs.VisibilityAdmin,
		Title:      "Import " + claim.FileName,
		Replay:     jobs.ReplayInput{Input: input},
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
