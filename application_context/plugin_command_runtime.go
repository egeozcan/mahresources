package application_context

import (
	"context"
	"fmt"
	"log"

	"mahresources/download_queue"
	"mahresources/plugin_commands"
)

const (
	pluginCommandJobSource = "plugin-command"
	pluginImportJobSource  = "plugin-command-import"
)

// commandLiveJobs adapts the shared jobs cockpit without putting pending
// command work into its registry. Dispatcher workers own admission first; only
// running work enters the independently capped managed lane.
type commandLiveJobs struct {
	manager *download_queue.DownloadManager
}

type commandProgress struct{ sink download_queue.ProgressSink }

func (p commandProgress) SetPhase(phase string) { p.sink.SetPhase(phase) }
func (p commandProgress) SetPhaseProgress(current, total int64) {
	p.sink.SetPhaseProgress(current, total)
}

func (j commandLiveJobs) SubmitCommandJob(spec plugin_commands.RunJobSpec, cancel func(string) error, run func(context.Context, plugin_commands.Progress) plugin_commands.Outcome) (string, error) {
	if j.manager == nil {
		return "", fmt.Errorf("plugin command managed job lane is unavailable")
	}
	job, err := j.manager.SubmitManagedJob(download_queue.ManagedJobOptions{
		JobOptions: download_queue.JobOptions{
			Source: pluginCommandJobSource, InitialPhase: plugin_commands.RunStatusRunning,
			OwnerUserID: clonePluginCommandActor(spec.OwnerUserID),
		},
		Controls: download_queue.JobControls{Cancel: true},
		Cancel:   cancel,
	}, func(ctx context.Context, _ *download_queue.DownloadJob, progress download_queue.ProgressSink) download_queue.ManagedJobOutcome {
		return managedCommandOutcome(run(ctx, commandProgress{sink: progress}))
	})
	if err != nil {
		return "", err
	}
	return job.ID, nil
}

func (j commandLiveJobs) SubmitImportJob(spec plugin_commands.ImportJobSpec, run func(context.Context, plugin_commands.Progress) plugin_commands.Outcome) (string, error) {
	if j.manager == nil {
		return "", fmt.Errorf("plugin command managed job lane is unavailable")
	}
	job, err := j.manager.SubmitManagedJob(download_queue.ManagedJobOptions{JobOptions: download_queue.JobOptions{
		Source: pluginImportJobSource, InitialPhase: plugin_commands.ImportStatusRunning,
		OwnerUserID: clonePluginCommandActor(spec.OwnerUserID),
	}}, func(ctx context.Context, _ *download_queue.DownloadJob, progress download_queue.ProgressSink) download_queue.ManagedJobOutcome {
		return managedCommandOutcome(run(ctx, commandProgress{sink: progress}))
	})
	if err != nil {
		return "", err
	}
	return job.ID, nil
}

func managedCommandOutcome(outcome plugin_commands.Outcome) download_queue.ManagedJobOutcome {
	status := download_queue.JobStatusFailed
	switch outcome.Status {
	case plugin_commands.RunStatusSucceeded:
		status = download_queue.JobStatusCompleted
	case plugin_commands.RunStatusCancelled:
		status = download_queue.JobStatusCancelled
	}
	return download_queue.ManagedJobOutcome{Status: status, AuthoritativeStatus: outcome.Status, Error: outcome.Error}
}

func clonePluginCommandActor(actor *uint) *uint {
	if actor == nil {
		return nil
	}
	copyOfActor := *actor
	return &copyOfActor
}

// commandSettings reserves the deployment-wide queue ceiling for each
// plugin's pending dispatcher work while delegating every live runtime setting.
type commandSettings struct{ plugin_commands.Settings }

func (s commandSettings) PendingPerPluginLimit() int { return download_queue.MaxQueueSize }

// StartPluginCommands constructs the application-owned host, recovers durable
// state, starts the dispatcher, and only then publishes mah.commands/mah.fs
// availability. Task 11 owns calling and stopping this lifecycle from main.
func (ctx *MahresourcesContext) StartPluginCommands(callCtx context.Context, settings plugin_commands.Settings) error {
	if settings == nil {
		return fmt.Errorf("plugin command settings are required")
	}
	if ctx.pluginManager == nil {
		return fmt.Errorf("plugin manager is unavailable")
	}
	wrappedSettings := commandSettings{Settings: settings}
	leases := plugin_commands.NewLeaseManager()
	executor := plugin_commands.NewExecutor(plugin_commands.RunnerDependencies{
		Store: ctx, Settings: wrappedSettings, Logf: log.Printf,
	})
	dispatcher := plugin_commands.NewDispatcher(plugin_commands.Dependencies{
		Store: ctx, Jobs: commandLiveJobs{manager: ctx.downloadManager}, Executor: executor,
		Settings: wrappedSettings, Leases: leases, Logf: log.Printf,
	})
	dispatcher.SetImporter(ctx)
	if err := dispatcher.Recover(callCtx); err != nil {
		return fmt.Errorf("recover plugin commands: %w", err)
	}
	if err := dispatcher.Start(callCtx); err != nil {
		return fmt.Errorf("start plugin commands: %w", err)
	}

	exchange := plugin_commands.NewExchangeWithLeases(ctx, wrappedSettings, leases)
	ctx.pluginCommandDispatcher = dispatcher
	ctx.pluginCommandExchange = exchange
	ctx.pluginManager.SetCommandSubmitter(ctx)
	ctx.pluginManager.SetExchangeMediator(ctx)
	return nil
}

// SubmitPluginCommand implements plugin_system.CommandSubmitter.
func (ctx *MahresourcesContext) SubmitPluginCommand(request plugin_commands.CommandRequest) (string, error) {
	if ctx.pluginCommandDispatcher == nil || ctx.pluginCommandExchange == nil {
		return "", fmt.Errorf("plugin commands are not available until startup recovery completes")
	}
	return ctx.pluginCommandDispatcher.Submit(request)
}

func (ctx *MahresourcesContext) CommandRuns(access plugin_commands.Access) ([]plugin_commands.RunView, error) {
	if ctx.pluginCommandExchange == nil {
		return nil, fmt.Errorf("plugin exchange files are not available until startup recovery completes")
	}
	// Durable run/import records are the complete recovery surface. File listing
	// is an explicit, bounded mah.fs.list call; runs() must not walk every
	// historical exchange directory merely to serve a reconciliation query.
	return ctx.Runs(access)
}

func (ctx *MahresourcesContext) ListCommandFiles(access plugin_commands.Access, runID string) (plugin_commands.Listing, error) {
	if ctx.pluginCommandExchange == nil {
		return plugin_commands.Listing{}, fmt.Errorf("plugin exchange files are not available until startup recovery completes")
	}
	return ctx.pluginCommandExchange.List(access, runID)
}

func (ctx *MahresourcesContext) ReadCommandFile(access plugin_commands.Access, runID, name string, maxBytes int64) ([]byte, error) {
	if ctx.pluginCommandExchange == nil {
		return nil, fmt.Errorf("plugin exchange files are not available until startup recovery completes")
	}
	return ctx.pluginCommandExchange.Read(access, runID, name, maxBytes)
}

func (ctx *MahresourcesContext) SubmitCommandImport(submission plugin_commands.ImportSubmission) (plugin_commands.ImportSubmitResult, error) {
	if ctx.pluginCommandDispatcher == nil || ctx.pluginCommandExchange == nil {
		return plugin_commands.ImportSubmitResult{}, fmt.Errorf("plugin command imports are not available until startup recovery completes")
	}
	return ctx.pluginCommandDispatcher.SubmitImport(submission)
}

func (ctx *MahresourcesContext) DiscardCommandFile(access plugin_commands.Access, runID, name string) error {
	if ctx.pluginCommandExchange == nil {
		return fmt.Errorf("plugin exchange files are not available until startup recovery completes")
	}
	return ctx.pluginCommandExchange.Discard(access, runID, name)
}

func (ctx *MahresourcesContext) DiscardCommandRun(access plugin_commands.Access, runID string) error {
	if ctx.pluginCommandExchange == nil {
		return fmt.Errorf("plugin exchange files are not available until startup recovery completes")
	}
	return ctx.pluginCommandExchange.DiscardRun(access, runID)
}
