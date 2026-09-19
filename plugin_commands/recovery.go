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
			finish = d.recoverRunning(ctx, run)
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

func (d *Dispatcher) recoverRunning(ctx context.Context, run RunRecord) RunFinish {
	finish := RunFinish{
		Status: RunStatusInterrupted, Error: "server interrupted while command was running",
		FinishedAt: time.Now().UTC(),
	}
	if run.ProcessGroupID == nil || *run.ProcessGroupID <= 0 {
		finish.OutputUnverified = true
		finish.Error += "; process group was not recorded"
		return finish
	}

	pgid := *run.ProcessGroupID
	identity, err := d.deps.Inspector.InspectGroup(pgid, run.ID)
	if err != nil {
		finish.OutputUnverified = true
		finish.Error += "; process group ownership could not be verified: " + err.Error()
		return finish
	}
	switch identity.State {
	case GroupDead:
		if run.CancelRequested {
			finish.Status, finish.Error = RunStatusCancelled, run.Error
		}
		return finish
	case GroupAliveUnverified:
		finish.OutputUnverified = true
		finish.Error += "; process group ownership could not be verified"
		return finish
	case GroupAliveOwned:
		if err := d.terminateRecoveredGroup(ctx, pgid, run.ID); err != nil {
			finish.OutputUnverified = true
			finish.Error += "; " + err.Error()
			return finish
		}
		if run.CancelRequested {
			finish.Status, finish.Error = RunStatusCancelled, run.Error
		}
		return finish
	default:
		finish.OutputUnverified = true
		finish.Error += "; process group ownership returned an unknown state"
		return finish
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
	for {
		identity, err := d.deps.Inspector.InspectGroup(pgid, runID)
		if err != nil {
			return fmt.Errorf("verify process group termination: %w", err)
		}
		if identity.State == GroupDead {
			return nil
		}
		if identity.State != GroupAliveOwned {
			d.deps.Logf("plugin command process group %d ownership became unverifiable after termination", pgid)
			return fmt.Errorf("process group %d ownership became unverifiable after termination", pgid)
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
