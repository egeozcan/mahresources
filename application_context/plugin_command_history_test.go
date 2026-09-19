package application_context

import (
	"fmt"
	"testing"
	"time"

	"mahresources/plugin_commands"
)

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
