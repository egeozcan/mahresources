package jobs

import (
	"errors"
	"testing"
	"time"
)

func TestFinishFinalProgressCommitsWithTheOutcome(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2032, 1, 2, 3, 4, 5, 0, time.UTC) }
	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}
	if _, err := svc.UpdateProgress(deps, ref, Progress{
		Completed: int64Ptr(70), Total: int64Ptr(100), Unit: "percent", Message: "Fetching result...",
	}); err != nil {
		t.Fatalf("seed progress: %v", err)
	}

	finalProgress := Progress{
		Completed: int64Ptr(100), Total: int64Ptr(100), Unit: "percent", Message: "Created resource #42",
	}
	finished, err := svc.Finish(deps, FinishRequest{
		ExecutionRef: ref, ExpectedVersion: job.Version, Outcome: StateSucceeded,
		FinalProgress: &finalProgress,
	})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if finished.State != StateSucceeded || finished.Progress.Completed == nil || *finished.Progress.Completed != 100 ||
		finished.Progress.Total == nil || *finished.Progress.Total != 100 ||
		finished.Progress.Unit != "percent" || finished.Progress.Message != "Created resource #42" {
		t.Fatalf("finished snapshot = state %s, progress %+v; want succeeded with final progress", finished.State, finished.Progress)
	}
	stored := jobRow(t, deps, job.ID)
	if stored.State != string(StateSucceeded) || stored.ProgressCompleted == nil || *stored.ProgressCompleted != 100 ||
		stored.ProgressTotal == nil || *stored.ProgressTotal != 100 ||
		stored.ProgressUnit != "percent" || stored.ProgressMessage != "Created resource #42" {
		t.Fatalf("stored job = state %s, progress %d/%d %q %q; want succeeded with final progress",
			stored.State, valueOrZero(stored.ProgressCompleted), valueOrZero(stored.ProgressTotal), stored.ProgressUnit, stored.ProgressMessage)
	}
}

func TestFinishRefusalLeavesStateAndProgressUnchangedWithFinalProgress(t *testing.T) {
	tests := []struct {
		name     string
		refToken string
		final    Progress
		wantErr  error
		required []string
		cancel   bool
	}{
		{
			name:     "invalid progress",
			refToken: "claim-a",
			final:    Progress{Completed: int64Ptr(-1)},
			wantErr:  ErrInvalidProgress,
		},
		{
			name:     "stale execution token",
			refToken: "stale-token",
			final:    Progress{Completed: int64Ptr(100), Total: int64Ptr(100)},
			wantErr:  ErrStaleExecution,
		},
		{
			name:     "cancellation intent won",
			refToken: "claim-a",
			final:    Progress{Completed: int64Ptr(100), Total: int64Ptr(100)},
			wantErr:  ErrControlIntentWon,
			cancel:   true,
		},
		{
			name:     "required output unavailable",
			refToken: "claim-a",
			final:    Progress{Completed: int64Ptr(100), Total: int64Ptr(100)},
			wantErr:  ErrRequiredOutputUnavailable,
			required: []string{"result"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			svc := NewService()
			deps.Now = func() time.Time { return time.Date(2032, 1, 2, 3, 4, 5, 0, time.UTC) }
			job := seededExecution(t, deps, StateRunning, "claim-a")
			if _, err := svc.UpdateProgress(deps, ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}, Progress{
				Completed: int64Ptr(70), Total: int64Ptr(100), Unit: "percent", Message: "Fetching result...",
			}); err != nil {
				t.Fatalf("seed progress: %v", err)
			}
			if tt.cancel {
				requestedAt := deps.now()
				if err := deps.DB.Model(&job).Updates(map[string]any{
					"control_intent": ControlIntentCancel, "control_requested_at": requestedAt,
				}).Error; err != nil {
					t.Fatalf("seed cancellation intent: %v", err)
				}
			}

			_, err := svc.Finish(deps, FinishRequest{
				ExecutionRef:    ExecutionRef{JobID: job.ID, ExecutionToken: tt.refToken},
				ExpectedVersion: job.Version, Outcome: StateSucceeded,
				FinalProgress: &tt.final, RequiredOutputs: tt.required,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Finish error = %v, want %v", err, tt.wantErr)
			}

			stored := jobRow(t, deps, job.ID)
			if stored.State != string(StateRunning) || stored.ProgressCompleted == nil || *stored.ProgressCompleted != 70 ||
				stored.ProgressTotal == nil || *stored.ProgressTotal != 100 ||
				stored.ProgressUnit != "percent" || stored.ProgressMessage != "Fetching result..." {
				t.Fatalf("refused Finish changed stored state/progress: state=%s progress=%d/%d %q %q",
					stored.State, valueOrZero(stored.ProgressCompleted), valueOrZero(stored.ProgressTotal), stored.ProgressUnit, stored.ProgressMessage)
			}
		})
	}
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
