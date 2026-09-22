package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

	// Four clocks run here, and only the last of them is the Job's metadata
	// deadline.
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

	// The bytes go on the same clock, and for the same reason: §7 makes an
	// artifact's retention its own, and §9 makes a pin exempt Job metadata and Job
	// Events and nothing else. Reaching cleanup from the metadata pass alone meant
	// an artifact promising an hour kept its bytes for the thirty days its history
	// had left — and a pinned Job kept them indefinitely. What still protects an
	// artifact is an unresolved execution claim, never a pin.
	cleanedArtifacts, err := s.cleanupExpiredArtifacts(deps, size, now)
	if err != nil {
		return result, err
	}
	result.Outputs += cleanedArtifacts

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
		// nobody but the Kind that published an artifact can establish it. What the
		// adapter acknowledges as removed is recorded whether or not the whole set
		// went, because an artifact that is really gone is gone whatever happens to
		// the history that names it. A refusal — an artifact still in use, an
		// adapter that cannot answer, a Kind this process has no adapter for at all
		// — keeps the Job, which is the only thing still naming what was left
		// behind.
		accounted, outputs, err := s.accountForArtifacts(deps, candidate, now)
		if err != nil {
			return result, err
		}
		result.Outputs += outputs
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
// and the delete cannot be separated by a pin or a claim landing in between —
// and on an engine with row locks the transaction has taken the Job's own row
// before this predicate is evaluated, so a writer holding it has committed by
// then.
const expiredJobPredicate = `jobs.id = ? AND jobs.state IN ? AND jobs.expires_at IS NOT NULL AND jobs.expires_at <= ?
	AND NOT EXISTS (SELECT 1 FROM job_preferences p WHERE p.job_id = jobs.id AND p.pinned_at IS NOT NULL)
	AND NOT EXISTS (SELECT 1 FROM job_claims c WHERE c.job_id = jobs.id AND c.state IN ?)`

// pruneExpiredJob applies one candidate in its own transaction.
//
// The transaction opens with a write rather than with a read: on SQLite the first
// statement of a write transaction must be the write, or a read snapshot is
// promoted to a write after another connection has committed, which SQLite
// refuses without running the busy handler. So on SQLite the guarded delete is
// first and carries the whole decision, and its row count is the answer — a Job
// that was pinned, claimed or changed underneath the selection matches no row and
// is left where it is. On an engine with row locks the candidate's own row is
// taken first, because the predicates below are evaluated per statement and a
// writer that holds that row without changing it — a pin, which is a viewer's own
// row rather than a fact about the Job — would otherwise stay invisible to the
// statement that decides.
func (s *Service) pruneExpiredJob(deps Deps, candidate models.Job, now time.Time) (bool, int, error) {
	pruned := false
	outputs := 0

	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		locked, err := lockPruneTarget(tx, candidate.ID)
		if err != nil {
			return err
		}
		if !locked {
			// The Job is gone: another pass, or an operator, removed it while this
			// one was reading candidates. There is nothing to decide and nothing to
			// record about a history that no longer exists.
			return nil
		}

		result := tx.Where(expiredJobPredicate,
			candidate.ID, terminalStates(), now, unresolvedClaimStates(),
		).Delete(&models.Job{})
		if result.Error != nil {
			return fmt.Errorf("jobs: prune expired job %s: %w", candidate.ID, result.Error)
		}
		pruned = result.RowsAffected == 1

		// Outputs are recorded before the history that points at them goes: the
		// output rows are a dependent table of the Job's, and the removal the Kind
		// established is recorded on them before the Job's own row is deleted.
		if !pruned {
			// The delete matched nothing: a pin was recorded, a claim landed, or a
			// transition moved the Job while this pass was running. Whichever it
			// was, the Job is not due, and nothing of its own may be written on the
			// way past — an output's own expiry belongs to the deadline pass, which
			// runs for every Job whatever its metadata retention says.
			return nil
		}

		removed, outputErr := markOutputsRemoved(tx, candidate.ID, now)
		if outputErr != nil {
			return outputErr
		}
		outputs = removed

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

// lockPruneTarget takes the candidate Job's own row before the statement that
// decides about it, on an engine that has row locks, and reports whether the row
// was there to take.
//
// The guarded delete's pin and claim predicates are `NOT EXISTS` subqueries, and
// PostgreSQL evaluates them from the snapshot the statement started with. A pin
// transaction takes the Job's row with `SELECT ... FOR UPDATE` and writes only
// its own preference row, so the Job tuple never changes and no recheck follows
// the lock wait: a delete that had already decided would go through on the
// snapshot taken before the pin committed. Waiting for that row here means the
// deciding statement begins after the pin is committed, and its fresh
// READ COMMITTED snapshot sees it.
//
// SQLite has no row locks and serializes writers, so the guarded delete is
// already the first statement of the transaction and is already the whole
// decision; this is inert there.
func lockPruneTarget(tx *gorm.DB, jobID string) (bool, error) {
	if tx.Dialector.Name() == "sqlite" {
		return true, nil
	}
	var job models.Job
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", jobID).First(&job).Error
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("jobs: lock job %s for pruning: %w", jobID, err)
	}
	return true, nil
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

// markOutputsRemoved records that a Job the pass is pruning had outputs and that
// they are gone with it, before the deletes that take the rows away. The artifacts
// among them were already confirmed gone by the Kind that produced them; this covers
// every other output the Job published, which goes with the history naming it.
func markOutputsRemoved(tx *gorm.DB, jobID string, now time.Time) (int, error) {
	result := tx.Model(&models.JobOutput{}).
		Where("job_id = ? AND availability <> ?", jobID, string(OutputRemoved)).
		Updates(map[string]any{
			"availability": string(OutputRemoved),
			"removed_at":   now,
			"version":      gorm.Expr("version + 1"),
			"updated_at":   now,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("jobs: record output removal for %s: %w", jobID, result.Error)
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
// deadline has passed, whatever its Job's state or retention says — and the Job
// Event that says so.
//
// It is bounded and it is per Job: the timeline position an expiry takes is a
// position on that Job's timeline, so the transaction that records it opens with
// the Job's own row — a write on SQLite, the row lock on PostgreSQL — exactly as
// every other writer of that timeline does. Recording the availability and the
// event together is what makes "this artifact expired" a fact a reader can be told
// rather than a clock comparison each reader makes for themselves (§7).
//
// Each row is guarded on the row this pass selected: its id, its version, its
// availability and the deadline that made it due. A publication of the same key —
// the same artifact produced again — replaces that row, and a publication landing
// in the window between the selection and the write used to be expired as the row
// it replaced.
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

	recorded := 0
	for _, batch := range outputsByJob(due) {
		expired, err := expireOutputsOfJob(deps, batch.jobID, batch.rows, now)
		if err != nil {
			return 0, err
		}
		recorded += expired
	}
	return recorded, nil
}

// expireOutputsOfJob records one Job's due expiries, and the event each one
// records, in one transaction.
func expireOutputsOfJob(deps Deps, jobID string, due []models.JobOutput, now time.Time) (int, error) {
	recorded := 0
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		// The first statement is the Job's own row: on SQLite it is the write the
		// transaction is promoted from, and on PostgreSQL it is the lock that
		// serializes this against a publication and against an append, both of
		// which take that row first as well.
		result := tx.Model(&models.Job{}).Where("id = ?", jobID).Update("updated_at", now)
		if result.Error != nil {
			return fmt.Errorf("jobs: touch job %s: %w", jobID, result.Error)
		}
		if result.RowsAffected == 0 {
			// The Job is gone and its outputs went with it: there is no timeline
			// left to record an expiry on.
			return nil
		}

		var job models.Job
		if err := tx.Where("id = ?", jobID).First(&job).Error; err != nil {
			return fmt.Errorf("jobs: read job %s: %w", jobID, err)
		}

		for _, row := range due {
			update := tx.Model(&models.JobOutput{}).
				Where("id = ? AND version = ? AND availability = ?",
					row.ID, row.Version, string(OutputAvailable)).
				Where("expires_at IS NOT NULL AND expires_at <= ?", now).
				Updates(map[string]any{
					"availability": string(OutputExpired),
					"version":      gorm.Expr("version + 1"),
					"updated_at":   now,
				})
			if update.Error != nil {
				return fmt.Errorf("jobs: record output expiry of %s: %w", row.Key, update.Error)
			}
			if update.RowsAffected == 0 {
				// The row this pass selected is not the row that is here any
				// more: a publication replaced it, and carries its own reference
				// and its own deadline.
				continue
			}
			recorded++

			detail, err := json.Marshal(map[string]string{"key": row.Key, "state": string(OutputExpired)})
			if err != nil {
				return fmt.Errorf("jobs: encode output expiry detail: %w", err)
			}
			if err := appendEventTx(tx, job, EventInput{Type: EventOutputExpired, Detail: detail}, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return recorded, nil
}

// cleanupExpiredArtifacts asks each Kind's adapter to remove the artifacts whose
// own deadline has passed, and records every removal the adapter acknowledges.
//
// Artifact retention belongs to the artifact: §7 makes an output's availability
// and retention independent of the Job's outcome, and §9 makes a pin exempt Job
// metadata and Job Events and nothing else. Driving cleanup from the metadata pass
// alone made availability a claim about the Job's history instead — an artifact
// promising an hour kept its bytes for the thirty days that history had left, and
// a pinned Job kept them indefinitely.
//
// Two things still hold it back, and both are about what is known rather than about
// what is due: a Job an unresolved claim protects, because that claim means the
// process may still be producing the file and §9 says no expiry may write through
// it; and a Kind this process has no adapter for, because nobody here can establish
// that the bytes are gone. Neither is a refusal to expire an output — the deadline
// pass records that regardless — only a refusal to delete what nobody accounted
// for.
func (s *Service) cleanupExpiredArtifacts(deps Deps, limit int, now time.Time) (int, error) {
	var due []models.JobOutput
	if err := deps.DB.Model(&models.JobOutput{}).
		Where("type = ?", OutputTypeArtifact).
		Where("availability <> ?", string(OutputRemoved)).
		Where("expires_at IS NOT NULL AND expires_at <= ?", now).
		Order("expires_at ASC, id ASC").Limit(limit).
		Find(&due).Error; err != nil {
		return 0, fmt.Errorf("jobs: read artifacts past their deadline: %w", err)
	}
	if len(due) == 0 {
		return 0, nil
	}

	recorded := 0
	for _, batch := range outputsByJob(due) {
		job, err := loadJob(deps.DB, batch.jobID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// The Job, and with it its outputs, is already gone.
				continue
			}
			return recorded, err
		}
		protected, err := protectedByUnresolvedClaim(deps.DB, job.ID)
		if err != nil {
			return recorded, err
		}
		if protected {
			continue
		}

		_, removed, err := s.askForArtifactRemoval(deps, job, batch.rows, now)
		if err != nil {
			return recorded, err
		}
		recorded += removed
	}
	return recorded, nil
}

// outputBatch is the outputs one pass decided about, grouped by the Job whose
// timeline their changes have to be recorded on.
type outputBatch struct {
	jobID string
	rows  []models.JobOutput
}

// outputsByJob groups a bounded selection by Job, preserving the order the rows
// were selected in — which is the order their deadlines passed.
func outputsByJob(rows []models.JobOutput) []outputBatch {
	batches := make([]outputBatch, 0, len(rows))
	index := make(map[string]int, len(rows))
	for _, row := range rows {
		at, seen := index[row.JobID]
		if !seen {
			at = len(batches)
			index[row.JobID] = at
			batches = append(batches, outputBatch{jobID: row.JobID})
		}
		batches[at].rows = append(batches[at].rows, row)
	}
	return batches
}

// accountForArtifacts asks the Kind's adapter whether every artifact one expired
// Job published is really gone, records each removal the adapter acknowledges,
// and reports whether every one of them is accounted for.
//
// The question is not answerable from the output rows: their availability is what
// this database believes, and the bytes may live anywhere the adapter put them.
// So the answer comes from the only thing that knows — and every answer but a
// complete accounting is a no: an artifact still there, an adapter that failed, a
// Kind this process cannot run at all. An already-missing artifact is a yes,
// because that is one of the outcomes §7 names.
//
// The acknowledgement is per artifact rather than per Job, and it is recorded per
// artifact. A cleanup that removed two of three left two artifacts that are really
// gone, and the Job they belonged to keeps its history because of the third — so
// reducing the answer to one boolean left those two advertised as available for as
// long as that history lived, which for an artifact with no expiry of its own is
// forever. The same loss happens the other way round: when the accounting is
// complete but the metadata cannot be pruned, because a pin landed while the
// cleanup ran.
//
// The call is bounded by the batch it belongs to — one Job's artifacts, at most
// one cleanup per candidate — and runs without a cancellation source because a
// sweep has none. An adapter whose cleanup can take a long time bounds itself.
func (s *Service) accountForArtifacts(deps Deps, job models.Job, now time.Time) (bool, int, error) {
	var rows []models.JobOutput
	if err := deps.DB.Where("job_id = ? AND type = ?", job.ID, OutputTypeArtifact).
		Order("key ASC").Find(&rows).Error; err != nil {
		return false, 0, fmt.Errorf("jobs: read artifacts of %s: %w", job.ID, err)
	}
	if len(rows) == 0 {
		return true, 0, nil
	}
	return s.askForArtifactRemoval(deps, job, rows, now)
}

// askForArtifactRemoval puts one Job's artifacts to the Kind that published them
// and records the removals it confirms. It reports whether every one of them was
// accounted for, and how many rows it recorded.
//
// A Kind this process cannot run and an adapter that failed both answer nothing,
// and neither may be read as "gone": the caller keeps whatever names the artifact.
// An adapter's own error text is discarded on purpose — it is a failure to answer,
// not a fact about the bytes.
func (s *Service) askForArtifactRemoval(deps Deps, job models.Job, rows []models.JobOutput, now time.Time) (bool, int, error) {
	adapter, _, err := s.adapterFor(job.Kind, job.KindVersion)
	if err != nil {
		return false, 0, nil
	}
	artifacts := make([]ArtifactRef, 0, len(rows))
	for _, row := range rows {
		artifacts = append(artifacts, ArtifactRef{Key: row.Key, Reference: json.RawMessage(copyJSON(row.Reference))})
	}
	result, err := adapter.CleanupArtifacts(context.Background(), ArtifactCleanupRequest{
		JobID: job.ID, Kind: job.Kind, KindVersion: job.KindVersion, Artifacts: artifacts,
	})
	if err != nil {
		return false, 0, nil
	}
	removed := make(map[string]bool, len(result.Removed))
	for _, key := range result.Removed {
		removed[key] = true
	}

	accounted := true
	acknowledged := make([]models.JobOutput, 0, len(rows))
	for _, row := range rows {
		if removed[row.Key] {
			acknowledged = append(acknowledged, row)
			continue
		}
		accounted = false
	}

	recorded, err := recordArtifactRemovals(deps, job, acknowledged, now)
	if err != nil {
		return false, 0, err
	}
	return accounted, recorded, nil
}

// recordArtifactRemovals records the artifacts one cleanup acknowledged as gone:
// each output row's own availability, and one Job Event saying the bytes are gone.
//
// The transaction opens with the parent Job's own row — a write on SQLite, the row
// lock on PostgreSQL — because the position it allocates is a position on that
// Job's timeline, and store.go's rule is that the count, the maximum and the insert
// are only serialized by holding it. Without it two writers of one timeline read
// the same maximum: on PostgreSQL the second insert waited on the unique index the
// first was about to commit into, and one of the two facts was rolled back — an
// artifact the Kind had confirmed gone, lost along with the record of it. Taking
// that row first is also the order every other writer uses: the prune, the deadline
// pass and the publication all take it before the rows that hang off it.
//
// Each output write carries that row's own version, so an artifact published again
// while the cleanup ran is not marked with an outcome that belongs to the row it
// replaced, and a row already recorded removed is not recorded twice — which is
// also what keeps a repeated pass over one Job from growing its timeline. The
// transaction is this one rather than the pruning one on purpose: an artifact that
// is really gone is gone whether or not the history naming it may follow.
func recordArtifactRemovals(deps Deps, job models.Job, removed []models.JobOutput, now time.Time) (int, error) {
	if len(removed) == 0 {
		return 0, nil
	}
	recorded := 0
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		// The first statement is a write, and it is on the parent Job's row: it is
		// what takes the writer lock before anything is read on SQLite, and what
		// serializes this against every other writer of the same timeline on
		// PostgreSQL.
		result := tx.Model(&models.Job{}).Where("id = ?", job.ID).Update("updated_at", now)
		if result.Error != nil {
			return fmt.Errorf("jobs: touch job %s: %w", job.ID, result.Error)
		}
		if result.RowsAffected == 0 {
			// The Job is gone, and its outputs went with it: there is no timeline
			// left to record a removal on.
			return nil
		}

		for _, output := range removed {
			result := tx.Model(&models.JobOutput{}).
				Where("id = ? AND version = ? AND availability <> ?",
					output.ID, output.Version, string(OutputRemoved)).
				Updates(map[string]any{
					"availability": string(OutputRemoved),
					"removed_at":   now,
					"version":      gorm.Expr("version + 1"),
					"updated_at":   now,
				})
			if result.Error != nil {
				return fmt.Errorf("jobs: record artifact removal for %s: %w", output.Key, result.Error)
			}
			if result.RowsAffected == 0 {
				continue
			}
			recorded++

			detail, err := json.Marshal(map[string]string{
				"key": output.Key, "availability": string(OutputRemoved),
			})
			if err != nil {
				return fmt.Errorf("jobs: encode artifact removal detail: %w", err)
			}
			if err := appendEventTx(tx, job, EventInput{Type: EventOutputRemoved, Detail: detail}, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return recorded, nil
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
