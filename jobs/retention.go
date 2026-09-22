package jobs

import (
	"context"
	"encoding/json"
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

	// Three clocks run here, and none of them is the Job's metadata deadline.
	//
	// Replay retention runs from terminal completion: an envelope whose window
	// passed is purged whether or not the Job's metadata is due, and a Job whose
	// metadata goes takes its envelope row with it.
	envelopes, err := s.PurgeExpiredReplay(deps, size)
	if err != nil {
		return result, err
	}
	result.Envelopes = envelopes

	// Output deadlines run from publication: an artifact that expires in an hour
	// is gone in an hour, whether its Job finished a minute ago, has a month of
	// history left, or is still running. Leaving that to the metadata pass made
	// "available" a claim about the Job's retention rather than about the
	// artifact.
	expiredOutputs, err := expireOutputsOnTheirDeadline(deps, size, now)
	if err != nil {
		return result, err
	}
	result.Outputs += expiredOutputs

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
		// A Job retention may not take at all is decided before anything is asked
		// of its artifacts: a pin or an unresolved claim keeps the history, and
		// cleaning up what that history points at would destroy the only thing
		// naming the artifact while leaving the record that named it.
		protected, err := sweepProtected(deps.DB, candidate.ID)
		if err != nil {
			return result, err
		}
		if protected {
			result.Skipped++
			continue
		}

		// The artifacts go next, and the answer has to be yes before the history
		// that points at them may: §9 requires the removal to be established, and
		// nobody but the Kind that published an artifact can establish it. A
		// refusal — an artifact still in use, an adapter that cannot answer, a
		// Kind this process has no adapter for at all — keeps the Job, which is
		// the only thing still naming what was left behind.
		accounted, err := s.artifactsAccountedFor(deps, candidate)
		if err != nil {
			return result, err
		}
		if !accounted {
			result.Skipped++
			continue
		}

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

// sweepProtected reports whether a candidate Job is one ordinary retention may
// not take: a Job somebody pinned, or one an unresolved claim still protects.
//
// It is asked before the sweep destroys anything belonging to the Job, and it is
// asked again — as the predicate of the deleting statement — before the Job's own
// row goes, because a pin or a claim landing in between is exactly what the
// guarded delete exists to catch.
func sweepProtected(db *gorm.DB, jobID string) (bool, error) {
	var pinned int64
	err := db.Model(&models.JobPreference{}).
		Where("job_id = ? AND pinned_at IS NOT NULL", jobID).Count(&pinned).Error
	if err != nil {
		return false, fmt.Errorf("jobs: read pins for %s: %w", jobID, err)
	}
	if pinned > 0 {
		return true, nil
	}
	return protectedByUnresolvedClaim(db, jobID)
}

// expireOutputsOnTheirDeadline records the expiry of every output whose own
// deadline has passed, whatever its Job's state or retention says.
//
// It is one bounded statement pair rather than a per-Job walk: the rows it is
// about are the ones an executor promised a viewer, and the sweep is the only
// sane place to turn "past its deadline" into the recorded fact that every reader
// agrees about. The update is guarded on the availability it is replacing, so two
// sweeps racing each other record one expiry once.
func expireOutputsOnTheirDeadline(deps Deps, limit int, now time.Time) (int, error) {
	var due []models.JobOutput
	if err := deps.DB.Model(&models.JobOutput{}).
		Where("availability = ?", string(OutputAvailable)).
		Where("expires_at IS NOT NULL AND expires_at <= ?", now).
		Order("expires_at ASC, id ASC").Limit(limit).
		Find(&due).Error; err != nil {
		return 0, fmt.Errorf("jobs: read outputs past their deadline: %w", err)
	}
	if len(due) == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(due))
	for _, row := range due {
		ids = append(ids, row.ID)
	}

	result := deps.DB.Model(&models.JobOutput{}).
		Where("id IN ? AND availability = ?", ids, string(OutputAvailable)).
		Updates(map[string]any{
			"availability": string(OutputExpired),
			"version":      gorm.Expr("version + 1"),
			"updated_at":   now,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("jobs: record output expiry: %w", result.Error)
	}
	return int(result.RowsAffected), nil
}

// artifactsAccountedFor asks the Kind's adapter whether every artifact one expired
// Job published is really gone, and reports whether the history that names them
// may be pruned.
//
// The question is not answerable from the output rows: their availability is what
// this database believes, and the bytes may live anywhere the adapter put them.
// So the answer comes from the only thing that knows — and every answer but a
// complete accounting is a no: an artifact still there, an adapter that failed, a
// Kind this process cannot run at all. An already-missing artifact is a yes,
// because that is one of the outcomes §7 names.
//
// The call is bounded by the batch it belongs to — one Job's artifacts, at most
// one cleanup per candidate — and runs without a cancellation source because a
// sweep has none. An adapter whose cleanup can take a long time bounds itself.
func (s *Service) artifactsAccountedFor(deps Deps, job models.Job) (bool, error) {
	var rows []models.JobOutput
	if err := deps.DB.Where("job_id = ? AND type = ?", job.ID, OutputTypeArtifact).
		Order("key ASC").Find(&rows).Error; err != nil {
		return false, fmt.Errorf("jobs: read artifacts of %s: %w", job.ID, err)
	}
	if len(rows) == 0 {
		return true, nil
	}

	adapter, _, err := s.adapterFor(job.Kind, job.KindVersion)
	if err != nil {
		return false, nil
	}
	artifacts := make([]ArtifactRef, 0, len(rows))
	for _, row := range rows {
		artifacts = append(artifacts, ArtifactRef{Key: row.Key, Reference: json.RawMessage(copyJSON(row.Reference))})
	}
	result, err := adapter.CleanupArtifacts(context.Background(), ArtifactCleanupRequest{
		JobID: job.ID, Kind: job.Kind, KindVersion: job.KindVersion, Artifacts: artifacts,
	})
	if err != nil {
		return false, nil
	}
	removed := make(map[string]bool, len(result.Removed))
	for _, key := range result.Removed {
		removed[key] = true
	}
	for _, artifact := range artifacts {
		if !removed[artifact.Key] {
			return false, nil
		}
	}
	return true, nil
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
