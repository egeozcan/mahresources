package jobview

import (
	"os"
	"slices"
	"strings"
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
		if entry.Label == "" || !slices.Contains(tones, entry.Tone) || entry.Since == "" {
			t.Fatalf("%s has label %q, tone %q and since %q", state, entry.Label, entry.Tone, entry.Since)
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
	if !slices.Contains(tones, statePresentations.Partial.Tone) || statePresentations.Partial.Label == "" || statePresentations.Partial.Since == "" {
		t.Fatalf("the partial entry is %+v", statePresentations.Partial)
	}
	if len(statePresentations.RunningIntents) != 2 {
		t.Fatalf("the table names %d running intents, want pause and cancel", len(statePresentations.RunningIntents))
	}
	for _, intent := range []string{jobs.ControlIntentPause, jobs.ControlIntentCancel} {
		entry, ok := statePresentations.RunningIntents[intent]
		if !ok || entry.Label == "" || !slices.Contains(tones, entry.Tone) {
			t.Fatalf("the %q intent is %+v", intent, entry)
		}
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

// A running Job with a pause or cancellation requested and not yet confirmed says
// what it is doing: it is pausing or cancelling, not simply running, and never
// paused or cancelled before its executor confirms it.
func TestPresentJobSaysWhatARequestedControlIsDoing(t *testing.T) {
	cases := []struct {
		snapshot jobs.Snapshot
		label    string
		tone     string
	}{
		{jobs.Snapshot{State: jobs.StateRunning, ControlIntent: jobs.ControlIntentPause}, "Pausing", "paused"},
		{jobs.Snapshot{State: jobs.StateRunning, ControlIntent: jobs.ControlIntentCancel}, "Cancelling", "working"},
		{jobs.Snapshot{State: jobs.StateRunning}, "Running", "working"},
		{jobs.Snapshot{State: jobs.StatePaused}, "Paused", "paused"},
		// A request outside running work is not what the row is doing.
		{jobs.Snapshot{State: jobs.StateBlocked, ControlIntent: jobs.ControlIntentCancel}, "Blocked", "warning"},
		{jobs.Snapshot{State: jobs.StateSucceeded, Phase: jobs.PhasePartial}, "Partially completed", "warning"},
	}
	for _, tc := range cases {
		got := PresentJob(tc.snapshot)
		if got.Label != tc.label || got.Tone != tc.tone {
			t.Fatalf("%s with intent %q is presented as %q/%q, want %q/%q",
				tc.snapshot.State, tc.snapshot.ControlIntent, got.Label, got.Tone, tc.label, tc.tone)
		}
		if got.Working != (tc.snapshot.State == jobs.StateRunning) {
			t.Fatalf("%s with intent %q is working=%v", tc.snapshot.State, tc.snapshot.ControlIntent, got.Working)
		}
	}
}

// TestEveryToneHasItsColour: a surface draws a state's tone with the class the
// tone names (job-tone--<tone> in public/index.css), so a tone the table uses
// with no rule there would draw a pill with no colour at all.
func TestEveryToneHasItsColour(t *testing.T) {
	css, err := os.ReadFile("../../public/index.css")
	if err != nil {
		t.Fatal(err)
	}
	tones := []string{statePresentations.Partial.Tone, statePresentations.Unknown.Tone}
	for _, entry := range statePresentations.States {
		tones = append(tones, entry.Tone)
	}
	for _, entry := range statePresentations.RunningIntents {
		tones = append(tones, entry.Tone)
	}
	for _, tone := range tones {
		if !strings.Contains(string(css), ".job-tone--"+tone+" ") && !strings.Contains(string(css), ".job-tone--"+tone+",") {
			t.Errorf("public/index.css has no rule for the %q tone", tone)
		}
	}
}
