package application_context

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"mahresources/plugin_commands"
)

func TestPluginCommandHistoryCancelRefusesQuarantineWithoutDurableMutation(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	runID := "queued-during-quarantine"
	require.NoError(t, ctx.CreateRun(
		plugin_commands.RunRecord{
			ID: runID, PluginName: "plug", CommandName: "download", ParamsJSON: `{}`,
			Status: plugin_commands.RunStatusQueued, CreatedAt: time.Now().UTC(),
		},
		plugin_commands.RunOutput{RunID: runID, ArgvJSON: `[]`, CreatedAt: time.Now().UTC()},
	))

	err := ctx.CancelPluginCommandRun(runID)
	require.ErrorIs(t, err, plugin_commands.ErrCommandRuntimeQuarantined)
	record, _, readErr := ctx.Run(runID)
	require.NoError(t, readErr)
	require.False(t, record.CancelRequested)

	available, reason := ctx.PluginCommandRuntimeAvailability()
	require.False(t, available)
	require.Contains(t, reason, "automatic recovery")
	require.Contains(t, reason, "/logs")
}

func TestPluginCommandHistoryCancelMissingRunIsNotFoundOnlyWithActiveDispatcher(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	dispatcher := plugin_commands.NewDispatcher(plugin_commands.Dependencies{Store: ctx})
	installPluginCommandActiveForTest(ctx, dispatcher, nil, nil)

	err := ctx.CancelPluginCommandRun("missing")
	require.ErrorIs(t, err, plugin_commands.ErrRunNotFound)
	available, reason := ctx.PluginCommandRuntimeAvailability()
	require.True(t, available)
	require.Empty(t, reason)
}

func TestPluginCommandHistoryIsBoundedNewestFirstAndOutputIsDetailOnly(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("history-%d", i)
		finished := base.Add(time.Duration(i) * time.Minute)
		if err := ctx.CreateRun(plugin_commands.RunRecord{ID: id, PluginName: "plug", CommandName: "download", ParamsJSON: `{}`, Status: plugin_commands.RunStatusQueued, CreatedAt: finished}, plugin_commands.RunOutput{RunID: id, ArgvJSON: `[]`, CreatedAt: base}); err != nil {
			t.Fatal(err)
		}
		if won, err := ctx.MarkRunRunning(id, finished); err != nil || !won {
			t.Fatalf("mark running: won=%v err=%v", won, err)
		}
		if won, err := ctx.FinishRun(id, plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded, OutputTail: "secret-tail", FinishedAt: finished}); err != nil || !won {
			t.Fatalf("finish: won=%v err=%v", won, err)
		}
	}
	runs, count, err := ctx.GetPluginCommandRuns(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 || len(runs) != 2 || runs[0].ID != "history-2" || runs[1].ID != "history-1" {
		t.Fatalf("count/runs = %d %+v", count, runs)
	}
	view, available, err := ctx.GetPluginCommandRun("history-2")
	if err != nil {
		t.Fatal(err)
	}
	if !available || view.Output.OutputTail != "secret-tail" {
		t.Fatalf("detail = available %v %+v", available, view)
	}
	if _, err := ctx.PruneRunOutputs(time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, available, err = ctx.GetPluginCommandRun("history-2")
	if err != nil {
		t.Fatal(err)
	}
	if available {
		t.Fatal("pruned output still reported available")
	}
}
