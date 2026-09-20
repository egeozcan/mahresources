package plugin_commands

import (
	"context"
	"errors"
	"fmt"
	"time"
)

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
			finish, err = d.recoverRunning(ctx, run)
			if err != nil {
				return fmt.Errorf("recover plugin command %s: %w", run.ID, err)
			}
		default:
			continue
		}
		won, err := d.deps.Store.FinishRun(run.ID, finish)
		if err != nil {
			return fmt.Errorf("recover plugin command %s: %w", run.ID, err)
		}
		if !won {
			d.deps.Logf("plugin command %s changed state during recovery", run.ID)
		}
	}
	return nil
}

func (d *Dispatcher) recoverRunning(ctx context.Context, run RunRecord) (RunFinish, error) {
	finish := RunFinish{
		Status: RunStatusInterrupted, Error: "server interrupted while command was running",
		FinishedAt: time.Now().UTC(),
	}
	if run.ProcessGroupID == nil || *run.ProcessGroupID <= 0 {
		// No numeric group survived the fork/persist crash window, so recovery has
		// nothing it can inspect. Preserve the historical terminal fail-closed
		// classification and refuse its output through OutputUnverified.
		finish.OutputUnverified = true
		finish.Error += "; process group was not recorded"
		return finish, nil
	}

	pgid := *run.ProcessGroupID
	identity, err := d.deps.Inspector.InspectGroup(pgid, run.ID)
	if err != nil {
		return RunFinish{}, fmt.Errorf("verify process group ownership: %w", err)
	}
	switch identity.State {
	case GroupDead:
		if run.CancelRequested {
			finish.Status, finish.Error = RunStatusCancelled, run.Error
		}
		return finish, nil
	case GroupAliveUnverified:
		return RunFinish{}, fmt.Errorf("process group %d ownership could not be verified while it remains alive", pgid)
	case GroupAliveOwned:
		if err := d.terminateRecoveredGroup(ctx, pgid, run.ID); err != nil {
			return RunFinish{}, err
		}
		if run.CancelRequested {
			finish.Status, finish.Error = RunStatusCancelled, run.Error
		}
		return finish, nil
	default:
		return RunFinish{}, fmt.Errorf("process group %d ownership returned an unknown state", pgid)
	}
}

func (d *Dispatcher) terminateRecoveredGroup(ctx context.Context, pgid int, runID string) error {
	// Recheck immediately before signalling. The pid namespace may have changed
	// between the recovery scan and this point.
	identity, err := d.deps.Inspector.InspectGroup(pgid, runID)
	if err != nil {
		return fmt.Errorf("recheck process group ownership: %w", err)
	}
	if identity.State == GroupDead {
		return nil
	}
	if identity.State != GroupAliveOwned {
		return errors.New("process group ownership changed before termination")
	}
	if err := d.deps.Inspector.KillGroup(pgid); err != nil {
		return fmt.Errorf("terminate owned process group: %w", err)
	}

	ticker := time.NewTicker(groupPollInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(groupDrainTimeout)
	defer deadline.Stop()
	ownershipLossLogged := false
	for {
		identity, err := d.deps.Inspector.InspectGroup(pgid, runID)
		if err != nil {
			return fmt.Errorf("verify process group termination: %w", err)
		}
		if identity.State == GroupDead {
			return nil
		}
		// Identity was rechecked immediately before the one signal this function
		// sends. Environment visibility may disappear while members are exiting
		// (notably for Apple platform binaries), so loss of the marker after that
		// point changes no signaling decision. Keep observing without signalling
		// again and publish only once the group is actually dead.
		if identity.State != GroupAliveOwned && !ownershipLossLogged {
			d.deps.Logf("plugin command process group %d ownership became unverifiable after termination; waiting for death", pgid)
			ownershipLossLogged = true
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("owned process group did not exit before recovery deadline")
		case <-ticker.C:
		}
	}
}
