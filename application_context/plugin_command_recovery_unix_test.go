//go:build darwin || linux

package application_context

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"mahresources/plugin_commands"
)

func TestPluginCommandLifecycleDoesNotPublishHostForLiveUnverifiedRecoveryGroup(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	root := t.TempDir()
	owner := uint(7)
	now := time.Now().UTC()
	pgid := syscall.Getpgrp()
	if err := ctx.CreateRun(plugin_commands.RunRecord{
		ID: "live-unverified-recovery", PluginName: "lifecycle", CommandName: "command",
		ParamsJSON: `{}`, Status: plugin_commands.RunStatusQueued,
		CreatedByUserID: &owner, CreatedAt: now,
	}, plugin_commands.RunOutput{RunID: "live-unverified-recovery", ArgvJSON: `[]`, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if won, err := ctx.MarkRunRunning("live-unverified-recovery", now); err != nil || !won {
		t.Fatalf("mark running: won=%v err=%v", won, err)
	}
	bootSessionID, err := plugin_commands.CurrentBootSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetRunProcessGroup("live-unverified-recovery", pgid, bootSessionID); err != nil {
		t.Fatal(err)
	}

	if err := ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{
		root: root, commandPath: t.TempDir(),
	}); err != nil {
		t.Fatalf("recovery quarantine failed process startup: %v", err)
	}
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	_, activeErr := ctx.pluginCommandActive()
	if !errors.Is(activeErr, plugin_commands.ErrCommandRuntimeQuarantined) {
		t.Fatalf("runtime availability = %v, want quarantine", activeErr)
	}
	run, _, readErr := ctx.Run("live-unverified-recovery")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if run.Status != plugin_commands.RunStatusRunning || run.FinishedAt != nil {
		t.Fatalf("recovery settled live unverified run: %+v", run)
	}
	if active, _ := ctx.pluginCommandActive(); active != nil {
		t.Fatal("command host was published after recovery refusal")
	}
	lease, leaseErr := plugin_commands.AcquireRuntimeLease(root)
	if leaseErr == nil || !errors.Is(leaseErr, plugin_commands.ErrRuntimeLeaseBusy) {
		if lease != nil {
			_ = lease.Close()
		}
		t.Fatalf("recovery quarantine did not retain runtime lease: %v", leaseErr)
	}
}
