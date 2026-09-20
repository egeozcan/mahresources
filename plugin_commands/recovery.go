package plugin_commands

import (
	"context"
	"fmt"
	"time"
)

// RecoveryBlocker describes one running command whose process group recovery
// could not be settled without risking an unrelated or still-live process.
type RecoveryBlocker struct {
	RunID          string
	ProcessGroupID int
	Reason         string
}

// RecoveryBlockedError reports every independently blocked running command
// found by one recovery scan. It is distinct from context, store, filesystem,
// and malformed-state failures, which remain ordinary recovery errors.
type RecoveryBlockedError struct {
	Blockers []RecoveryBlocker
}

func (e *RecoveryBlockedError) Error() string {
	if e == nil || len(e.Blockers) == 0 {
		return "plugin command recovery is blocked"
	}
	message := fmt.Sprintf("plugin command recovery is blocked by %d process group(s)", len(e.Blockers))
	for _, blocker := range e.Blockers {
		message += fmt.Sprintf("; run %s pgid %d: %s", blocker.RunID, blocker.ProcessGroupID, blocker.Reason)
	}
	return message
}

// Recover classifies command rows left nonterminal by a previous process. It
// never trusts a persisted process-group id by itself: a group is signalled only
// after at least one current member proves the run id through its environment.
func (d *Dispatcher) Recover(ctx context.Context) error {
	if d == nil || d.deps.Store == nil || d.deps.Inspector == nil {
		return fmt.Errorf("plugin_commands: recovery dependencies are incomplete")
	}
	imports, err := d.deps.Store.NonterminalImports()
	if err != nil {
		return fmt.Errorf("list nonterminal plugin command imports: %w", err)
	}
	if err := d.deps.Store.InterruptNonterminalImports(time.Now().UTC()); err != nil {
		return fmt.Errorf("interrupt plugin command imports: %w", err)
	}
	for _, record := range imports {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !validExchangeComponent(record.ID) {
			return fmt.Errorf("invalid plugin command import id %q during recovery", record.ID)
		}
	}
	// No import worker exists yet while startup recovery runs. Every directory
	// under import_tmp is therefore orphaned, including the narrow crash window
	// after durable success but before the worker's deferred cleanup.
	if d.deps.Settings != nil {
		if err := cleanupImportTemps(d.deps.Settings.StagingRoot()); err != nil {
			return err
		}
	} else if len(imports) != 0 {
		return fmt.Errorf("plugin_commands: import recovery settings are unavailable")
	}

	runs, err := d.deps.Store.NonterminalRuns()
	if err != nil {
		return err
	}
	var blocked []RecoveryBlocker
	for _, run := range runs {
		if err := ctx.Err(); err != nil {
			return err
		}
		finish := RunFinish{FinishedAt: time.Now().UTC()}
		switch run.Status {
		case RunStatusQueued:
			if run.CancelRequested {
				finish.Status, finish.Error = RunStatusCancelled, run.Error
			} else {
				finish.Status, finish.Error = RunStatusInterrupted, "server interrupted before command start"
			}
		case RunStatusRunning:
			var blocker *RecoveryBlocker
			finish, blocker, err = d.recoverRunning(ctx, run)
			if err != nil {
				return fmt.Errorf("recover plugin command %s: %w", run.ID, err)
			}
			if blocker != nil {
				blocked = append(blocked, *blocker)
				continue
			}
		default:
			return fmt.Errorf("plugin command run %q has unknown status %q", run.ID, run.Status)
		}
		won, err := d.deps.Store.FinishRun(run.ID, finish)
		if err != nil {
			return fmt.Errorf("recover plugin command %s: %w", run.ID, err)
		}
		if !won {
			d.deps.Logf("plugin command %s changed state during recovery", run.ID)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(blocked) != 0 {
		return &RecoveryBlockedError{Blockers: blocked}
	}
	return nil
}

func (d *Dispatcher) recoverRunning(ctx context.Context, run RecoveryRun) (RunFinish, *RecoveryBlocker, error) {
	finish := RunFinish{
		Status: RunStatusInterrupted, Error: "server interrupted while command was running",
		FinishedAt: time.Now().UTC(),
	}
	if run.ProcessGroupID == nil {
		// No numeric group survived the fork/persist crash window, so recovery has
		// nothing it can inspect. Preserve the historical terminal fail-closed
		// classification and refuse its output through OutputUnverified.
		finish.OutputUnverified = true
		finish.Error += "; process group was not recorded"
		return finish, nil, nil
	}
	if *run.ProcessGroupID <= 0 {
		return RunFinish{}, nil, fmt.Errorf("persisted non-positive process group %d", *run.ProcessGroupID)
	}

	pgid := *run.ProcessGroupID
	if d.deps.BootSessionID != "" && run.BootSessionID != "" && d.deps.BootSessionID != run.BootSessionID {
		finish.OutputUnverified = true
		finish.Error += "; process group belongs to a prior boot"
		if run.CancelRequested {
			finish.Status = RunStatusCancelled
			finish.Error = "process group belongs to a prior boot"
			if run.Error != "" {
				finish.Error = run.Error + "; " + finish.Error
			}
		}
		return finish, nil, nil
	}

	if err := ctx.Err(); err != nil {
		return RunFinish{}, nil, err
	}
	identity, err := d.deps.Inspector.InspectGroup(pgid, run.ID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return RunFinish{}, nil, ctxErr
		}
		return RunFinish{}, recoveryBlocker(run.ID, pgid, fmt.Sprintf("verify process group ownership: %v", err)), nil
	}
	switch identity.State {
	case GroupDead:
		if run.CancelRequested {
			finish.Status, finish.Error = RunStatusCancelled, run.Error
		}
		return finish, nil, nil
	case GroupAliveUnverified:
		return RunFinish{}, recoveryBlocker(run.ID, pgid, fmt.Sprintf("process group %d ownership could not be verified while it remains alive", pgid)), nil
	case GroupAliveOwned:
		reason, err := d.terminateRecoveredGroup(ctx, pgid, run.ID)
		if err != nil {
			return RunFinish{}, nil, err
		}
		if reason != "" {
			return RunFinish{}, recoveryBlocker(run.ID, pgid, reason), nil
		}
		if run.CancelRequested {
			finish.Status, finish.Error = RunStatusCancelled, run.Error
		}
		return finish, nil, nil
	default:
		return RunFinish{}, nil, fmt.Errorf("process group %d ownership returned an unknown state", pgid)
	}
}

func recoveryBlocker(runID string, pgid int, reason string) *RecoveryBlocker {
	return &RecoveryBlocker{RunID: runID, ProcessGroupID: pgid, Reason: reason}
}

func (d *Dispatcher) terminateRecoveredGroup(ctx context.Context, pgid int, runID string) (string, error) {
	// Recheck immediately before signalling. The pid namespace may have changed
	// between the recovery scan and this point.
	if err := ctx.Err(); err != nil {
		return "", err
	}
	identity, err := d.deps.Inspector.InspectGroup(pgid, runID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return fmt.Sprintf("recheck process group ownership: %v", err), nil
	}
	switch identity.State {
	case GroupDead:
		return "", nil
	case GroupAliveOwned:
	case GroupAliveUnverified:
		return "process group ownership changed before termination", nil
	default:
		return "", fmt.Errorf("process group %d ownership returned an unknown state before termination", pgid)
	}

	if d.claimRecoverySignal(runID) {
		if err := d.deps.Inspector.KillGroup(pgid); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", ctxErr
			}
			return fmt.Sprintf("terminate owned process group: %v", err), nil
		}
	}

	pollInterval := d.recoveryPollInterval
	if pollInterval <= 0 {
		pollInterval = groupPollInterval
	}
	drainTimeout := d.recoveryDrainTimeout
	if drainTimeout <= 0 {
		drainTimeout = groupDrainTimeout
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(drainTimeout)
	defer deadline.Stop()
	ownershipLossLogged := false
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		identity, err := d.deps.Inspector.InspectGroup(pgid, runID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", ctxErr
			}
			return fmt.Sprintf("verify process group termination: %v", err), nil
		}
		switch identity.State {
		case GroupDead:
			return "", nil
		case GroupAliveOwned:
		case GroupAliveUnverified:
			// Identity was rechecked immediately before this process's only signal
			// attempt. Environment visibility may disappear while members exit, so
			// the loss changes no signalling decision.
			if !ownershipLossLogged {
				d.deps.Logf("plugin command process group %d ownership became unverifiable after termination; waiting for death", pgid)
				ownershipLossLogged = true
			}
		default:
			return "", fmt.Errorf("process group %d ownership returned an unknown state after termination", pgid)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline.C:
			return "owned process group did not exit before recovery deadline", nil
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) claimRecoverySignal(runID string) bool {
	d.recoveryMu.Lock()
	defer d.recoveryMu.Unlock()
	if d.recoverySignals == nil {
		d.recoverySignals = make(map[string]struct{})
	}
	if _, claimed := d.recoverySignals[runID]; claimed {
		return false
	}
	d.recoverySignals[runID] = struct{}{}
	return true
}
