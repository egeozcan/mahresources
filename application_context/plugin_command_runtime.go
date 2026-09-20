package application_context

import (
	"context"
	"errors"
	"fmt"
	"time"

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

type commandProgress struct {
	sink download_queue.ManagedProgressSink
}

func (p commandProgress) SetPhase(phase string) { p.sink.SetPhase(phase) }
func (p commandProgress) SetPhaseProgress(current, total int64) {
	p.sink.SetPhaseProgress(current, total)
}
func (p commandProgress) SetAuthoritativeStatus(status string) {
	p.sink.SetAuthoritativeStatus(status)
}

func (j commandLiveJobs) SubmitCommandJob(spec plugin_commands.RunJobSpec, cancel func(string) error, run func(context.Context, plugin_commands.Progress) plugin_commands.Outcome) (string, error) {
	if j.manager == nil {
		return "", fmt.Errorf("plugin command managed job lane is unavailable")
	}
	job, err := j.manager.SubmitManagedJob(download_queue.ManagedJobOptions{
		JobOptions: download_queue.JobOptions{
			Source: pluginCommandJobSource, InitialPhase: "starting command",
			OwnerUserID: clonePluginCommandActor(spec.OwnerUserID),
		},
		Controls:        download_queue.JobControls{Cancel: true},
		Cancel:          cancel,
		AuthoritativeID: spec.RunID,
	}, func(ctx context.Context, _ *download_queue.DownloadJob, progress download_queue.ManagedProgressSink) download_queue.ManagedJobOutcome {
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
	}}, func(ctx context.Context, _ *download_queue.DownloadJob, progress download_queue.ManagedProgressSink) download_queue.ManagedJobOutcome {
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
	return download_queue.ManagedJobOutcome{Status: status, AuthoritativeStatus: outcome.AuthoritativeStatus, Error: outcome.Error}
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

// StartPluginCommands constructs the application-owned controller. Lease or
// recovery safety contention enters a self-healing quarantine; deterministic
// startup failures still fail the process.
func (ctx *MahresourcesContext) StartPluginCommands(callCtx context.Context, settings plugin_commands.Settings) error {
	return ctx.startPluginCommandsWithConfig(callCtx, settings, defaultPluginCommandControllerConfig())
}

// StartPluginCommandsIfEnabled is the production admission gate. Keeping the
// disabled branch beside construction makes it testable and guarantees that a
// disabled deployment creates no dispatcher goroutine or Lua host surface.
func (ctx *MahresourcesContext) StartPluginCommandsIfEnabled(callCtx context.Context, settings plugin_commands.Settings) error {
	if ctx == nil {
		return fmt.Errorf("plugin command context is unavailable")
	}
	if ctx.Config.PluginsDisabled {
		return nil
	}
	return ctx.StartPluginCommands(callCtx, settings)
}

// StopPluginCommands drains terminal persistence before the download manager
// and plugin manager are stopped. Its error is part of process shutdown: a
// terminal write that did not persist must make the process exit unsuccessfully.
func (ctx *MahresourcesContext) StopPluginCommands() error {
	return ctx.stopPluginCommandsWithin(30 * time.Second)
}

func (ctx *MahresourcesContext) stopPluginCommandsWithin(timeout time.Duration) error {
	if ctx == nil || ctx.pluginCommandController == nil {
		return nil
	}
	controller := ctx.pluginCommandController
	controller.mu.Lock()
	if controller.state == pluginCommandRuntimeIdle {
		controller.mu.Unlock()
		return nil
	}
	if controller.state != pluginCommandRuntimeStopping {
		controller.state = pluginCommandRuntimeStopping
		controller.reason = "plugin command runtime is stopping"
		controller.retryAt = time.Time{}
		if active := controller.active.Swap(nil); active != nil {
			controller.draining = active
		}
		if controller.cancel != nil {
			controller.cancel()
		}
	}
	done := controller.done
	controller.mu.Unlock()
	if done != nil {
		<-done
	}

	controller.mu.Lock()
	draining := controller.draining
	lease := controller.lease
	controller.mu.Unlock()
	var stopErr error
	if draining != nil && draining.dispatcher != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), timeout)
		stopErr = draining.dispatcher.Stop(stopCtx)
		cancel()
		if !draining.dispatcher.RuntimeLeaseReleasable() {
			// Keep the dispatcher and lease owned so a later Stop can confirm full
			// quiescence before releasing the staging root.
			return stopErr
		}
	}
	var leaseErr error
	if lease != nil {
		leaseErr = lease.Close()
	}
	controller.mu.Lock()
	controller.lease = nil
	controller.pending = nil
	controller.pendingExchange = nil
	controller.draining = nil
	controller.cancel = nil
	controller.done = nil
	controller.mu.Unlock()
	return errors.Join(stopErr, leaseErr)
}

// SubmitPluginCommand implements plugin_system.CommandSubmitter.
func (ctx *MahresourcesContext) SubmitPluginCommand(request plugin_commands.CommandRequest) (string, error) {
	active, err := ctx.pluginCommandActive()
	if err != nil {
		return "", err
	}
	return active.dispatcher.Submit(request)
}

func (ctx *MahresourcesContext) CommandRuns(access plugin_commands.Access) ([]plugin_commands.RunView, error) {
	if _, err := ctx.pluginCommandActive(); err != nil {
		return nil, err
	}
	// Durable run/import records are the complete recovery surface. File listing
	// is an explicit, bounded mah.fs.list call; runs() must not walk every
	// historical exchange directory merely to serve a reconciliation query.
	return ctx.Runs(access)
}

func (ctx *MahresourcesContext) ListCommandFiles(access plugin_commands.Access, runID string) (plugin_commands.Listing, error) {
	active, err := ctx.pluginCommandActive()
	if err != nil {
		return plugin_commands.Listing{}, err
	}
	return active.exchange.List(access, runID)
}

func (ctx *MahresourcesContext) ReadCommandFile(access plugin_commands.Access, runID, name string, maxBytes int64) ([]byte, error) {
	active, err := ctx.pluginCommandActive()
	if err != nil {
		return nil, err
	}
	return active.exchange.Read(access, runID, name, maxBytes)
}

func (ctx *MahresourcesContext) SubmitCommandImport(submission plugin_commands.ImportSubmission) (plugin_commands.ImportSubmitResult, error) {
	active, err := ctx.pluginCommandActive()
	if err != nil {
		return plugin_commands.ImportSubmitResult{}, err
	}
	if submission.ActorUserID == nil || *submission.ActorUserID == 0 {
		if ctx.AuthEnabled() {
			return plugin_commands.ImportSubmitResult{}, fmt.Errorf("plugin command import requires an acting user")
		}
		root, err := ctx.RootAdminPrincipal()
		if err != nil {
			return plugin_commands.ImportSubmitResult{}, fmt.Errorf("resolve no-auth plugin command import actor: %w", err)
		}
		if root == nil || root.UserID == 0 {
			return plugin_commands.ImportSubmitResult{}, fmt.Errorf("resolve no-auth plugin command import actor: root administrator is unavailable")
		}
		actor := root.UserID
		submission.ActorUserID = &actor
		accessActor := actor
		submission.Access.ActorUserID = &accessActor
	}
	return active.dispatcher.SubmitImport(submission)
}

func (ctx *MahresourcesContext) DiscardCommandFile(access plugin_commands.Access, runID, name string) error {
	active, err := ctx.pluginCommandActive()
	if err != nil {
		return err
	}
	return active.exchange.Discard(access, runID, name)
}

func (ctx *MahresourcesContext) DiscardCommandRun(access plugin_commands.Access, runID string) error {
	active, err := ctx.pluginCommandActive()
	if err != nil {
		return err
	}
	return active.exchange.DiscardRun(access, runID)
}
