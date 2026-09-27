package jobview

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"mahresources/jobs"
)

// job_states.json is how every Job surface names a state: its label, the drawer
// group it belongs to, the tone its badge takes, and whether it is work in
// progress that may draw a moving bar. The server-rendered list reads it here and
// the browser components import the same file, so a card, the Job page and the
// Jobs drawer cannot say different things about one Job.
//
//go:embed job_states.json
var jobStatesJSON []byte

// StatePresentation is one state's entry in job_states.json.
type StatePresentation struct {
	Label string `json:"label"`
	// Group is the Jobs drawer section the state is listed under: "attention"
	// (Needs attention), "active" (Active and scheduled) or "finished" (the
	// finished Jobs that need no attention).
	Group string `json:"group"`
	// Tone is the badge colour's meaning: working, waiting, paused, warning,
	// done, failed or neutral. The label says the same thing in words.
	Tone string `json:"tone"`
	// Working is true only for a state in which an executor is doing the work,
	// so only it may draw an indeterminate "Working" bar.
	Working  bool `json:"working"`
	Terminal bool `json:"terminal"`
}

type statePresentationTable struct {
	States  map[string]StatePresentation `json:"states"`
	Partial StatePresentation            `json:"partial"`
	// RunningIntents is what a running Job says while a control it was asked for
	// is on its way to its executor: it is still running, and not yet paused or
	// cancelled.
	RunningIntents map[string]StatePresentation `json:"runningIntents"`
	Unknown        StatePresentation            `json:"unknown"`
}

var statePresentations = mustLoadStatePresentations()

func mustLoadStatePresentations() statePresentationTable {
	var table statePresentationTable
	if err := json.Unmarshal(jobStatesJSON, &table); err != nil {
		panic(fmt.Sprintf("jobview: job_states.json: %v", err))
	}
	return table
}

// PresentState is how every surface shows one Job's state. A succeeded Job its
// Kind recorded as partial reads as partially completed, with that entry's label
// and tone.
func PresentState(state jobs.State, phase string) StatePresentation {
	presentation, ok := statePresentations.States[string(state)]
	if !ok {
		return statePresentations.Unknown
	}
	if state == jobs.StateSucceeded && phase == jobs.PhasePartial {
		presentation.Label = statePresentations.Partial.Label
		presentation.Tone = statePresentations.Partial.Tone
	}
	return presentation
}

// PresentJob is how every surface shows one Job: its state as PresentState
// shows it, and, while it runs with a pause or a cancellation requested but not
// yet confirmed by its executor, what that request is doing ("Pausing",
// "Cancelling"). It stays running work in its group and in whether it may pulse.
func PresentJob(snapshot jobs.Snapshot) StatePresentation {
	presentation := PresentState(snapshot.State, snapshot.Phase)
	if snapshot.State != jobs.StateRunning {
		return presentation
	}
	if intent, ok := statePresentations.RunningIntents[snapshot.ControlIntent]; ok {
		presentation.Label = intent.Label
		presentation.Tone = intent.Tone
	}
	return presentation
}

// StatesInGroup lists the states the table puts in one drawer group, in the
// order jobs.AllStates presents them.
func StatesInGroup(group string) []string {
	var states []string
	for _, state := range jobs.AllStates {
		if statePresentations.States[string(state)].Group == group {
			states = append(states, string(state))
		}
	}
	return states
}

// TerminalStates lists the finished states — what the Job Center's Finished
// filter means — in the order jobs.AllStates presents them.
func TerminalStates() []string {
	var states []string
	for _, state := range jobs.AllStates {
		if statePresentations.States[string(state)].Terminal {
			states = append(states, string(state))
		}
	}
	return states
}
