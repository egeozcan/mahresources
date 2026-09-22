package jobs

import (
	"fmt"
	"strings"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

// This file holds ordinary history retention: the bounded, resumable sweep that
// takes finished work out of the Job tables once its window has passed.
//
// Three things are deliberately outside its reach, and each of them is a
// different kind of "not yet":
//
//   - nonterminal work, including blocked work waiting for a person. Its window
//     has not started, so there is nothing to expire;
//   - a Job an unresolved claim still protects. A quarantined claim means the
//     process group that held it may still be alive, and no sweep may go around
//     that;
//   - a pinned Job's metadata and events, for as long as any viewer holds the
//     pin. A pin exempts nothing else: an artifact has its own expiry, and a
//     relative is a different Job.
//
// Everything it does is bounded and idempotent, and it is driven by a keyset
// cursor rather than by a page number, because the rows in front of the cursor
// are the ones it is deleting.

// Sweep takes one bounded batch of finished Jobs out of ordinary history.
//
// The window is measured from finished_at — never from acceptance, never from
// the last write — because that is when the retention the design describes
// starts. A Job carries its own deadline, stamped by the terminal transition
// from the policy in effect at the time, so a later policy change does not
// retroactively rewrite what happened. A Job that finished before deadlines
// existed is given the policy's window once, from its own finish time, which is
// the same thing migration does for backfilled history.
//
// The caller drives it with the returned cursor until Next is nil; a pass with
// no cursor starts again from the oldest expired work, which is safe because a
// pass is idempotent and because pruning is what removes the rows in front of
// it.
func (s *Service) Sweep(deps Deps, policy RetentionPolicy, cursor SweepCursor, limit int) (SweepResult, error) {
	size, err := sweepBatchSize(limit)
	if err != nil {
		return SweepResult{}, err
	}
	if err := validateSweepCursor(cursor); err != nil {
		return SweepResult{}, err
	}
	now := deps.now()
	var result SweepResult

	// Replay retention runs on its own clock, from terminal completion, and it is
	// bounded the same way: an envelope whose window passed is purged whether or
	// not the Job's metadata is due, and a Job whose metadata goes takes its
	// envelope row with it.
	envelopes, err := s.PurgeExpiredReplay(deps, size)
	if err != nil {
		return result, err
	}
	result.Envelopes = envelopes

	if err := s.stampMissingDeadlines(deps, policy, size); err != nil {
		return result, err
	}

	var candidates []models.Job
	err = deps.DB.Model(&models.Job{}).
		Where("state IN ?", terminalStates()).
		Where("finished_at IS NOT NULL AND expires_at IS NOT NULL AND expires_at <= ?", now).
		Where("(jobs.finished_at > ? OR (jobs.finished_at = ? AND jobs.id > ?))", cursor.FinishedAt, cursor.FinishedAt, cursor.ID).
		Order("finished_at ASC, jobs.id ASC").
		Limit(size).
		Find(&candidates).Error
	if err != nil {
		return result, fmt.Errorf("jobs: select expired jobs: %w", err)
	}
	result.Examined = len(candidates)

	for _, candidate := range candidates {
		pruned, outputs, err := s.pruneExpiredJob(deps, candidate, now)
		if err != nil {
			return result, err
		}
		result.Outputs += outputs
		if pruned {
			result.Pruned++
		} else {
			result.Skipped++
		}
	}
	if len(candidates) == size {
		last := candidates[len(candidates)-1]
		result.Next = &SweepCursor{FinishedAt: last.FinishedAt.UTC(), ID: last.ID}
	}
	return result, nil
}

// expiredJobPredicate is the retention decision, written once: a terminal Job
// whose deadline passed, which nobody pinned and no unresolved claim still
// protects. It is re-asserted inside the deleting transaction, so the decision
// and the delete cannot be separated by a pin or a claim landing in between.
const expiredJobPredicate = `jobs.id = ? AND jobs.state IN ? AND jobs.expires_at IS NOT NULL AND jobs.expires_at <= ?
	AND NOT EXISTS (SELECT 1 FROM job_preferences p WHERE p.job_id = jobs.id AND p.pinned_at IS NOT NULL)
	AND NOT EXISTS (SELECT 1 FROM job_claims c WHERE c.job_id = jobs.id AND c.state IN ?)`

// pruneExpiredJob applies one candidate in its own transaction.
//
// The transaction opens with the guarded delete rather than with a read: on
// SQLite the first statement of a write transaction must be the write, or a read
// snapshot is promoted to a write after another connection has committed, which
// SQLite refuses without running the busy handler. The delete therefore carries
// the whole decision, and its row count is the answer — a Job that was pinned,
// claimed or changed underneath the selection matches no row and is left where
// it is.
func (s *Service) pruneExpiredJob(deps Deps, candidate models.Job, now time.Time) (bool, int, error) {
	pruned := false
	outputs := 0

	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Where(expiredJobPredicate,
			candidate.ID, terminalStates(), now, unresolvedClaimStates(),
		).Delete(&models.Job{})
		if result.Error != nil {
			return fmt.Errorf("jobs: prune expired job %s: %w", candidate.ID, result.Error)
		}
		pruned = result.RowsAffected == 1

		// Outputs are recorded before the history that points at them goes: an
		// artifact whose expiry has passed is no longer available whatever the
		// Job's own retention says, and a pin is not a retention policy for what
		// a Job produced. Recording it first is what keeps a Job from claiming an
		// artifact it no longer has.
		if !pruned {
			// The delete matched nothing, and the two reasons are not the same.
			// A pinned Job keeps its history and still has its own artifact
			// expiry recorded. A Job an unresolved claim protects is not touched
			// at all: a claim nothing could prove dead is the one thing §9 says no
			// expiry may write through, and "not touching it" has to mean no row
			// of its own either.
			protected, protectErr := protectedByUnresolvedClaim(tx, candidate.ID)
			if protectErr != nil {
				return protectErr
			}
			if protected {
				return nil
			}
			var err error
			outputs, err = recordOutputAvailability(tx, candidate.ID, false, now)
			return err
		}

		var outputErr error
		outputs, outputErr = recordOutputAvailability(tx, candidate.ID, true, now)
		if outputErr != nil {
			return outputErr
		}

		for _, table := range []any{
			&models.JobEvent{}, &models.JobOutput{}, &models.JobPreference{},
			&models.JobClaim{}, &models.JobCapacityLease{}, &models.JobReplayEnvelope{},
		} {
			if err := tx.Where("job_id = ?", candidate.ID).Delete(table).Error; err != nil {
				return fmt.Errorf("jobs: prune %s of job %s: %w", tableName(tx, table), candidate.ID, err)
			}
		}
		// Lineage is pruned from both ends: a link is a fact about two Jobs, and
		// one endpoint expiring must not leave a row no reader can resolve. The
		// relation itself is not history a viewer can see alone — it names the
		// other endpoint's identity — so it goes with the Job rather than
		// becoming a dangling edge.
		if err := tx.Where("from_job_id = ? OR to_job_id = ?", candidate.ID, candidate.ID).
			Delete(&models.JobLink{}).Error; err != nil {
			return fmt.Errorf("jobs: prune lineage of job %s: %w", candidate.ID, err)
		}
		return nil
	})
	return pruned, outputs, err
}

// protectedByUnresolvedClaim reports whether an execution claim nobody could
// resolve still protects a Job. It is asked after the guarded delete has already
// taken the transaction's write, so it is a read inside a write transaction
// rather than a read that a write is promoted from.
func protectedByUnresolvedClaim(tx *gorm.DB, jobID string) (bool, error) {
	var count int64
	err := tx.Model(&models.JobClaim{}).
		Where("job_id = ? AND state IN ?", jobID, unresolvedClaimStates()).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("jobs: read claim state for %s: %w", jobID, err)
	}
	return count > 0, nil
}

// recordOutputAvailability records what happened to a Job's outputs, and
// reports how many rows it changed.
//
// A pruned Job's outputs are gone with it, and are recorded removed first. A Job
// the pass leaves alone keeps them, and only the ones whose own planned expiry
// has passed are marked expired — a recorded fact rather than a clock comparison
// at read time, so every reader agrees about what happened.
func recordOutputAvailability(tx *gorm.DB, jobID string, pruned bool, now time.Time) (int, error) {
	query := tx.Model(&models.JobOutput{}).Where("job_id = ?", jobID)
	updates := map[string]any{"updated_at": now}

	if pruned {
		query = query.Where("availability <> ?", string(OutputRemoved))
		updates["availability"] = string(OutputRemoved)
		updates["removed_at"] = now
	} else {
		query = query.Where("availability = ?", string(OutputAvailable)).
			Where("expires_at IS NOT NULL AND expires_at <= ?", now)
		updates["availability"] = string(OutputExpired)
	}
	updates["version"] = gorm.Expr("version + 1")

	result := query.Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("jobs: record output availability for %s: %w", jobID, result.Error)
	}
	return int(result.RowsAffected), nil
}

// stampMissingDeadlines gives the policy's window to terminal Jobs that carry no
// deadline: work that finished before deadlines existed, which is what migration
// faces for backfilled history. It is bounded, it writes only where the column
// is still NULL, and it measures from the Job's own finish instant rather than
// from now — so a legacy Job is not handed a full window from the moment someone
// upgraded.
func (s *Service) stampMissingDeadlines(deps Deps, policy RetentionPolicy, limit int) error {
	var rows []models.Job
	err := deps.DB.Model(&models.Job{}).
		Where("state IN ?", terminalStates()).
		Where("finished_at IS NOT NULL AND expires_at IS NULL").
		Order("finished_at ASC, jobs.id ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return fmt.Errorf("jobs: read jobs without a deadline: %w", err)
	}
	for _, row := range rows {
		expires := row.FinishedAt.UTC().Add(policy.windowFor(State(row.State)))
		if err := deps.DB.Model(&models.Job{}).
			Where("id = ? AND expires_at IS NULL", row.ID).
			Update("expires_at", expires).Error; err != nil {
			return fmt.Errorf("jobs: stamp deadline for %s: %w", row.ID, err)
		}
	}
	return nil
}

// sweepBatchSize resolves a requested batch, refusing one beyond the ceiling
// rather than deleting more than the caller asked for.
func sweepBatchSize(limit int) (int, error) {
	switch {
	case limit == 0:
		return DefaultSweepBatch, nil
	case limit < 0:
		return 0, fmt.Errorf("%w: %d", ErrInvalidPage, limit)
	case limit > MaxSweepBatch:
		return 0, fmt.Errorf("%w: %d is over the %d-job sweep ceiling", ErrInvalidPage, limit, MaxSweepBatch)
	}
	return limit, nil
}

// validateSweepCursor checks a sweep position the same way a listing checks one:
// the zero value starts at the oldest expired work, and anything else names both
// halves of the key.
func validateSweepCursor(cursor SweepCursor) error {
	if cursor.ID == "" && cursor.FinishedAt.IsZero() {
		return nil
	}
	if cursor.ID == "" || cursor.FinishedAt.IsZero() {
		return fmt.Errorf("%w: a sweep cursor needs both the finish instant and the job id", ErrInvalidCursor)
	}
	return nil
}

// unresolvedClaimStates are the claim states that still protect a Job: one an
// execution holds, and one nobody could prove had stopped.
func unresolvedClaimStates() []string {
	return []string{models.JobClaimStateHeld, models.JobClaimStateQuarantined}
}

// tableName names a model's table for an error message, so a failed prune says
// which part of the history it could not remove.
func tableName(db *gorm.DB, model any) string {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return "history"
	}
	return strings.TrimSpace(stmt.Table)
}
