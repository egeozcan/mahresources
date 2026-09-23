package jobs

import (
	"fmt"

	"mahresources/models"

	"gorm.io/gorm"
)

// This file holds the one bounded walk of a Job's lineage that answers "what does this
// work still depend on?".
//
// Two callers need it and neither can see the other's tables. Retention needs it because
// a finished Job's row is what carries the *handles* a later stage reads: an import
// apply's plan and archive are named by its parse's handle, which the walk finds two hops
// above it, so pruning that parse while the apply is still queued deletes the input of
// work that has not run. The application's startup sweep needs it for the same reason,
// from the other side: it protects the staged bytes those handles name.
//
// The relations it follows are the ones a staged hand-off travels:
//
//   - parent/child, where the parent's handle names the archive and plan a child
//     workflow reads;
//   - retry-of and repeat-of, where a successor still names its ancestor's inputs — a
//     Retry copies the ancestor's sealed input, so what that input names is as much the
//     successor's dependency as the ancestor's.
//
// It is bounded in hops rather than trusting the table: lineage is data, and a corrupted
// or hand-edited link table must not turn a retention pass into an unbounded traversal.
// Reaching the bound stops the walk rather than failing it, which errs toward keeping
// files and rows alive slightly longer than they had to be.

// MaxStagingLineageHops bounds the walk above. The relations a staged name travels are
// one or two links deep in every shape this tree has, so this is a ceiling on a corrupt
// table rather than a limit on a real one.
const MaxStagingLineageHops = 32

// LineageAncestors returns every Job above one set of Jobs, over the links a staged
// hand-off travels, excluding the Jobs the walk started from.
//
// The caller supplies the handle: it may be a transaction, and the walk must read what
// that transaction can see.
func LineageAncestors(db *gorm.DB, ids []string, maxHops int) ([]string, error) {
	if db == nil || len(ids) == 0 {
		return nil, nil
	}
	if maxHops <= 0 {
		maxHops = MaxStagingLineageHops
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	frontier := append([]string(nil), ids...)
	ancestors := make([]string, 0, len(ids))

	for hop := 0; hop < maxHops && len(frontier) > 0; hop++ {
		var parents []string
		if err := db.Model(&models.JobLink{}).
			Where("type = ? AND to_job_id IN ?", string(LinkParentChild), frontier).
			Distinct().Pluck("from_job_id", &parents).Error; err != nil {
			return nil, fmt.Errorf("jobs: read lineage parents: %w", err)
		}
		var prior []string
		if err := db.Model(&models.JobLink{}).
			Where("type IN ? AND from_job_id IN ?",
				[]string{string(LinkRetryOf), string(LinkRepeatOf)}, frontier).
			Distinct().Pluck("to_job_id", &prior).Error; err != nil {
			return nil, fmt.Errorf("jobs: read lineage ancestors: %w", err)
		}
		parents = append(parents, prior...)

		frontier = frontier[:0]
		for _, id := range parents {
			if seen[id] {
				continue
			}
			seen[id] = true
			ancestors = append(ancestors, id)
			frontier = append(frontier, id)
		}
	}
	return ancestors, nil
}

// WorkStillDependsOnJob reports whether any nonterminal Job is one this Job's own
// lineage is an ancestor of.
//
// It is the retention question written as a fact about the *candidate*: a finished Job
// is not history while a live Job still names what it staged. The walk goes downward from
// the candidate, over both the relations above reversed, and it is bounded by the hop
// ceiling for the same reason the upward walk is.
func WorkStillDependsOnJob(db *gorm.DB, jobID string, maxHops int) (bool, error) {
	if db == nil || jobID == "" {
		return false, nil
	}
	if maxHops <= 0 {
		maxHops = MaxStagingLineageHops
	}
	seen := map[string]bool{jobID: true}
	frontier := []string{jobID}

	for hop := 0; hop < maxHops && len(frontier) > 0; hop++ {
		var children []string
		if err := db.Model(&models.JobLink{}).
			Where("type = ? AND from_job_id IN ?", string(LinkParentChild), frontier).
			Distinct().Pluck("to_job_id", &children).Error; err != nil {
			return false, fmt.Errorf("jobs: read lineage children: %w", err)
		}
		var successors []string
		if err := db.Model(&models.JobLink{}).
			Where("type IN ? AND to_job_id IN ?",
				[]string{string(LinkRetryOf), string(LinkRepeatOf)}, frontier).
			Distinct().Pluck("from_job_id", &successors).Error; err != nil {
			return false, fmt.Errorf("jobs: read lineage successors: %w", err)
		}
		children = append(children, successors...)

		next := make([]string, 0, len(children))
		for _, id := range children {
			if seen[id] {
				continue
			}
			seen[id] = true
			next = append(next, id)
		}
		if len(next) == 0 {
			return false, nil
		}
		var live int64
		if err := db.Model(&models.Job{}).
			Where("id IN ? AND state NOT IN ?", next, terminalStates()).
			Count(&live).Error; err != nil {
			return false, fmt.Errorf("jobs: read dependents of %s: %w", jobID, err)
		}
		if live > 0 {
			return true, nil
		}
		frontier = next
	}
	return false, nil
}
