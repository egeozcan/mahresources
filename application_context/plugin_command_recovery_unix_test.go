//go:build darwin || linux

package application_context

import (
	"context"
	"strings"
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
	if err := ctx.SetRunProcessGroup("live-unverified-recovery", pgid, "test-boot-session"); err != nil {
		t.Fatal(err)
	}

	err := ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{
		root: root, commandPath: t.TempDir(),
	})
	if err == nil {
		_ = ctx.StopPluginCommands()
		t.Fatal("startup published a command runtime for a live unverified recovery group")
	}
	if !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("startup error = %v", err)
	}
	run, _, readErr := ctx.Run("live-unverified-recovery")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if run.Status != plugin_commands.RunStatusRunning || run.FinishedAt != nil {
		t.Fatalf("recovery settled live unverified run: %+v", run)
	}
	if ctx.pluginCommandDispatcher != nil || ctx.pluginCommandExchange != nil {
		t.Fatal("command host was published after recovery refusal")
	}
	lease, leaseErr := plugin_commands.AcquireRuntimeLease(root)
	if leaseErr != nil {
		t.Fatalf("failed startup retained runtime lease: %v", leaseErr)
	}
	_ = lease.Close()
}
