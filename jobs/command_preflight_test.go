package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// preflightAdapter is the test Kind with a dispatch-time refusal it can already
// see when a command asks.
type preflightAdapter struct {
	*testAdapter
	refuse func(CommandContext, string) CommandRefusal
	asked  []string
}

func (a *preflightAdapter) PreflightCommand(_ context.Context, command CommandContext, key string) (CommandRefusal, error) {
	a.asked = append(a.asked, key)
	if a.refuse == nil {
		return CommandRefusal{}, nil
	}
	return a.refuse(command, key), nil
}

// A command that starts work is refused up front, with the Kind's reason, when
// the Kind's dispatch would refuse it anyway: a Retry must not create a
// successor that can only block, and a Resume must not queue a Job to block it
// again. Nothing is written, so asking again once the reason clears works.
func TestACommandTheKindWouldRefuseIsRefusedWithItsReason(t *testing.T) {
	deps := newTestDeps(t)
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "preflight-key")}
	clock := time.Date(2034, 7, 1, 12, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { clock = clock.Add(time.Second); return clock }
	svc := NewService()
	registerTestCodec(t, svc)
	adapter := &preflightAdapter{testAdapter: newTestAdapter(testDefinition())}
	adapter.advertise = func(_ context.Context, command CommandContext) ([]Command, error) {
		switch command.Snapshot.State {
		case StateFailed:
			return []Command{{Key: CommandRetry, Label: "Retry"}}, nil
		case StateBlocked:
			return []Command{{Key: CommandResume, Label: "Resume"}}, nil
		}
		return nil, nil
	}
	if err := svc.RegisterAdapter(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	refusing := true
	adapter.refuse = func(_ CommandContext, key string) CommandRefusal {
		if !refusing {
			return CommandRefusal{}
		}
		return CommandRefusal{Reason: "scope-refused", Message: "the target group is outside your permitted scope"}
	}

	owner := uint(7)
	viewer := Access{UserID: owner}
	accept := func() Snapshot {
		return acceptFor(t, svc, deps, Acceptance{
			Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: &owner, ActorUserID: &owner, Replay: ReplayInput{Input: json.RawMessage(`{"url":"x"}`)},
		})
	}
	settle := func(job Snapshot, to State) {
		execution, ok := claimOnce(t, svc, deps, "runtime")
		if !ok || execution.JobID != job.ID {
			t.Fatalf("claim %s: %+v, %v", job.ID, execution, ok)
		}
		var err error
		if to == StateFailed {
			_, err = svc.Finish(deps, FinishRequest{
				ExecutionRef:    ExecutionRef{JobID: job.ID, ExecutionToken: execution.ExecutionToken},
				ExpectedVersion: jobRow(t, deps, job.ID).Version, Outcome: StateFailed,
				Failure: &Failure{Code: "boom", Class: FailureClassInternal},
			})
		} else {
			_, err = svc.Transition(deps, Transition{
				JobID: job.ID, ExpectedVersion: jobRow(t, deps, job.ID).Version,
				ExecutionToken: execution.ExecutionToken, To: to, Event: EventInput{Type: EventBlocked},
			})
		}
		if err != nil {
			t.Fatalf("move %s to %s: %v", job.ID, to, err)
		}
	}
	failed := accept()
	settle(failed, StateFailed)
	blocked := accept()
	settle(blocked, StateBlocked)

	for _, target := range []struct {
		job Snapshot
		key string
	}{{failed, CommandRetry}, {blocked, CommandResume}} {
		before := jobRow(t, deps, target.job.ID).Version
		result, err := svc.ExecuteCommand(context.Background(), deps, CommandRequest{
			JobID: target.job.ID, Key: target.key, IdempotencyKey: "refused-" + target.key,
			ExpectedVersion: before, Actor: viewer, Origin: "api",
		})
		if !errors.Is(err, ErrCommandRefused) || result.Code != CommandCodeRefused ||
			result.Message != "the target group is outside your permitted scope" {
			t.Fatalf("%s = %+v, %v; want refused with the Kind's reason", target.key, result, err)
		}
		if result.SuccessorID != "" {
			t.Fatalf("a refused %s created successor %s", target.key, result.SuccessorID)
		}
		if after := jobRow(t, deps, target.job.ID).Version; after != before {
			t.Fatalf("a refused %s moved the job from version %d to %d", target.key, before, after)
		}
	}
	successors, err := retrySuccessors(deps.DB, failed.ID)
	if err != nil || len(successors) != 0 {
		t.Fatalf("the refused retry left successors %v, %v", successors, err)
	}

	refusing = false
	result, err := svc.ExecuteCommand(context.Background(), deps, CommandRequest{
		JobID: failed.ID, Key: CommandRetry, IdempotencyKey: "refused-" + CommandRetry,
		ExpectedVersion: jobRow(t, deps, failed.ID).Version, Actor: viewer, Origin: "api",
	})
	if err != nil || result.SuccessorID == "" {
		t.Fatalf("the retry once allowed = %+v, %v; the refusal must have recorded nothing", result, err)
	}
}
