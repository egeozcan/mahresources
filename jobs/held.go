package jobs

import (
	"encoding/json"
	"fmt"
)

// PauseBlockedHold records a blocked Job as the paused hold its Kind says it is.
//
// An earlier release recorded a person's hold of a download as blocked. The state
// machine has no edge from blocked to paused, because a pause is confirmed by the
// execution that owns running work and a blocked Job has none; this is the one
// writer that moves a Job that way, for a Kind that knows the block records a hold
// its executor already confirmed. It moves only a Job no execution owns: a
// quarantine keeps its token and is never a hold. A cancellation that won owns the
// Job's outcome, so that Job stays where it is.
//
// progress, when set, replaces the row's progress snapshot in the same write, so
// the paused row can say what Resume does.
func (s *Service) PauseBlockedHold(deps Deps, jobID string, expectedVersion uint64, detail json.RawMessage, progress *Progress) (Snapshot, error) {
	transition := Transition{
		JobID:           jobID,
		ExpectedVersion: expectedVersion,
		To:              StatePaused,
		Event:           EventInput{Type: EventPaused, Detail: detail},
		Progress:        progress,
	}
	if err := validateTransition(&transition); err != nil {
		return Snapshot{}, err
	}
	job, err := loadJob(deps.DB, jobID)
	if err != nil {
		return Snapshot{}, err
	}
	if job.Version != expectedVersion {
		return Snapshot{}, fmt.Errorf("%w: job %s is at version %d, the request expected %d",
			ErrVersionConflict, job.ID, job.Version, expectedVersion)
	}
	if State(job.State) != StateBlocked || job.ExecutionToken != "" {
		return Snapshot{}, fmt.Errorf("%w: job %s is not a block no execution owns", ErrIllegalTransition, job.ID)
	}
	if job.ControlIntent == ControlIntentCancel {
		return Snapshot{}, fmt.Errorf("%w: job %s", ErrControlIntentWon, job.ID)
	}

	now := deps.now()
	next, updates := applyTransition(job, transition, deps.retention(), now)
	prepared := preparedTransition{
		job:           job,
		next:          next,
		updates:       updates,
		eventType:     EventPaused,
		eventDetail:   detail,
		at:            now,
		releaseReason: ReleaseReasonStateChanged,
	}
	if progress != nil {
		if err := applyFinalProgress(&prepared, *progress, now); err != nil {
			return Snapshot{}, err
		}
	}
	return s.commitTransition(deps, prepared, nil)
}
