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
// no cursor starts a new cycle from the oldest expired work, which is safe
// because a pass is idempotent and because pruning is what removes the rows in
// front of it.
//
// A cycle fixes its range when it starts (sweepBound): the newest Job that was due
// then is where the walk ends. That is not an optimization but the difference
// between finishing and not: work this pass may not take — a pinned Job, a Job an
// unresolved claim protects — is skipped and left behind the cursor, and with
// deletions and arrivals on both sides of it a purely positional walk would have
// something ahead of it for ever and never come back. Reaching the end of the
// range ends the cycle and the next pass starts again at the oldest due work,
// which is the next chance for whatever was left behind.
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

	// The cycle's range is fixed here, after the deadlines this pass stamped: a
	// legacy Job that just acquired one is due now, and belongs to the cycle that
	// made it due.
	bound, err := sweepBound(deps.DB, cursor, now)
	if err != nil {
		return result, err
	}

	candidates, err := expiredJobs(deps.DB, cursor, bound, size, now)
	if err != nil {
		return result, err
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
		result.Next = &SweepCursor{FinishedAt: last.FinishedAt.UTC(), ID: last.ID, Bound: bound}
	}
	return result, nil
}

// expiredJobs reads the due Jobs one pass examines: those the cursor has not
// passed, within the range the cycle fixed when it started, oldest first.
// An empty boundary is an empty cycle — nothing was due when it began — and the
// pass therefore examines nothing at all rather than the work that arrived while
// it was reading, which belongs to the cycle after this one.
func expiredJobs(db *gorm.DB, cursor SweepCursor, bound *SweepBound, size int, now time.Time) ([]models.Job, error) {
	if bound == nil {
		return nil, nil
	}
	var candidates []models.Job
	err := db.Model(&models.Job{}).
		Where("state IN ?", terminalStates()).
		Where("finished_at IS NOT NULL AND expires_at IS NOT NULL AND expires_at <= ?", now).
		Where("(jobs.finished_at > ? OR (jobs.finished_at = ? AND jobs.id > ?))", cursor.FinishedAt, cursor.FinishedAt, cursor.ID).
		Where("(jobs.finished_at < ? OR (jobs.finished_at = ? AND jobs.id <= ?))", bound.FinishedAt, bound.FinishedAt, bound.ID).
		Order("finished_at ASC, jobs.id ASC").
		Limit(size).
		Find(&candidates).Error
	if err != nil {
		return nil, fmt.Errorf("jobs: select expired jobs: %w", err)
	}
	return candidates, nil
}

// sweepBound is the upper boundary of the cycle this pass belongs to: the one a
// continued cursor carries, or — for a pass that starts a cycle — the newest Job
// that is due right now.
//
// A pass reads it with the same due-work predicate the candidates are selected
// with rather than with the wall clock, because "due" is a Job's own deadline and
// not an instant: a Job that finished long ago may still have months of its window
// left, and a legacy row gets its deadline in this very pass.
func sweepBound(db *gorm.DB, cursor SweepCursor, now time.Time) (*SweepBound, error) {
	if cursor.Bound != nil {
		return cursor.Bound, nil
	}
	var newest []models.Job
	err := db.Model(&models.Job{}).
		Where("state IN ?", terminalStates()).
		Where("finished_at IS NOT NULL AND expires_at IS NOT NULL AND expires_at <= ?", now).
		Order("finished_at DESC, jobs.id DESC").
		Limit(1).
		Find(&newest).Error
	if err != nil {
		return nil, fmt.Errorf("jobs: select the sweep cycle's boundary: %w", err)
	}
	if len(newest) == 0 {
		// Nothing is due: the cycle has no range and the pass has no candidates.
		return nil, nil
	}
	return &SweepBound{FinishedAt: newest[0].FinishedAt.UTC(), ID: newest[0].ID}, nil
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
// What still holds it back is what is known rather than what is due, and neither
// refusal may become a candidate's standing: a Job an unresolved claim protects,
// because that claim means the process may still be producing the file and §9 says
// no expiry may write through it, and a Kind this process has no adapter for,
// because nobody here can establish that the bytes are gone. The first is excluded
// from the batch itself and re-checked under the Job's own row, and the second is
// deferred, so an artifact nothing can act on waits its turn rather than holding the
// head of every pass. Neither is a refusal to expire an output — the deadline pass
// records that regardless — only a refusal to delete what nobody accounted for.
func (s *Service) cleanupExpiredArtifacts(deps Deps, limit int, now time.Time) (int, error) {
	due, err := dueArtifactsForCleanup(deps.DB, limit, now)
	if err != nil {
		return 0, err
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

		removed, unaccounted, err := s.removeJobArtifacts(deps, job, batch.rows, now)
		if err != nil {
			return recorded, err
		}
		recorded += removed
		if len(unaccounted) > 0 {
			// The bytes are still there and still due. Asking about them again on the
			// very next pass would keep the artifacts behind them from ever being
			// reached, which is what a batch of one makes visible.
			if err := deferArtifactCleanup(deps.DB, unaccounted, now.Add(DefaultArtifactCleanupRetry)); err != nil {
				return recorded, err
			}
		}
	}
	return recorded, nil
}

// dueArtifactsForCleanup reads the expired artifacts a pass may act on: their own
// deadline has passed, no earlier pass has deferred them, and the Job that
// published them is not one an unresolved claim still protects.
//
// The claim predicate is in the selection as well as in the fence below, because a
// candidate the pass can never act on must not be the only thing a bounded batch
// reads: with a batch of one, an older protected artifact was selected on every
// pass forever, and the artifact that came due behind it was never reached. It is
// re-asserted under the Job's own row before anything is deleted, because a claim
// can land between this read and that deletion — the selection decides what to look
// at, and the fence decides what may go.
func dueArtifactsForCleanup(db *gorm.DB, limit int, now time.Time) ([]models.JobOutput, error) {
	var due []models.JobOutput
	err := db.Model(&models.JobOutput{}).
		Where("type = ?", OutputTypeArtifact).
		Where("availability <> ?", string(OutputRemoved)).
		Where("expires_at IS NOT NULL AND expires_at <= ?", now).
		Where("(next_cleanup_at IS NULL OR next_cleanup_at <= ?)", now).
		Where("NOT EXISTS (SELECT 1 FROM job_claims c WHERE c.job_id = job_outputs.job_id AND c.state IN ?)",
			unresolvedClaimStates()).
		Order("COALESCE(next_cleanup_at, expires_at) ASC, id ASC").
		Limit(limit).
		Find(&due).Error
	if err != nil {
		return nil, fmt.Errorf("jobs: read artifacts past their deadline: %w", err)
	}
	return due, nil
}

// deferArtifactCleanup gives artifacts this pass could not establish are gone a
// later instant to be asked about again.
//
// The write is guarded by the row each candidate was selected as — its id and its
// own version — so an artifact published again while the pass ran is not deferred
// under a decision that belonged to the row it replaced; the publication cleared
// any deferral of its own. The rows are left out of the batch until then, which is
// what keeps one candidate nothing can act on from being the head of every pass.
func deferArtifactCleanup(db *gorm.DB, rows []models.JobOutput, until time.Time) error {
	for _, row := range rows {
		// UpdateColumn rather than Update: a deferral is cleanup bookkeeping, not a
		// publication of the row, so it neither moves the output's own version nor
		// rewrites the instant a reader sees as the row's last change.
		result := db.Model(&models.JobOutput{}).
			Where("id = ? AND version = ?", row.ID, row.Version).
			UpdateColumn("next_cleanup_at", until)
		if result.Error != nil {
			return fmt.Errorf("jobs: defer cleanup of artifact %s: %w", row.Key, result.Error)
		}
	}
	return nil
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
	removed, unaccounted, err := s.removeJobArtifacts(deps, job, rows, now)
	if err != nil {
		return false, 0, err
	}
	return len(unaccounted) == 0, removed, nil
}

// removeJobArtifacts puts one Job's artifacts to the Kind that published them,
// records the removals it confirms, and reports which of them it could not account
// for. It is the only place this module deletes artifact bytes, and it runs under
// the Job's own row.
//
// The row is what makes the deletion admission rather than a hope. A candidate is
// selected outside any transaction, and a Job waiting to run can be claimed at any
// instant — a claim is a new execution, and a new execution publishes, the same key
// again among them. A check before the call is therefore not enough: the claim
// commits in the gap and the bytes go underneath it. Taking the Job's row is the
// cheap half of that claim (an UPDATE on SQLite, the row lock on PostgreSQL), and
// re-reading the protection under it is the expensive half: a claim that committed
// while this waited is visible by the time the reading statement starts, so the
// pass stands down instead of deleting. In the other order the claim simply waits
// for the deletion, which is the direction that cannot delete a published file.
//
// The row does not fence the deletion against a claim that has already committed
// and been released, and neither does the protection read: a queued Job can be
// claimed, run, publish the same key — the same artifact produced again, with a
// deadline of its own — and finish, all between the selection and this
// transaction. So the candidates are re-read under that row and the ones the pass
// was not looking at are dropped: the deletion is decided from the rows as they
// are, never from the rows the selection read.
//
// Every answer but a complete accounting is a no, and a Kind this process cannot
// run and an adapter that failed both answer nothing. Neither may be read as
// "gone": the caller keeps whatever names the artifact — the history, where the
// metadata path asked, and the deferral, where the deadline path did.
func (s *Service) removeJobArtifacts(deps Deps, job models.Job, rows []models.JobOutput, now time.Time) (int, []models.JobOutput, error) {
	removed := 0
	var unaccounted []models.JobOutput
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		admitted, err := lockArtifactCleanupTarget(tx, job.ID, now)
		if err != nil {
			return err
		}
		if !admitted {
			// The Job is gone, or an unresolved claim took it between the selection
			// and this transaction: either way nothing of its own may be deleted.
			unaccounted = rows
			return nil
		}

		protected, err := protectedByUnresolvedClaim(tx, job.ID)
		if err != nil {
			return err
		}
		if protected {
			unaccounted = rows
			return nil
		}

		candidates, err := currentArtifacts(tx, job.ID, rows)
		if err != nil {
			return err
		}
		if len(candidates) == 0 {
			// Every candidate was replaced while the pass was running, so there is
			// nothing here to ask the Kind about and nothing to defer: the artifacts
			// that replaced them carry their own deadlines.
			return nil
		}

		removed, unaccounted, err = s.askForArtifactRemoval(tx, job, candidates, now)
		return err
	})
	if err != nil {
		return 0, nil, err
	}
	return removed, unaccounted, nil
}

// currentArtifacts re-reads the candidates a pass selected, under the Job's own row,
// and keeps the ones that are still the rows that pass decided about.
//
// The row's own version is what makes it the same artifact, and it is the whole of
// the check: every column that says what an output promises — its reference, its
// availability, its deadline — is changed only by a write that moves the version
// with it, so a row still at the version the pass read is the row the pass decided
// about, reference, deadline and availability included. A row that is not is one a
// reader would be served differently: the same key produced again with a deadline of
// its own, or one already recorded gone. Handing a reference like that to the Kind
// deletes bytes an output still advertises, and the version guard on the recording
// cannot undo it — it can only decline to record what already happened.
//
// A deferral is the one write to these rows that deliberately moves no version, and
// it is not a reason to skip a candidate here: it says a pass could not establish
// that the bytes are gone, which is the question this pass is answering.
func currentArtifacts(tx *gorm.DB, jobID string, rows []models.JobOutput) ([]models.JobOutput, error) {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}

	var stored []models.JobOutput
	if err := tx.Where("job_id = ? AND id IN ?", jobID, ids).Find(&stored).Error; err != nil {
		return nil, fmt.Errorf("jobs: re-read artifacts before cleanup: %w", err)
	}
	byID := make(map[string]models.JobOutput, len(stored))
	for _, row := range stored {
		byID[row.ID] = row
	}

	current := make([]models.JobOutput, 0, len(rows))
	for _, row := range rows {
		here, found := byID[row.ID]
		if !found || here.Version != row.Version {
			continue
		}
		current = append(current, here)
	}
	return current, nil
}

// lockArtifactCleanupTarget takes the Job's own row before the adapter is asked
// about its artifacts, and reports whether the row was there to take.
//
// The two engines say it differently, exactly as lockPruneTarget does. SQLite has
// no row locks, so the transaction's first statement has to be the write that takes
// the writer lock — the alternative is a read snapshot promoted to a write after
// another connection has committed, which SQLite refuses without running the busy
// handler — and that statement carries the protection predicate as well, so a
// protected Job is not even touched. PostgreSQL has row locks, so the row is taken
// first and the protection re-read as its own statement, which is what gives that
// read a snapshot taken after a claim that was already waiting on the row had
// committed.
func lockArtifactCleanupTarget(tx *gorm.DB, jobID string, now time.Time) (bool, error) {
	if tx.Dialector.Name() == "sqlite" {
		result := tx.Model(&models.Job{}).
			Where("id = ? AND NOT EXISTS (SELECT 1 FROM job_claims c WHERE c.job_id = jobs.id AND c.state IN ?)",
				jobID, unresolvedClaimStates()).
			Update("updated_at", now)
		if result.Error != nil {
			return false, fmt.Errorf("jobs: take job %s for artifact cleanup: %w", jobID, result.Error)
		}
		return result.RowsAffected == 1, nil
	}

	var job models.Job
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", jobID).First(&job).Error
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("jobs: lock job %s for artifact cleanup: %w", jobID, err)
	}
	return true, nil
}

// askForArtifactRemoval asks one Kind to remove one Job's artifacts, inside the
// caller's transaction, and records each removal it acknowledges. It reports how
// many rows it recorded and which artifacts it could not account for.
//
// The caller has taken the Job's row. That is what makes asking and recording one
// decision: the publication of an artifact under this Job waits on that row too, so
// the answer this Kind gives is about the artifacts that are there rather than
// about a set that changed while it was being asked. An adapter's own error text is
// discarded on purpose — it is a failure to answer, not a fact about the bytes.
func (s *Service) askForArtifactRemoval(tx *gorm.DB, job models.Job, rows []models.JobOutput, now time.Time) (int, []models.JobOutput, error) {
	adapter, _, err := s.adapterFor(job.Kind, job.KindVersion)
	if err != nil {
		return 0, rows, nil
	}
	artifacts := make([]ArtifactRef, 0, len(rows))
	for _, row := range rows {
		artifacts = append(artifacts, ArtifactRef{Key: row.Key, Reference: json.RawMessage(copyJSON(row.Reference))})
	}
	result, err := adapter.CleanupArtifacts(context.Background(), ArtifactCleanupRequest{
		JobID: job.ID, Kind: job.Kind, KindVersion: job.KindVersion, Artifacts: artifacts,
	})
	if err != nil {
		return 0, rows, nil
	}
	removed := make(map[string]bool, len(result.Removed))
	for _, key := range result.Removed {
		removed[key] = true
	}

	acknowledged := make([]models.JobOutput, 0, len(rows))
	unaccounted := make([]models.JobOutput, 0, len(rows))
	for _, row := range rows {
		if removed[row.Key] {
			acknowledged = append(acknowledged, row)
			continue
		}
		unaccounted = append(unaccounted, row)
	}

	recorded, err := recordArtifactRemovals(tx, job, acknowledged, now)
	if err != nil {
		return 0, nil, err
	}
	return recorded, unaccounted, nil
}

// recordArtifactRemovals records the artifacts one cleanup acknowledged as gone:
// each output row's own availability, and one Job Event saying the bytes are gone.
//
// Its caller's transaction has taken the Job's own row — a write on SQLite, the row
// lock on PostgreSQL — because the position it allocates is a position on that
// Job's timeline, and store.go's rule is that the count, the maximum and the insert
// are only serialized by holding it. Without it two writers of one timeline read
// the same maximum: on PostgreSQL the second insert waited on the unique index the
// first was about to commit into, and one of the two facts was rolled back — an
// artifact the Kind had confirmed gone, lost along with the record of it. Taking
// that row first is also the order every other writer uses: the prune, the deadline
// pass and the publication all take it before the rows that hang off it. The same
// row is what fences the deletion itself against a claim, which is why it is taken
// before the Kind is asked rather than only before the recording.
//
// Each output write carries that row's own version, so an artifact published again
// while the cleanup ran is not marked with an outcome that belongs to the row it
// replaced, and a row already recorded removed is not recorded twice — which is
// also what keeps a repeated pass over one Job from growing its timeline. The
// recording is this transaction rather than the pruning one on purpose: an artifact
// that is really gone is gone whether or not the history naming it may follow.
func recordArtifactRemovals(tx *gorm.DB, job models.Job, removed []models.JobOutput, now time.Time) (int, error) {
	if len(removed) == 0 {
		return 0, nil
	}
	recorded := 0
	for _, output := range removed {
		result := tx.Model(&models.JobOutput{}).
			Where("id = ? AND version = ? AND availability <> ?",
				output.ID, output.Version, string(OutputRemoved)).
			Updates(map[string]any{
				"availability":    string(OutputRemoved),
				"removed_at":      now,
				"next_cleanup_at": nil,
				"version":         gorm.Expr("version + 1"),
				"updated_at":      now,
			})
		if result.Error != nil {
			return 0, fmt.Errorf("jobs: record artifact removal for %s: %w", output.Key, result.Error)
		}
		if result.RowsAffected == 0 {
			continue
		}
		recorded++

		detail, err := json.Marshal(map[string]string{
			"key": output.Key, "availability": string(OutputRemoved),
		})
		if err != nil {
			return 0, fmt.Errorf("jobs: encode artifact removal detail: %w", err)
		}
		if err := appendEventTx(tx, job, EventInput{Type: EventOutputRemoved, Detail: detail}, now); err != nil {
			return 0, err
		}
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
// the zero value starts a cycle at the oldest expired work, and anything else
// names the position and the cycle's own boundary — a position alone is a walk
// with no end, which is the shape the boundary exists to remove.
func validateSweepCursor(cursor SweepCursor) error {
	empty := cursor.ID == "" && cursor.FinishedAt.IsZero()
	if empty {
		if cursor.Bound != nil {
			return fmt.Errorf("%w: a sweep cycle boundary needs the position it is walked from", ErrInvalidCursor)
		}
		return nil
	}
	if cursor.ID == "" || cursor.FinishedAt.IsZero() {
		return fmt.Errorf("%w: a sweep cursor needs both the finish instant and the job id", ErrInvalidCursor)
	}
	if cursor.Bound == nil {
		return fmt.Errorf("%w: a sweep cursor needs the cycle it belongs to", ErrInvalidCursor)
	}
	if cursor.Bound.ID == "" || cursor.Bound.FinishedAt.IsZero() {
		return fmt.Errorf("%w: a sweep cycle boundary needs both the finish instant and the job id", ErrInvalidCursor)
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
