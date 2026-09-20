//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package plugin_commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	quotaSampleInterval    = time.Second
	groupPollInterval      = 20 * time.Millisecond
	stuckGroupPollInterval = time.Second
	groupDrainTimeout      = 10 * time.Second
	stuckGroupWarningAfter = time.Minute
)

type commandExecutor struct {
	deps              RunnerDependencies
	cleanupTimeout    time.Duration
	quotaInterval     time.Duration
	pollInterval      time.Duration
	stuckPollInterval time.Duration
	stuckWarningAfter time.Duration
	// Test barriers sit on the cancellation boundaries which must remain closed:
	// before the per-run fork lock, after the durable check while that lock is
	// held, and after Start before pgid persistence. Nil in production.
	beforeStart            func()
	afterCancellationCheck func()
	afterStart             func()
}

func NewExecutor(deps RunnerDependencies) Executor {
	if deps.Logf == nil {
		deps.Logf = func(string, ...any) {}
	}
	if deps.Warn == nil {
		deps.Warn = func(RuntimeWarning) {}
	}
	if deps.Inspector == nil {
		deps.Inspector = nativeProcessInspector{}
	}
	if deps.Usage == nil {
		deps.Usage = NewStagingUsageCache()
	}
	return &commandExecutor{
		deps: deps, cleanupTimeout: groupDrainTimeout, quotaInterval: quotaSampleInterval,
		pollInterval: groupPollInterval, stuckPollInterval: stuckGroupPollInterval,
		stuckWarningAfter: stuckGroupWarningAfter,
	}
}

func (e *commandExecutor) stagingUsageCache() *StagingUsageCache { return e.deps.Usage }

func (e *commandExecutor) Prepare(run QueuedRun) error {
	if e.deps.Store == nil || e.deps.Settings == nil {
		return fmt.Errorf("plugin_commands: runner dependencies are incomplete")
	}
	root := filepath.Clean(e.deps.Settings.StagingRoot())
	if !filepath.IsAbs(root) {
		return fmt.Errorf("plugin command staging root must be absolute")
	}
	if err := ensureRunPath(root, run); err != nil {
		return err
	}
	usage, err := e.deps.Usage.Current()
	if err != nil {
		return fmt.Errorf("measure global staging quota: %w", err)
	}
	limit := effectiveQuota(e.deps.Settings.GlobalStagingQuota(), defaultGlobalStagingQuota)
	if usage > limit {
		return fmt.Errorf("global staging quota exceeded: %d bytes used, limit %d", usage, limit)
	}

	pluginDir := filepath.Dir(run.ExchangeDir)
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		return fmt.Errorf("create plugin exchange parent: %w", err)
	}
	if err := os.Chmod(pluginDir, 0o700); err != nil {
		return fmt.Errorf("secure plugin exchange parent: %w", err)
	}
	if err := os.Mkdir(run.ExchangeDir, 0o700); err != nil {
		return fmt.Errorf("create private exchange directory: %w", err)
	}
	if err := os.Chmod(run.ExchangeDir, 0o700); err != nil {
		e.Cleanup(run)
		return fmt.Errorf("secure exchange directory: %w", err)
	}
	tmpDir := filepath.Join(run.ExchangeDir, ".tmp")
	if err := os.Mkdir(tmpDir, 0o700); err != nil {
		e.Cleanup(run)
		return fmt.Errorf("create private command temp directory: %w", err)
	}
	if err := os.Chmod(tmpDir, 0o700); err != nil {
		e.Cleanup(run)
		return fmt.Errorf("secure command temp directory: %w", err)
	}
	return nil
}

func ensureRunPath(root string, run QueuedRun) error {
	if run.RunID == "" || run.Request.PluginName == "" {
		return fmt.Errorf("plugin command run requires id and plugin name")
	}
	want := filepath.Join(root, "plugin_exchange", run.Request.PluginName, run.RunID)
	if filepath.Clean(run.ExchangeDir) != want {
		return fmt.Errorf("plugin command exchange directory is outside its managed run path")
	}
	for _, value := range []string{run.Request.PluginName, run.RunID} {
		if value == "." || value == ".." || strings.ContainsAny(value, `/\\`) {
			return fmt.Errorf("plugin command managed path component is invalid")
		}
	}
	return nil
}

func (e *commandExecutor) Cleanup(run QueuedRun) {
	root := filepath.Clean(e.deps.Settings.StagingRoot())
	if ensureRunPath(root, run) != nil {
		return
	}
	if err := os.RemoveAll(run.ExchangeDir); err != nil {
		e.deps.Logf("remove rejected plugin command exchange %s: %v", run.RunID, err)
	}
}

func (e *commandExecutor) Execute(ctx context.Context, run QueuedRun) Outcome {
	if e.deps.Store == nil || e.deps.Settings == nil {
		return Outcome{Status: RunStatusFailed, Error: "plugin command runner dependencies are incomplete"}
	}
	started := time.Now().UTC()
	won, err := e.deps.Store.MarkRunRunning(run.RunID, started)
	if err != nil {
		return Outcome{Status: RunStatusFailed, Error: fmt.Sprintf("mark command running: %v", err)}
	}
	if !won {
		return e.finishWithoutStart(run, "command was no longer queued")
	}
	if run.progress != nil {
		run.progress.SetAuthoritativeStatus(RunStatusRunning)
	}
	if outcome, stop := e.stopBeforeStart(ctx, run); stop {
		return outcome
	}

	commandPath, err := normalizeCommandPath(e.deps.Settings.CommandPath())
	if err != nil {
		return e.finish(run, RunFinish{Status: RunStatusFailed, Error: err.Error(), FinishedAt: time.Now().UTC()})
	}
	path, err := resolveExecutable(run.Invocation.Argv[0], commandPath)
	if err != nil {
		return e.finish(run, RunFinish{Status: RunStatusFailed, Error: err.Error(), FinishedAt: time.Now().UTC()})
	}
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		return e.finish(run, RunFinish{Status: RunStatusFailed, Error: fmt.Sprintf("open null stdin: %v", err), FinishedAt: time.Now().UTC()})
	}
	defer stdin.Close()

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return e.finish(run, RunFinish{Status: RunStatusFailed, Error: fmt.Sprintf("create stdout pipe: %v", err), FinishedAt: time.Now().UTC()})
	}
	defer stdoutR.Close()
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		stdoutR.Close()
		stdoutW.Close()
		return e.finish(run, RunFinish{Status: RunStatusFailed, Error: fmt.Sprintf("create stderr pipe: %v", err), FinishedAt: time.Now().UTC()})
	}
	defer stderrR.Close()

	tail := newOutputTail()
	var drains sync.WaitGroup
	drains.Add(2)
	drainDone := make(chan struct{})
	go func() { defer drains.Done(); _, _ = io.Copy(tail, stdoutR) }()
	go func() { defer drains.Done(); _, _ = io.Copy(tail, stderrR) }()
	go func() { drains.Wait(); close(drainDone) }()

	cmd := &exec.Cmd{
		Path: path,
		Args: append([]string(nil), run.Invocation.Argv...),
		Dir:  run.ExchangeDir,
		Env: []string{
			"PATH=" + commandPath,
			"HOME=" + os.Getenv("HOME"),
			"TMPDIR=" + filepath.Join(run.ExchangeDir, ".tmp"),
			"LANG=" + os.Getenv("LANG"),
			"TZ=" + os.Getenv("TZ"),
			"MAHR_PLUGIN_NAME=" + run.Request.PluginName,
			"MAHR_COMMAND_RUN_ID=" + run.RunID,
			"MAHR_EXCHANGE_DIR=" + run.ExchangeDir,
		},
		Stdin: stdin, Stdout: stdoutW, Stderr: stderrW,
		SysProcAttr: &syscall.SysProcAttr{Setpgid: true},
	}
	// Order the final durable-latch read and Start against Dispatcher.Cancel.
	// Cancel takes this same per-run lock before persisting its latch. Therefore a
	// cancellation is either visible here and no process starts, or Start wins
	// first and cancellation follows the post-fork kill/reap path.
	if e.beforeStart != nil {
		e.beforeStart()
	}
	control := run.control
	if control == nil {
		control = &runControl{}
	}
	control.forkMu.Lock()
	if outcome, stop := e.stopBeforeStart(ctx, run); stop {
		control.forkMu.Unlock()
		stdoutW.Close()
		stderrW.Close()
		<-drainDone
		return outcome
	}
	if e.afterCancellationCheck != nil {
		e.afterCancellationCheck()
	}
	if control.cancelled.Load() {
		control.forkMu.Unlock()
		stdoutW.Close()
		stderrW.Close()
		<-drainDone
		if outcome, stop := e.stopBeforeStart(ctx, run); stop {
			return outcome
		}
		return e.finish(run, RunFinish{Status: RunStatusCancelled, Error: e.cancelReason(run.RunID), FinishedAt: time.Now().UTC()})
	}
	startErr := cmd.Start()
	control.forkMu.Unlock()
	if startErr != nil {
		stdoutW.Close()
		stderrW.Close()
		<-drainDone
		return e.finish(run, RunFinish{Status: RunStatusFailed, Error: fmt.Sprintf("start command: %v", startErr), OutputTail: tail.String(), FinishedAt: time.Now().UTC()})
	}
	if e.afterStart != nil {
		e.afterStart()
	}

	// The timeout belongs to the spawned process, not to the bookkeeping that
	// follows it. In particular, SetRunProcessGroup may wait on a busy database.
	timer := time.NewTimer(run.Request.Declaration.Timeout)
	defer timer.Stop()
	stdoutW.Close()
	stderrW.Close()
	pgid := cmd.Process.Pid

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	pgidDone := make(chan error, 1)
	go func() { pgidDone <- e.deps.Store.SetRunProcessGroup(run.RunID, pgid, e.deps.BootSessionID) }()
	quotaInterval := e.quotaInterval
	if quotaInterval <= 0 {
		quotaInterval = quotaSampleInterval
	}
	quotaTicker := time.NewTicker(quotaInterval)
	defer quotaTicker.Stop()
	pollInterval := e.pollInterval
	if pollInterval <= 0 {
		pollInterval = groupPollInterval
	}
	stuckPollInterval := e.stuckPollInterval
	if stuckPollInterval <= 0 {
		stuckPollInterval = stuckGroupPollInterval
	}
	stuckWarningAfter := e.stuckWarningAfter
	if stuckWarningAfter <= 0 {
		stuckWarningAfter = stuckGroupWarningAfter
	}
	var groupTicker *time.Ticker
	var groupTick <-chan time.Time
	startGroupPolling := func() {
		if groupTicker != nil {
			return
		}
		groupTicker = time.NewTicker(pollInterval)
		groupTick = groupTicker.C
	}
	defer func() {
		if groupTicker != nil {
			groupTicker.Stop()
		}
	}()

	cleanupTimeout := e.cleanupTimeout
	if cleanupTimeout <= 0 {
		cleanupTimeout = groupDrainTimeout
	}
	var waitErr error
	parentDone, pipesDone, processGroupRecorded := false, false, false
	status, reason := "", ""
	ctxDone := ctx.Done()
	timerDone := timer.C
	outputUnverified := false
	forcedCleanup := false
	groupDead := false
	groupSignalAttempts := 0
	lastInspectionAllowsResignal := false
	resignalOpportunityUsed := false
	warningEmitted := false
	var cleanupDeadline time.Time
	var cleanupStarted time.Time
	appendReason := func(message string) {
		if message == "" || strings.Contains(reason, message) {
			return
		}
		if reason == "" {
			reason = message
		} else {
			reason += "; " + message
		}
	}
	signalGroup := func(allowResignal bool) {
		startGroupPolling()
		if groupSignalAttempts >= 2 {
			return
		}
		if groupSignalAttempts > 0 && (!allowResignal || !lastInspectionAllowsResignal) {
			return
		}
		// The first attempt rests on this worker's creation-time authority. The
		// only later attempt is bounded to the first forced-cleanup edge and needs
		// an immediately preceding successful observation that the group is alive.
		groupSignalAttempts++
		if err := e.deps.Inspector.KillGroup(pgid); err != nil && !errors.Is(err, syscall.ESRCH) {
			appendReason(fmt.Sprintf("kill process group: %v", err))
			if processErr := cmd.Process.Kill(); processErr != nil && !errors.Is(processErr, os.ErrProcessDone) {
				appendReason(fmt.Sprintf("kill command process: %v", processErr))
			}
		}
	}
	killGroup := func() { signalGroup(false) }
	terminate := func(nextStatus, nextReason string, cancellationWins bool) {
		if status == "" || cancellationWins {
			status, reason = nextStatus, nextReason
		}
		if cleanupDeadline.IsZero() {
			cleanupDeadline = time.Now().Add(cleanupTimeout)
		}
		killGroup()
	}

	for {
		inspectionDue := false
		groupInspectionDue := false
		select {
		case <-ctxDone:
			ctxDone = nil
			cancelStatus, cancelReason := e.contextTermination(ctx, run.RunID)
			terminate(cancelStatus, cancelReason, true)
			inspectionDue = true
		case <-timerDone:
			timerDone = nil
			terminate(RunStatusFailed, fmt.Sprintf("command timeout exceeded (%s)", run.Request.Declaration.Timeout), false)
			inspectionDue = true
		case <-quotaTicker.C:
			usage, usageErr := runUsage(e.deps.Store, e.deps.Settings.StagingRoot(), run.RunID, run.ExchangeDir)
			if usageErr != nil {
				terminate(RunStatusFailed, usageErr.Error(), false)
				inspectionDue = true
			} else if limit := effectiveQuota(e.deps.Settings.PerRunQuota(), defaultPerRunQuota); usage > limit {
				terminate(RunStatusFailed, fmt.Sprintf("per-run quota exceeded: %d bytes used, limit %d", usage, limit), false)
				inspectionDue = true
			}
		case waitErr = <-waitDone:
			parentDone = true
			waitDone = nil
			startGroupPolling()
			if cleanupDeadline.IsZero() {
				cleanupDeadline = time.Now().Add(cleanupTimeout)
			}
			inspectionDue = true
		case <-drainDone:
			pipesDone = true
			drainDone = nil
		case persistErr := <-pgidDone:
			processGroupRecorded = true
			pgidDone = nil
			if persistErr != nil {
				terminate(RunStatusFailed, fmt.Sprintf("persist command process group: %v", persistErr), false)
				inspectionDue = true
			} else if status != "" {
				killGroup()
			}
		case <-groupTick:
			inspectionDue = true
			groupInspectionDue = true
		}

		// After forced cleanup only the backed-off group ticker may inspect. State
		// changes remain recorded immediately, but their process-table observation
		// can safely wait for that ticker.
		if forcedCleanup {
			inspectionDue = groupInspectionDue
		}
		// An auxiliary event can be the first wakeup at either pre-backoff
		// cleanup edge. Take the observation needed for the transition there, but
		// once forced cleanup begins only group ticks may drive inspection. Quota
		// sampling and warning timing remain independent.
		if !forcedCleanup && !cleanupDeadline.IsZero() && !time.Now().Before(cleanupDeadline) {
			inspectionDue = true
		}
		if inspectionDue && (parentDone || status != "") {
			lastInspectionAllowsResignal = false
			identity, inspectErr := e.deps.Inspector.InspectGroup(pgid, run.RunID)
			groupDead = inspectErr == nil && identity.State == GroupDead
			if inspectErr != nil {
				outputUnverified = true
				terminate(RunStatusFailed, fmt.Sprintf("inspect process group: %v", inspectErr), false)
			} else {
				lastInspectionAllowsResignal = identity.State == GroupAliveOwned || identity.State == GroupAliveUnverified
			}
		}
		if parentDone && pipesDone && groupDead {
			// Prefer a persistence result which became ready alongside the final
			// process events; select is otherwise free to observe group death first.
			if !processGroupRecorded && pgidDone != nil {
				select {
				case persistErr := <-pgidDone:
					processGroupRecorded = true
					pgidDone = nil
					if persistErr != nil {
						status = RunStatusFailed
						appendReason(fmt.Sprintf("persist command process group: %v", persistErr))
					}
				default:
				}
			}
			if !processGroupRecorded {
				if persisted, _, readErr := e.deps.Store.Run(run.RunID); readErr == nil &&
					persisted.ProcessGroupID != nil && *persisted.ProcessGroupID == pgid {
					processGroupRecorded = true
				}
			}
			if !processGroupRecorded {
				outputUnverified = true
				appendReason("process-group persistence did not finish before command cleanup")
			}
			break
		}
		if forcedCleanup && !warningEmitted && !groupDead && time.Since(cleanupStarted) >= stuckWarningAfter {
			warningEmitted = true
			e.deps.Warn(RuntimeWarning{
				Event: RuntimeWarningEventPinnedSlot, Message: "process group remains alive after forced cleanup",
				RunID: run.RunID, ProcessGroupID: pgid, ActiveLimit: maxActiveCommands,
			})
		}
		if cleanupDeadline.IsZero() || time.Now().Before(cleanupDeadline) {
			continue
		}

		if status == "" && !groupDead {
			// A parent may exit while descendants continue. Give the group one
			// bounded grace period, then terminate it and allow one bounded reap.
			terminate(RunStatusFailed, "process group remained alive after command exit", false)
			cleanupDeadline = time.Now().Add(cleanupTimeout)
			continue
		}

		if !forcedCleanup {
			forcedCleanup = true
			cleanupStarted = time.Now()
			if groupTicker != nil {
				groupTicker.Reset(stuckPollInterval)
			}
		}
		if !groupDead {
			outputUnverified = true
			appendReason("process group did not exit before cleanup deadline")
			if !resignalOpportunityUsed {
				resignalOpportunityUsed = true
				signalGroup(true)
			}
		}
		if !pipesDone {
			outputUnverified = true
			if status == "" {
				status = RunStatusFailed
			}
			appendReason("output pipes did not close before cleanup deadline")
			_ = stdoutR.Close()
			_ = stderrR.Close()
		}
		if !processGroupRecorded {
			outputUnverified = true
			if status == "" {
				status = RunStatusFailed
			}
			appendReason("process-group persistence did not finish before cleanup deadline")
		}
		if parentDone && pipesDone && groupDead {
			break
		}
		// Keep polling until inspection proves the locally owned group dead.
		// Neither an unverifiable member environment nor an inspection failure can
		// turn possible surviving writers into terminal output.
		cleanupDeadline = time.Now().Add(cleanupTimeout)
	}

	// A final sample closes the sub-second successful-writer hole. Sampling is
	// still approximate while a command runs, but an over-limit run can never
	// be published as succeeded merely because it exited before the first tick.
	if status == "" {
		usage, usageErr := runUsage(e.deps.Store, e.deps.Settings.StagingRoot(), run.RunID, run.ExchangeDir)
		if usageErr != nil {
			status, reason = RunStatusFailed, usageErr.Error()
		} else if limit := effectiveQuota(e.deps.Settings.PerRunQuota(), defaultPerRunQuota); usage > limit {
			status = RunStatusFailed
			reason = fmt.Sprintf("per-run quota exceeded: %d bytes used, limit %d", usage, limit)
		}
	}
	if record, _, readErr := e.deps.Store.Run(run.RunID); readErr == nil && record.CancelRequested {
		status, reason = RunStatusCancelled, record.Error
	} else if readErr != nil && status == "" {
		status, reason = RunStatusFailed, fmt.Sprintf("read cancellation latch: %v", readErr)
	}

	finish := RunFinish{OutputTail: tail.String(), OutputUnverified: outputUnverified, FinishedAt: time.Now().UTC()}
	if status != "" {
		finish.Status, finish.Error = status, reason
	} else {
		finish.ExitCode = exitCode(cmd)
		if waitErr == nil && finish.ExitCode != nil && *finish.ExitCode == 0 {
			finish.Status = RunStatusSucceeded
		} else {
			finish.Status = RunStatusFailed
			finish.Error = commandExitError(waitErr, finish.ExitCode)
		}
	}
	if forcedCleanup && finish.Error == "" {
		finish.Error = "command cleanup exceeded its deadline"
	}
	return e.finish(run, finish)
}

func (e *commandExecutor) stopBeforeStart(ctx context.Context, run QueuedRun) (Outcome, bool) {
	record, _, err := e.deps.Store.Run(run.RunID)
	if err != nil {
		return e.finish(run, RunFinish{
			Status: RunStatusFailed, Error: "read command cancellation latch: " + err.Error(), FinishedAt: time.Now().UTC(),
		}), true
	}
	if record.CancelRequested {
		return e.finish(run, RunFinish{
			Status: RunStatusCancelled, Error: record.Error, FinishedAt: time.Now().UTC(),
		}), true
	}
	if ctx.Err() == nil {
		return Outcome{}, false
	}
	status, reason := e.contextTermination(ctx, run.RunID)
	return e.finish(run, RunFinish{Status: status, Error: reason, FinishedAt: time.Now().UTC()}), true
}

func (e *commandExecutor) contextTermination(ctx context.Context, runID string) (string, string) {
	if record, _, err := e.deps.Store.Run(runID); err == nil && record.CancelRequested {
		return RunStatusCancelled, record.Error
	}
	if errors.Is(context.Cause(ctx), errDispatcherShutdown) {
		return RunStatusInterrupted, "server interrupted"
	}
	return RunStatusCancelled, e.cancelReason(runID)
}

func (e *commandExecutor) finishWithoutStart(run QueuedRun, fallback string) Outcome {
	record, _, err := e.deps.Store.Run(run.RunID)
	if err != nil {
		return Outcome{Status: RunStatusFailed, Error: fallback + ": " + err.Error()}
	}
	if RunStatusTerminal(record.Status) {
		return durableOutcome(record.Status, record.Error)
	}
	status, reason := RunStatusInterrupted, fallback
	if record.CancelRequested {
		status, reason = RunStatusCancelled, record.Error
	}
	return e.finish(run, RunFinish{Status: status, Error: reason, FinishedAt: time.Now().UTC()})
}

func (e *commandExecutor) finish(run QueuedRun, finish RunFinish) Outcome {
	won, err := e.deps.Store.FinishRun(run.RunID, finish)
	if err != nil {
		return e.outcomeAfterPersistenceError(run.RunID, finish.Error, err)
	}
	if won {
		return durableOutcome(finish.Status, finish.Error)
	}
	record, _, err := e.deps.Store.Run(run.RunID)
	if err != nil {
		return Outcome{Status: RunStatusFailed, Error: finish.Error + "; read terminal command: " + err.Error()}
	}
	if !RunStatusTerminal(record.Status) && record.CancelRequested && finish.Status != RunStatusCancelled {
		cancelled := finish
		cancelled.Status = RunStatusCancelled
		cancelled.Error = record.Error
		cancelled.ExitCode = nil
		won, err = e.deps.Store.FinishRun(run.RunID, cancelled)
		if err != nil {
			return e.outcomeAfterPersistenceError(run.RunID, cancelled.Error, err)
		}
		if won {
			return durableOutcome(RunStatusCancelled, cancelled.Error)
		}
		record, _, err = e.deps.Store.Run(run.RunID)
		if err != nil {
			return Outcome{Status: RunStatusFailed, Error: cancelled.Error + "; read terminal command: " + err.Error()}
		}
	}
	return durableOutcome(record.Status, record.Error)
}

// outcomeAfterPersistenceError never publishes the intended terminal status as
// durable authority. If the store can still be read, the live cockpit mirrors
// the status that actually persisted (normally running); if it cannot, the
// managed job falls back to its generic failure rather than inventing authority.
func (e *commandExecutor) outcomeAfterPersistenceError(runID, reason string, persistErr error) Outcome {
	message := "persist terminal command: " + persistErr.Error()
	if reason != "" {
		message = reason + "; " + message
	}
	record, _, readErr := e.deps.Store.Run(runID)
	if readErr != nil {
		return Outcome{Status: RunStatusFailed, Error: message + "; read durable command status: " + readErr.Error()}
	}
	return durableOutcome(record.Status, message)
}

func durableOutcome(status, message string) Outcome {
	return Outcome{Status: status, AuthoritativeStatus: status, Error: message}
}

func (e *commandExecutor) cancelReason(runID string) string {
	record, _, err := e.deps.Store.Run(runID)
	if err == nil && record.CancelRequested && record.Error != "" {
		return record.Error
	}
	return "command cancelled"
}

func normalizeCommandPath(commandPath string) (string, error) {
	if commandPath == "" {
		return "", fmt.Errorf("plugin command path is empty")
	}
	entries := filepath.SplitList(commandPath)
	cleaned := make([]string, 0, len(entries))
	for _, directory := range entries {
		if directory == "" || !filepath.IsAbs(directory) {
			return "", fmt.Errorf("plugin command path contains invalid directory %q", directory)
		}
		cleaned = append(cleaned, filepath.Clean(directory))
	}
	return strings.Join(cleaned, string(os.PathListSeparator)), nil
}

func resolveExecutable(base, commandPath string) (string, error) {
	if err := validateExecutable("run", base); err != nil {
		return "", err
	}
	cleanedPath, err := normalizeCommandPath(commandPath)
	if err != nil {
		return "", err
	}
	for _, directory := range filepath.SplitList(cleanedPath) {
		candidate := filepath.Join(directory, base)
		info, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("inspect executable %q: %w", candidate, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("executable %q was not found in the configured plugin command path", base)
}

func exitCode(cmd *exec.Cmd) *int {
	if cmd.ProcessState == nil {
		return nil
	}
	code := cmd.ProcessState.ExitCode()
	if code < 0 {
		return nil
	}
	return &code
}

func commandExitError(waitErr error, code *int) string {
	if code != nil {
		return fmt.Sprintf("command exited with status %d", *code)
	}
	if waitErr != nil {
		return fmt.Sprintf("wait for command: %v", waitErr)
	}
	return "command exited without a status"
}
