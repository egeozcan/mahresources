package plugin_commands

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestACommandRefusedByAFullJobBudgetWaitsInItsQueue pins the difference between
// a wait and a failure. A full deployment budget writes nothing and starts
// nothing, so the run stays queued — durably and in its plugin's queue — with no
// callback delivered, and it is handed to the live lane once the budget frees.
// Recording it as a failed dispatch would end, for good, a run that never started
// and could have run a moment later.
func TestACommandRefusedByAFullJobBudgetWaitsInItsQueue(t *testing.T) {
	d, store, jobs := startTestDispatcher(t, 10)
	jobs.mu.Lock()
	jobs.commandErr = fmt.Errorf("%w: global allows 1 concurrent executions", ErrJobCapacityFull)
	jobs.mu.Unlock()

	completed := make(chan Result, 1)
	runID, err := d.Submit(commandRequest("capacity", func(result Result) { completed <- result }))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * jobCapacityRetryDelay)

	select {
	case result := <-completed:
		t.Fatalf("a run refused by a full budget delivered its completion: %+v", result)
	default:
	}
	store.mu.Lock()
	finish, finished := store.finishes[runID]
	status := store.runs[runID].Status
	store.mu.Unlock()
	if finished || status != RunStatusQueued {
		t.Fatalf("a run refused by a full budget is %q (finished=%v %+v), want queued", status, finished, finish)
	}
	if jobs.commandCount() != 0 {
		t.Fatalf("a refused run was registered as a live job %d times", jobs.commandCount())
	}

	jobs.mu.Lock()
	jobs.commandErr = nil
	jobs.mu.Unlock()
	waitFor(t, func() bool { return jobs.commandCount() == 1 })
	if got := jobs.commandSnapshot()[0].spec.RunID; got != runID {
		t.Fatalf("the live lane was handed %q, want the waiting run %q", got, runID)
	}
}

// TestAnImportRefusedByAFullJobBudgetWaitsInItsQueue is the same wait for an
// import: its lease and its place are kept, and nothing is recorded as failed.
func TestAnImportRefusedByAFullJobBudgetWaitsInItsQueue(t *testing.T) {
	d, store, jobs := startTestDispatcher(t, 10)
	jobs.mu.Lock()
	jobs.importErr = fmt.Errorf("%w: global allows 1 concurrent executions", ErrJobCapacityFull)
	jobs.mu.Unlock()

	if err := d.submitImport(ImportJobSpec{ImportID: "import-waits", RunID: "run-import-waits", PluginName: "p"},
		func(context.Context, Progress) Outcome { return Outcome{Status: ImportStatusSucceeded} }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * jobCapacityRetryDelay)

	store.mu.Lock()
	finish, finished := store.importFinishes["import-waits"]
	store.mu.Unlock()
	if finished {
		t.Fatalf("an import refused by a full budget was finished: %+v", finish)
	}
	if jobs.importCount() != 0 {
		t.Fatalf("a refused import was registered as a live job %d times", jobs.importCount())
	}

	jobs.mu.Lock()
	jobs.importErr = nil
	jobs.mu.Unlock()
	waitFor(t, func() bool { return jobs.importCount() == 1 })
	if got := jobs.importSnapshot()[0].spec.ImportID; got != "import-waits" {
		t.Fatalf("the live lane was handed %q, want the waiting import", got)
	}
}

// TestRunsAndImportsTakeTurnsForTheJobBudget pins fairness between the two kinds
// of work one budget admits. With one slot freeing at a time and runs queued
// ahead of an import, the slot after a run goes to the import: runs that keep
// arriving cannot take every slot while the import waits.
func TestRunsAndImportsTakeTurnsForTheJobBudget(t *testing.T) {
	d, _, jobs := startTestDispatcher(t, 10)
	none := 0
	jobs.mu.Lock()
	jobs.slots = &none
	jobs.mu.Unlock()
	free := func() {
		jobs.mu.Lock()
		*jobs.slots = 1
		jobs.mu.Unlock()
	}

	for i := 0; i < 3; i++ {
		if _, err := d.Submit(commandRequest(fmt.Sprintf("runs-%d", i), nil)); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.submitImport(ImportJobSpec{ImportID: "import-turn", RunID: "run-import-turn", PluginName: "imports"},
		func(context.Context, Progress) Outcome { return Outcome{Status: ImportStatusSucceeded} }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * jobCapacityRetryDelay)

	free()
	waitFor(t, func() bool { return jobs.commandCount()+jobs.importCount() == 1 })
	free()
	waitFor(t, func() bool { return jobs.commandCount()+jobs.importCount() == 2 })
	if jobs.importCount() != 1 {
		t.Fatalf("with runs still queued the second slot went to a run (%d runs, %d imports started), want the waiting import",
			jobs.commandCount(), jobs.importCount())
	}
}
