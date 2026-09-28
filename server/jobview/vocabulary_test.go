package jobview

import (
	"testing"

	"mahresources/jobs"
)

// Every failure class the Job Service records has a name, and an identifier the
// table does not know is shown as itself rather than as nothing.
func TestTheVocabularyNamesEveryFailureClass(t *testing.T) {
	for _, class := range []string{
		jobs.FailureClassPolicy, jobs.FailureClassValidation, jobs.FailureClassDependency, jobs.FailureClassTimeout,
		jobs.FailureClassCapacity, jobs.FailureClassConflict, jobs.FailureClassCancellation, jobs.FailureClassInternal,
	} {
		if label := FailureClassLabel(class); label == class || label == "" {
			t.Errorf("failure class %q has no name", class)
		}
	}
	if got := KindLabel("remote-download"); got != "Download" {
		t.Fatalf("remote-download reads %q", got)
	}
	if got := KindLabel("a-plugin-kind"); got != "a-plugin-kind" {
		t.Fatalf("an unknown Kind reads %q; want its identifier", got)
	}
	if got := OriginLabel("api"); got == "api" {
		t.Fatalf("the api origin reads %q", got)
	}
}

// A blocked reason reads as a sentence; one the table does not know is read
// from its code, never shown as blank.
func TestABlockedReasonReadsAsASentence(t *testing.T) {
	if got := BlockedReasonText("role-refused"); got == "" || got == "role-refused" {
		t.Fatalf("role-refused reads %q", got)
	}
	if got := BlockedReasonText("source-row-missing"); got != "Source row missing." {
		t.Fatalf("an unknown reason reads %q", got)
	}
	if got := BlockedReasonText(""); got != "" {
		t.Fatalf("no reason reads %q", got)
	}
}

// The phase beside a state is shown only when it says something the state does
// not.
func TestThePhaseBesideAStateIsNotARepeat(t *testing.T) {
	for _, c := range []struct {
		snapshot jobs.Snapshot
		want     string
	}{
		{jobs.Snapshot{State: jobs.StateRunning, Phase: "running"}, ""},
		{jobs.Snapshot{State: jobs.StateRunning, Phase: "downloading"}, "downloading"},
		{jobs.Snapshot{State: jobs.StatePaused, Phase: "paused"}, ""},
		{jobs.Snapshot{State: jobs.StateRunning, Phase: "cancelling", ControlIntent: jobs.ControlIntentCancel}, ""},
		{jobs.Snapshot{State: jobs.StateSucceeded, Phase: jobs.PhasePartial}, ""},
		{jobs.Snapshot{State: jobs.StateBlocked, Phase: "paused"}, "paused"},
	} {
		if got := PhaseText(c.snapshot); got != c.want {
			t.Errorf("%s with phase %q shows %q; want %q", c.snapshot.State, c.snapshot.Phase, got, c.want)
		}
	}
}
