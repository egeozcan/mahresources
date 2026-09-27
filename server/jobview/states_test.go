package jobview

import (
	"slices"
	"testing"

	"mahresources/jobs"
)

// TestTheStateTableNamesEveryStateAsTheLifecycleDefinesIt keeps job_states.json
// the lifecycle's own: one entry per state, finished exactly where the state is
// terminal, and the groups the glossary gives (CONTEXT.md: an Active Job is
// scheduled, queued, running or paused; a blocked Job Needs Attention, as does a
// failed or interrupted one).
func TestTheStateTableNamesEveryStateAsTheLifecycleDefinesIt(t *testing.T) {
	if len(statePresentations.States) != len(jobs.AllStates) {
		t.Fatalf("the table names %d states, the lifecycle has %d", len(statePresentations.States), len(jobs.AllStates))
	}
	tones := []string{"working", "waiting", "paused", "warning", "done", "failed", "neutral"}
	for _, state := range jobs.AllStates {
		entry, ok := statePresentations.States[string(state)]
		if !ok {
			t.Fatalf("the table has no entry for %s", state)
		}
		if entry.Label == "" || !slices.Contains(tones, entry.Tone) {
			t.Fatalf("%s has label %q and tone %q", state, entry.Label, entry.Tone)
		}
		if entry.Terminal != state.Terminal() {
			t.Fatalf("%s is terminal=%v in the table and %v in the lifecycle", state, entry.Terminal, state.Terminal())
		}
		if entry.Working != (state == jobs.StateRunning) {
			t.Fatalf("%s is working=%v; only running work is being done", state, entry.Working)
		}
	}
	if got, want := StatesInGroup("active"), []string{"scheduled", "queued", "running", "paused"}; !slices.Equal(got, want) {
		t.Fatalf("active = %v, want %v", got, want)
	}
	if got, want := StatesInGroup("attention"), []string{"blocked", "failed", "interrupted"}; !slices.Equal(got, want) {
		t.Fatalf("attention = %v, want %v", got, want)
	}
	if got, want := StatesInGroup("finished"), []string{"succeeded", "cancelled"}; !slices.Equal(got, want) {
		t.Fatalf("finished = %v, want %v", got, want)
	}
	if got, want := TerminalStates(), []string{"succeeded", "failed", "cancelled", "interrupted"}; !slices.Equal(got, want) {
		t.Fatalf("terminal = %v, want %v", got, want)
	}
	if !slices.Contains(tones, statePresentations.Partial.Tone) || statePresentations.Partial.Label == "" {
		t.Fatalf("the partial entry is %+v", statePresentations.Partial)
	}
}

func TestPresentStateReadsAPartialSuccessAsPartial(t *testing.T) {
	if got := PresentState(jobs.StateSucceeded, jobs.PhasePartial); got.Label != "Partially completed" || got.Tone != "warning" || !got.Terminal {
		t.Fatalf("a partial success is presented as %+v", got)
	}
	if got := PresentState(jobs.StateSucceeded, "downloading"); got.Label != "Succeeded" || got.Tone != "done" {
		t.Fatalf("a success is presented as %+v", got)
	}
	if got := PresentState(jobs.StatePaused, jobs.PhasePartial); got.Label != "Paused" {
		t.Fatalf("the partial phase changed a paused Job's label to %q", got.Label)
	}
	if got := PresentState("bogus", ""); got.Label != "Unknown" || got.Group != "other" {
		t.Fatalf("an unknown state is presented as %+v", got)
	}
}
