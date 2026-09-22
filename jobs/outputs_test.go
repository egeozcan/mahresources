package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

// TestOutputPublicationStoresATypedOutputWithAnIndependentVersion covers what a
// published output is: a durable typed reference with its own availability and
// its own version, separate from the Job's lifecycle version, because a Job may
// republish the same key — an at-least-once executor that crashed after writing
// the row will — without moving the execution it is describing.
func TestOutputPublicationStoresATypedOutputWithAnIndependentVersion(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}
	expiresAt := clock.Add(24 * time.Hour)

	published, err := svc.PublishOutput(deps, ref, OutputInput{
		Key:       "artifact",
		Type:      OutputTypeArtifact,
		Label:     "group-export.tar",
		Reference: json.RawMessage(`{"path":"exports/17.tar","bytes":4096}`),
		Required:  true,
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("PublishOutput: %v", err)
	}

	if published.Key != "artifact" || published.Type != OutputTypeArtifact || published.Label != "group-export.tar" {
		t.Errorf("published output = %+v", published)
	}
	if published.Availability != OutputAvailable || published.Version != 1 || !published.Required {
		t.Errorf("published output availability/version/required = %s/%d/%v", published.Availability, published.Version, published.Required)
	}
	if string(published.Reference) != `{"path":"exports/17.tar","bytes":4096}` {
		t.Errorf("published reference = %s", published.Reference)
	}
	if published.ExpiresAt == nil || !published.ExpiresAt.Equal(expiresAt) || published.ExpiresAt.Location() != time.UTC {
		t.Errorf("published expiry = %v, want %v in UTC", published.ExpiresAt, expiresAt)
	}
	if published.ID == "" || !uuidV7Pattern.MatchString(published.ID) {
		t.Errorf("published output id = %q, want a UUIDv7 of its own", published.ID)
	}
	if stored := jobRow(t, deps, job.ID); stored.Version != job.Version {
		t.Errorf("publishing an output moved the lifecycle version to %d", stored.Version)
	}
	if events := jobEvents(t, deps, job.ID); len(events) != 0 {
		t.Errorf("publishing an available output recorded %d events", len(events))
	}

	// Republishing the same key replaces it rather than accumulating: one row
	// per Key, its own version advanced, and still no Job version movement.
	// A removal recorded against the row is cleared with it — the publication is
	// the current truth about the key, and "available" beside a removal instant
	// would be a row that contradicts itself.
	removedAt := clock.Add(-time.Minute)
	if err := deps.DB.Model(&models.JobOutput{}).Where("id = ?", published.ID).
		Updates(map[string]any{"availability": string(OutputRemoved), "removed_at": removedAt}).Error; err != nil {
		t.Fatalf("seed a removed output: %v", err)
	}

	clock = clock.Add(time.Minute)
	republished, err := svc.PublishOutput(deps, ref, OutputInput{
		Key:       "artifact",
		Type:      OutputTypeArtifact,
		Label:     "group-export.tar",
		Reference: json.RawMessage(`{"path":"exports/17.tar","bytes":8192}`),
		Required:  true,
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("second PublishOutput: %v", err)
	}
	if republished.Version != 2 {
		t.Errorf("republished version = %d, want 2", republished.Version)
	}
	if republished.Availability != OutputAvailable || republished.RemovedAt != nil {
		t.Errorf("republished availability/removedAt = %s/%v, want available and no removal instant",
			republished.Availability, republished.RemovedAt)
	}
	if string(republished.Reference) != `{"path":"exports/17.tar","bytes":8192}` {
		t.Errorf("republished reference = %s", republished.Reference)
	}

	var count int64
	deps.DB.Model(&models.JobOutput{}).Where("job_id = ? AND key = ?", job.ID, "artifact").Count(&count)
	if count != 1 {
		t.Fatalf("%d rows for one output key, want exactly one", count)
	}
}

// TestOutputPublicationRefusesAStaleExecutionToken is the same fence the rest of
// the executor surface has: an output is published under the claim that owns the
// Job, so a replaced worker cannot attach an artifact to an execution it no
// longer owns.
func TestOutputPublicationRefusesAStaleExecutionToken(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC) }

	job := seededExecution(t, deps, StateRunning, "claim-a")

	_, err := svc.PublishOutput(deps, ExecutionRef{JobID: job.ID, ExecutionToken: "claim-b"}, OutputInput{
		Key: "report", Type: OutputTypeReport, Reference: json.RawMessage(`{"url":"/reports/17"}`),
	})
	if !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("stale token = %v, want ErrStaleExecution", err)
	}
	var count int64
	deps.DB.Model(&models.JobOutput{}).Where("job_id = ?", job.ID).Count(&count)
	if count != 0 {
		t.Fatalf("a refused publication stored %d outputs", count)
	}
}

// TestOutputPublicationRefusesAFinishedJob keeps the terminal-immutability rule
// at this seam too: an execution that ended cannot gain an output afterwards,
// because the Job's outcome is what its outputs were verified against.
func TestOutputPublicationRefusesAFinishedJob(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC) }

	job := seededExecution(t, deps, StateSucceeded, "claim-a")

	_, err := svc.PublishOutput(deps, ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}, OutputInput{
		Key: "report", Type: OutputTypeReport, Reference: json.RawMessage(`{"url":"/reports/17"}`),
	})
	if !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("publishing onto a succeeded Job = %v, want ErrIllegalTransition", err)
	}
}

// TestOutputCeilingsRefuseOversizedAndExcessOutputs covers the bounds on the
// output table: a reference nobody can act on is refused at the boundary that
// accepts it, and one Job cannot grow an unbounded list of them.
func TestOutputCeilingsRefuseOversizedAndExcessOutputs(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}

	tests := []struct {
		name    string
		input   OutputInput
		target  error
		wantErr bool
	}{
		{
			name:    "no key",
			input:   OutputInput{Type: OutputTypeReport, Reference: json.RawMessage(`{"url":"/r"}`)},
			target:  ErrInvalidOutput,
			wantErr: true,
		},
		{
			name:    "key over its ceiling",
			input:   OutputInput{Key: string(make([]byte, MaxOutputKeyBytes+1)), Type: OutputTypeReport},
			target:  ErrInvalidOutput,
			wantErr: true,
		},
		{
			name:    "no type",
			input:   OutputInput{Key: "report", Reference: json.RawMessage(`{"url":"/r"}`)},
			target:  ErrInvalidOutput,
			wantErr: true,
		},
		{
			name:    "unknown type",
			input:   OutputInput{Key: "report", Type: "thumbnail", Reference: json.RawMessage(`{"url":"/r"}`)},
			target:  ErrInvalidOutput,
			wantErr: true,
		},
		{
			name: "label over its ceiling",
			input: OutputInput{
				Key: "report", Type: OutputTypeReport, Label: string(make([]byte, MaxOutputLabelBytes+1)),
			},
			target:  ErrInvalidOutput,
			wantErr: true,
		},
		{
			name: "reference over its ceiling",
			input: OutputInput{
				Key: "report", Type: OutputTypeReport,
				Reference: json.RawMessage(`"` + string(make([]byte, MaxOutputReferenceBytes+1)) + `"`),
			},
			target:  ErrInvalidOutput,
			wantErr: true,
		},
		{
			name:    "malformed reference",
			input:   OutputInput{Key: "report", Type: OutputTypeReport, Reference: json.RawMessage(`{"unterminated"`)},
			target:  ErrInvalidOutput,
			wantErr: true,
		},
		{
			name:    "no reference at all",
			input:   OutputInput{Key: "report", Type: OutputTypeReport},
			target:  ErrInvalidOutput,
			wantErr: true,
		},
		{
			name:   "a well-formed summary output",
			input:  OutputInput{Key: "summary", Type: OutputTypeSummary, Reference: json.RawMessage(`{"count":17}`)},
			target: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.PublishOutput(deps, ref, tt.input)
			switch {
			case tt.wantErr && !errors.Is(err, tt.target):
				t.Fatalf("PublishOutput = %v, want %v", err, tt.target)
			case !tt.wantErr && err != nil:
				t.Fatalf("PublishOutput: %v", err)
			}
		})
	}

	// The count ceiling: one Job's output list is bounded, and republishing a
	// key it already has is always allowed.
	stored := int64(0)
	deps.DB.Model(&models.JobOutput{}).Where("job_id = ?", job.ID).Count(&stored)
	for i := int64(0); i < MaxOutputsPerJob-stored; i++ {
		_, err := svc.PublishOutput(deps, ref, OutputInput{
			Key:       outputKey(i),
			Type:      OutputTypeEntity,
			Reference: json.RawMessage(`{"resourceId":17}`),
		})
		if err != nil {
			t.Fatalf("publishing output %d: %v", i, err)
		}
	}

	if _, err := svc.PublishOutput(deps, ref, OutputInput{
		Key: "one-too-many", Type: OutputTypeEntity, Reference: json.RawMessage(`{"resourceId":17}`),
	}); !errors.Is(err, ErrOutputCapacityExhausted) {
		t.Fatalf("over the output ceiling = %v, want ErrOutputCapacityExhausted", err)
	}

	var count int64
	deps.DB.Model(&models.JobOutput{}).Where("job_id = ?", job.ID).Count(&count)
	if count != MaxOutputsPerJob {
		t.Fatalf("stored %d outputs, want the %d-output ceiling", count, MaxOutputsPerJob)
	}

	if _, err := svc.PublishOutput(deps, ref, OutputInput{
		Key: outputKey(0), Type: OutputTypeEntity, Reference: json.RawMessage(`{"resourceId":18}`),
	}); err != nil {
		t.Fatalf("republishing an existing key at the ceiling: %v", err)
	}
}

func outputKey(i int64) string {
	return fmt.Sprintf("entity-%d", i)
}

// TestOutputOptionalFailureWarnsWithoutChangingOutcome is §7's asymmetry: a
// required output that is not available prevents success, while an optional one
// that could not be made available becomes a bounded warning and leaves the
// execution alone — an artifact nobody can fetch must not fail finished work,
// and it must not be advertised as available either.
func TestOutputOptionalFailureWarnsWithoutChangingOutcome(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}
	past := clock.Add(-time.Hour)

	published, err := svc.PublishOutput(deps, ref, OutputInput{
		Key:       "report",
		Type:      OutputTypeReport,
		Label:     "partial import report",
		Reference: json.RawMessage(`{"url":"/imports/17/report"}`),
		ExpiresAt: &past,
	})
	if err != nil {
		t.Fatalf("PublishOutput: %v", err)
	}
	if published.Availability != OutputExpired {
		t.Fatalf("availability = %s, want %s for an output whose planned expiry has passed", published.Availability, OutputExpired)
	}

	if stored := jobRow(t, deps, job.ID); stored.State != string(StateRunning) {
		t.Fatalf("an optional output failure left the Job in %s", stored.State)
	}
	events := jobEvents(t, deps, job.ID)
	if len(events) != 1 || events[0].Type != EventWarning {
		t.Fatalf("events = %+v, want one warning", events)
	}
	if events[0].ReservedHost {
		t.Error("the optional-output warning is not a lifecycle fact and must not claim the reserved capacity")
	}

	// The committed success is unchanged by it: the warning stays on the
	// timeline and the Job still finishes.
	snap, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ref,
		ExpectedVersion: job.Version,
		Outcome:         StateSucceeded,
	})
	if err != nil {
		t.Fatalf("Finish after an optional output failure: %v", err)
	}
	if snap.State != StateSucceeded {
		t.Fatalf("state = %s, want succeeded", snap.State)
	}
	events = jobEvents(t, deps, job.ID)
	if len(events) != 2 || events[0].Type != EventWarning || events[1].Type != EventSucceeded {
		t.Fatalf("timeline = %+v, want the warning kept ahead of the terminal event", events)
	}
}

// TestOutputRequiredOutputsAndSuccessCommitTogether is §7's success contract: a
// Job becomes succeeded only after the outputs success depends on are durable
// and available. Verification runs inside the terminal transaction, so the state
// and its event commit with the outputs they were checked against — and a
// caller's own transaction takes the whole fact back with it or none of it.
func TestOutputRequiredOutputsAndSuccessCommitTogether(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}
	expiresAt := clock.Add(24 * time.Hour)

	published, err := svc.PublishOutput(deps, ref, OutputInput{
		Key:       "artifact",
		Type:      OutputTypeArtifact,
		Label:     "group-export.tar",
		Reference: json.RawMessage(`{"path":"exports/17.tar"}`),
		Required:  true,
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("PublishOutput: %v", err)
	}

	snap, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ref,
		ExpectedVersion: job.Version,
		Outcome:         StateSucceeded,
		RequiredOutputs: []string{"artifact"},
	})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if snap.State != StateSucceeded || snap.Version != job.Version+1 {
		t.Fatalf("snapshot = %s v%d, want succeeded v%d", snap.State, snap.Version, job.Version+1)
	}
	if snap.Terminal() == false || snap.FinishedAt == nil || !snap.FinishedAt.Equal(clock) {
		t.Fatalf("terminal snapshot = %+v", snap)
	}

	stored := jobRow(t, deps, job.ID)
	if stored.State != string(StateSucceeded) {
		t.Fatalf("stored state = %s", stored.State)
	}
	events := jobEvents(t, deps, job.ID)
	if len(events) != 1 || events[0].Type != EventSucceeded || events[0].JobVersion != snap.Version {
		t.Fatalf("terminal events = %+v", events)
	}
	var output models.JobOutput
	if err := deps.DB.Where("id = ?", published.ID).First(&output).Error; err != nil {
		t.Fatalf("read output: %v", err)
	}
	if output.Availability != string(OutputAvailable) {
		t.Fatalf("verified output availability = %s", output.Availability)
	}

	// A finish inside a caller's transaction is that caller's fact: it commits
	// with the surrounding work or not at all, so the terminal state and its
	// event never outlive a domain write that was rolled back.
	second := seededExecution(t, deps, StateRunning, "claim-b")
	secondRef := ExecutionRef{JobID: second.ID, ExecutionToken: "claim-b"}
	if _, err := svc.PublishOutput(deps, secondRef, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Reference: json.RawMessage(`{"path":"exports/18.tar"}`), Required: true,
	}); err != nil {
		t.Fatalf("publish second artifact: %v", err)
	}
	rollback := errors.New("the caller rolled back")
	err = deps.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := svc.Finish(Deps{DB: tx, Now: deps.Now}, FinishRequest{
			ExecutionRef:    secondRef,
			ExpectedVersion: second.Version,
			Outcome:         StateSucceeded,
			RequiredOutputs: []string{"artifact"},
		}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("caller transaction = %v, want %v", err, rollback)
	}
	if stored := jobRow(t, deps, second.ID); stored.State != string(StateRunning) {
		t.Fatalf("a rolled-back finish left the Job in %s", stored.State)
	}
	if events := jobEvents(t, deps, second.ID); len(events) != 0 {
		t.Fatalf("a rolled-back finish recorded %d events", len(events))
	}
}

// TestFinishRefusesSuccessWithoutAvailableRequiredOutputs is the other half of
// the same contract: success is refused — writing nothing — when an output it
// depends on is missing, or when a row it published as required is no longer
// available. The stored rows are authoritative, so an adapter that forgets to
// name a required output at finish time cannot slip past it.
func TestFinishRefusesSuccessWithoutAvailableRequiredOutputs(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	t.Run("a required output that was never published", func(t *testing.T) {
		job := seededExecution(t, deps, StateRunning, "claim-a")
		_, err := svc.Finish(deps, FinishRequest{
			ExecutionRef:    ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"},
			ExpectedVersion: job.Version,
			Outcome:         StateSucceeded,
			RequiredOutputs: []string{"artifact"},
		})
		if !errors.Is(err, ErrRequiredOutputUnavailable) {
			t.Fatalf("Finish = %v, want ErrRequiredOutputUnavailable", err)
		}
		if stored := jobRow(t, deps, job.ID); stored.State != string(StateRunning) || stored.Version != job.Version {
			t.Fatalf("a refused success left the Job in %s v%d", stored.State, stored.Version)
		}
		if events := jobEvents(t, deps, job.ID); len(events) != 0 {
			t.Fatalf("a refused success recorded %d events", len(events))
		}
	})

	t.Run("a required output that is no longer available", func(t *testing.T) {
		job := seededExecution(t, deps, StateRunning, "claim-b")
		ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-b"}
		past := clock.Add(-time.Hour)
		if _, err := svc.PublishOutput(deps, ref, OutputInput{
			Key: "artifact", Type: OutputTypeArtifact, Required: true,
			Reference: json.RawMessage(`{"path":"exports/19.tar"}`), ExpiresAt: &past,
		}); err != nil {
			t.Fatalf("PublishOutput: %v", err)
		}

		// The caller names nothing: the durable required row is what success is
		// verified against.
		_, err := svc.Finish(deps, FinishRequest{
			ExecutionRef:    ref,
			ExpectedVersion: job.Version,
			Outcome:         StateSucceeded,
		})
		if !errors.Is(err, ErrRequiredOutputUnavailable) {
			t.Fatalf("Finish = %v, want ErrRequiredOutputUnavailable", err)
		}
		if stored := jobRow(t, deps, job.ID); stored.State != string(StateRunning) {
			t.Fatalf("a refused success left the Job in %s", stored.State)
		}
	})

	t.Run("the same row published as optional", func(t *testing.T) {
		job := seededExecution(t, deps, StateRunning, "claim-c")
		ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-c"}
		past := clock.Add(-time.Hour)
		if _, err := svc.PublishOutput(deps, ref, OutputInput{
			Key: "artifact", Type: OutputTypeArtifact,
			Reference: json.RawMessage(`{"path":"exports/20.tar"}`), ExpiresAt: &past,
		}); err != nil {
			t.Fatalf("PublishOutput: %v", err)
		}
		if _, err := svc.Finish(deps, FinishRequest{
			ExecutionRef:    ref,
			ExpectedVersion: job.Version,
			Outcome:         StateSucceeded,
		}); err != nil {
			t.Fatalf("optional output availability must not refuse success: %v", err)
		}
	})
}

// TestFinishRefusesANonTerminalOutcomeAndAStaleExecutionToken covers the rest of
// what Finish refuses. It ends a Job, so anything that is not an end state is
// refused before anything is written, and the token fence applies exactly as it
// does to a plain transition.
func TestFinishRefusesANonTerminalOutcomeAndAStaleExecutionToken(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC) }

	job := seededExecution(t, deps, StateRunning, "claim-a")

	for _, outcome := range []State{StateQueued, StateRunning, StateBlocked, StatePaused, StateScheduled} {
		_, err := svc.Finish(deps, FinishRequest{
			ExecutionRef:    ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"},
			ExpectedVersion: job.Version,
			Outcome:         outcome,
		})
		if !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("Finish(%s) = %v, want ErrInvalidTransition", outcome, err)
		}
	}

	_, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: job.ID, ExecutionToken: "claim-b"},
		ExpectedVersion: job.Version,
		Outcome:         StateSucceeded,
	})
	if !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("stale token = %v, want ErrStaleExecution", err)
	}

	if stored := jobRow(t, deps, job.ID); stored.State != string(StateRunning) || stored.Version != job.Version {
		t.Fatalf("a refused finish left the Job in %s v%d", stored.State, stored.Version)
	}
	if events := jobEvents(t, deps, job.ID); len(events) != 0 {
		t.Fatalf("a refused finish recorded %d events", len(events))
	}

	// A failure is a finish too, and it records its taxonomy the same way a
	// transition does.
	snap, err := svc.Finish(deps, FinishRequest{
		ExecutionRef:    ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"},
		ExpectedVersion: job.Version,
		Outcome:         StateFailed,
		Failure:         &Failure{Code: "origin-5xx", Class: FailureClassDependency, Message: "the origin refused"},
	})
	if err != nil {
		t.Fatalf("Finish(failed): %v", err)
	}
	if snap.State != StateFailed || snap.Failure == nil || snap.Failure.Code != "origin-5xx" {
		t.Fatalf("snapshot = %+v", snap)
	}
}

// TestOutputSuccessThroughATransitionVerifiesRequiredOutputs closes the hole a
// second success boundary opened: Execution.Transition permits running ->
// succeeded, and it used to commit that success without the output verification
// Finish performs, so an adapter could publish an artifact that is not available
// and then record success by transitioning rather than finishing.
//
// §7 states the contract at the outcome, not at one entry point: a Job becomes
// succeeded only after its required outputs are durable and available, whichever
// call records it.
func TestOutputSuccessThroughATransitionVerifiesRequiredOutputs(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	t.Run("an unavailable required output refuses a success recorded by a transition", func(t *testing.T) {
		job := seededExecution(t, deps, StateRunning, "claim-a")
		ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}
		past := clock.Add(-time.Hour)
		if _, err := svc.PublishOutput(deps, ref, OutputInput{
			Key: "artifact", Type: OutputTypeArtifact, Required: true,
			Reference: json.RawMessage(`{"path":"exports/21.tar"}`), ExpiresAt: &past,
		}); err != nil {
			t.Fatalf("PublishOutput: %v", err)
		}

		_, err := svc.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: job.Version, ExecutionToken: "claim-a", To: StateSucceeded,
		})
		if !errors.Is(err, ErrRequiredOutputUnavailable) {
			t.Fatalf("Transition to succeeded = %v, want ErrRequiredOutputUnavailable", err)
		}
		if stored := jobRow(t, deps, job.ID); stored.State != string(StateRunning) || stored.Version != job.Version {
			t.Fatalf("a refused success left the Job in %s v%d", stored.State, stored.Version)
		}
		if events := jobEvents(t, deps, job.ID); len(events) != 0 {
			t.Fatalf("a refused success recorded %d events", len(events))
		}
	})

	t.Run("a Job with no required outputs still succeeds through a transition", func(t *testing.T) {
		job := seededExecution(t, deps, StateRunning, "claim-b")
		snap, err := svc.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: job.Version, ExecutionToken: "claim-b", To: StateSucceeded,
		})
		if err != nil {
			t.Fatalf("Transition to succeeded: %v", err)
		}
		if snap.State != StateSucceeded {
			t.Fatalf("state = %s, want succeeded", snap.State)
		}
	})
}

// TestOutputRequiredOutputsAreVerifiedAgainstTheirDeadlineNotOnlyTheStoredString
// is §7's "required artifact availability is verified" read as a fact about now
// rather than about a string some earlier write recorded.
//
// An artifact may expire at any moment, and a sweep that records that fact runs
// on its own cadence — so between the two there is a window in which the row
// still says available and the artifact is gone. Verifying the string alone
// commits success over an artifact nobody can fetch.
func TestOutputRequiredOutputsAreVerifiedAgainstTheirDeadlineNotOnlyTheStoredString(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2032, 3, 4, 5, 6, 7, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}
	expiresAt := clock.Add(time.Hour)
	published, err := svc.PublishOutput(deps, ref, OutputInput{
		Key: "artifact", Type: OutputTypeArtifact, Required: true,
		Reference: json.RawMessage(`{"path":"exports/22.tar"}`), ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("PublishOutput: %v", err)
	}
	if published.Availability != OutputAvailable {
		t.Fatalf("published availability = %s, want available while the deadline is ahead", published.Availability)
	}

	// The artifact's own deadline passes and no sweep has recorded it yet.
	clock = clock.Add(2 * time.Hour)

	_, err = svc.Finish(deps, FinishRequest{
		ExecutionRef: ref, ExpectedVersion: job.Version, Outcome: StateSucceeded,
	})
	if !errors.Is(err, ErrRequiredOutputUnavailable) {
		t.Fatalf("Finish over an artifact past its deadline = %v, want ErrRequiredOutputUnavailable", err)
	}
	if stored := jobRow(t, deps, job.ID); stored.State != string(StateRunning) {
		t.Fatalf("a refused success left the Job in %s", stored.State)
	}
}
