package application_context

import (
	"errors"

	"gorm.io/gorm"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// storeBulkOverride is BulkOverrideReductionClusters on the tables (design D3):
// check or uncheck many Clusters in one write, with today's request validation and
// the same Changed / NeedsReview / counted semantics.
//
// The transaction begins with the compare-and-set. It then share-locks the member
// Resources of every Cluster it might act on, in global id order and in keyset
// chunks (a no-op on SQLite), and only after that decides visibility and state, so
// an owner move cannot slip between the check and the approval. The exact target
// set is then marked bulk_op = :op in id keyset chunks, with the predicate
// re-checked in the locking statement itself; the counts are read over the marked
// set, the update touches only marked rows, and the same statement clears the
// mark. The counted set and the written set are therefore one set.
func (ctx *MahresourcesContext) storeBulkOverride(r *models.ResourceReduction, request *query_models.ReductionBulkOverride) (*contracts.ReductionBulkResult, error) {
	if r == nil || request == nil || r.ID == 0 {
		return nil, errors.New("no Resource Reduction given")
	}
	if request.Action != ReductionActionCheck && request.Action != ReductionActionUncheck {
		return nil, ErrReductionBulkTarget
	}
	if request.AllIdentical == (len(request.ClusterIDs) > 0) {
		return nil, ErrReductionBulkTarget
	}
	if request.AllIdentical && request.ExpectedCount == nil {
		return nil, ErrReductionBulkNeedsCount
	}
	if len(request.ClusterIDs) > reductionBulkIDCap {
		return nil, ErrReductionBulkTooMany
	}
	if err := reductionEditable(r); err != nil {
		return nil, err
	}
	if request.Version != r.Version {
		return nil, ErrReductionConflict
	}
	if r.PlanGeneration == nil {
		if request.AllIdentical {
			// An empty plan: nothing to act on, and the count the button showed was zero.
			if *request.ExpectedCount != 0 {
				return nil, ErrReductionCountMismatch
			}
			return ctx.emptyBulk(r)
		}
		return nil, ErrReductionClusterNotFound
	}
	generation := *r.PlanGeneration
	checking := request.Action == ReductionActionCheck
	store := ctx.reductionStore()

	var result *contracts.ReductionBulkResult
	err := retryOnLockContention(reductionCASRetries, func() error {
		return store.db.Transaction(func(tx *gorm.DB) error {
			ok, err := casReductionTx(tx, r)
			if err != nil {
				return err
			}
			if !ok {
				return ErrReductionConflict
			}
			st := store.in(tx)
			op := newClaimID()
			if request.AllIdentical {
				if err := st.markAllIdentical(r, generation, op); err != nil {
					return err
				}
			} else if err := st.markNamed(r, generation, op, request.ClusterIDs); err != nil {
				return err
			}
			counts, err := st.countMarked(r, generation, op, checking)
			if err != nil {
				return err
			}
			if request.AllIdentical && uint(counts.counted) != *request.ExpectedCount {
				return ErrReductionCountMismatch
			}
			if err := st.writeMarked(r, generation, op, checking); err != nil {
				return err
			}
			updated, err := reloadReduction(tx, r.ID)
			if err != nil {
				return err
			}
			result = &contracts.ReductionBulkResult{Version: updated.Version, Changed: counts.changed, NeedsReview: counts.needsReview}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// emptyBulk is the answer for a Reduction with no landed plan: the version moves
// (as every decision's does) and nothing changes.
func (ctx *MahresourcesContext) emptyBulk(r *models.ResourceReduction) (*contracts.ReductionBulkResult, error) {
	var result *contracts.ReductionBulkResult
	err := retryOnLockContention(reductionCASRetries, func() error {
		return ctx.reductionStore().db.Transaction(func(tx *gorm.DB) error {
			ok, err := casReductionTx(tx, r)
			if err != nil {
				return err
			}
			if !ok {
				return ErrReductionConflict
			}
			updated, err := reloadReduction(tx, r.ID)
			if err != nil {
				return err
			}
			result = &contracts.ReductionBulkResult{Version: updated.Version}
			return nil
		})
	})
	return result, err
}

// shareLockMembersOf share-locks, in global ascending Resource id and in keyset
// chunks, the Resources that are members of the Clusters `clusterWhere` selects
// (a predicate over reduction_clusters aliased c, in this Reduction and generation).
// Nothing beyond one chunk of ids is held. A no-op on SQLite.
func (s *reductionStore) shareLockMembersOf(r *models.ResourceReduction, generation, clusterWhere string, clusterArgs ...any) error {
	if !s.postgres() {
		return nil
	}
	after := uint(0)
	for {
		args := append([]any{r.ID, generation}, clusterArgs...)
		args = append(args, after, reductionRowChunk)
		var ids []uint
		if err := s.db.Raw(`SELECT m.resource_id FROM reduction_members m JOIN reduction_clusters c ON c.id = m.cluster_id
			WHERE c.reduction_id = ? AND c.generation = ? AND `+clusterWhere+` AND m.resource_id > ?
			GROUP BY m.resource_id ORDER BY m.resource_id LIMIT ?`, args...).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := s.shareLockResources(ids); err != nil {
			return err
		}
		after = ids[len(ids)-1]
		if len(ids) < reductionRowChunk {
			return nil
		}
	}
}

// markClusters locks the Cluster rows among ids (ascending) and marks the ones
// that still satisfy the predicate that makes them a target, returning how many it
// marked. The predicate is evaluated by the marking statement itself, after the
// locks, so a deletion that committed in between is not marked, and so is
// visibility: the share locks on the members' Resources hold off an owner move, but
// not a re-parent of a Group, which can take a Cluster out of the principal's
// subtree after it was selected. A Cluster withheld by the time of this statement
// is not marked.
func (s *reductionStore) markClusters(ids []uint, op string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if err := s.lockClusterRows(ids); err != nil {
		return 0, err
	}
	prefix, args := s.with()
	res := s.db.Exec(prefix+`UPDATE reduction_clusters SET bulk_op = ? WHERE id IN ? AND state = 'open' AND gone_members = 0 AND NOT read_only AND NOT `+s.withheld("reduction_clusters"),
		append(args, op, ids)...)
	return res.RowsAffected, res.Error
}

// markNamed resolves the keys of a bulk-by-ids request and marks them. Visibility
// is decided after the members' Resources are locked, and classification keeps
// today's order: an unknown or withheld key refuses the request as not found
// before any Cluster's state is looked at, then a Cluster that is not open refuses
// it as not open.
func (s *reductionStore) markNamed(r *models.ResourceReduction, generation, op string, keys []string) error {
	seen := map[string]bool{}
	named := make([]string, 0, len(keys))
	for _, k := range keys {
		if !seen[k] {
			seen[k] = true
			named = append(named, k)
		}
	}
	if err := s.shareLockMembersOf(r, generation, `c."key" IN ?`, named); err != nil {
		return err
	}
	afterLocks()
	ids, err := s.classifyNamed(r, generation, named)
	if err != nil {
		return err
	}
	marked, err := s.markClusters(ids, op)
	if err != nil {
		return err
	}
	if int(marked) != len(ids) {
		// One stopped being a target between the classification and the marking
		// statement. With the locks now held, classify again to say which way: a
		// Cluster that left the principal's subtree is not found, a deletion that
		// landed makes it not open.
		if _, err := s.classifyNamed(r, generation, named); err != nil {
			return err
		}
		return ErrReductionClusterNotOpen
	}
	return nil
}

// classifyNamed resolves the keys of a bulk-by-ids request to Cluster ids, in
// today's order: an unknown or withheld key refuses the request as not found before
// any Cluster's state is looked at, then a Cluster that is not open refuses it as
// not open.
func (s *reductionStore) classifyNamed(r *models.ResourceReduction, generation string, named []string) ([]uint, error) {
	prefix, where, args := s.base(r, generation)
	var rows []struct {
		ID          uint
		Key         string
		State       string
		GoneMembers int
		ReadOnly    bool
	}
	if err := s.db.Raw(prefix+`SELECT c.id, c."key", c.state, c.gone_members, c.read_only FROM reduction_clusters c WHERE `+where+` AND c."key" IN ?`,
		append(args, named)...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != len(named) {
		return nil, ErrReductionClusterNotFound
	}
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		if row.State != models.ReductionClusterOpen || row.GoneMembers > 0 || row.ReadOnly {
			return nil, ErrReductionClusterNotOpen
		}
		ids = append(ids, row.ID)
	}
	return ids, nil
}

// markAllIdentical marks every open Identical Cluster the principal may fully see.
// The structural superset (every open Identical Cluster of the generation, before
// any visibility test) has its member Resources locked first, so a Cluster that
// becomes visible later is already locked.
func (s *reductionStore) markAllIdentical(r *models.ResourceReduction, generation, op string) error {
	if err := s.shareLockMembersOf(r, generation, `c.tier = 'identical' AND c.state = 'open'`); err != nil {
		return err
	}
	afterLocks()
	after := uint(0)
	for {
		var ids []uint
		sql, args := s.identicalCandidates(r, generation, after)
		if err := s.db.Raw(sql, args...).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if _, err := s.markClusters(ids, op); err != nil {
			return err
		}
		after = ids[len(ids)-1]
		if len(ids) < reductionRowChunk {
			return nil
		}
	}
}

// identicalCandidates is one keyset chunk of the open Identical Clusters the
// principal may fully see, after the id `after`. The bulk_op = ” equality is what
// lets idx_rc_bulk (reduction, generation, bulk_op, id) serve the id order: every
// unmarked Cluster of the generation is walked once across all chunks, never a sort
// of the whole open set per chunk. Nothing else is marked while this runs: the
// Reduction row's compare-and-set serializes bulk writers.
func (s *reductionStore) identicalCandidates(r *models.ResourceReduction, generation string, after uint) (string, []any) {
	prefix, where, args := s.base(r, generation)
	return prefix + `SELECT c.id FROM reduction_clusters c WHERE ` + where + ` AND c.bulk_op = '' AND c.tier = 'identical' AND ` + actionable("c") +
			` AND c.id > ? ORDER BY c.id LIMIT ?`,
		append(append([]any(nil), args...), after, reductionRowChunk)
}

type bulkCounts struct {
	// counted is what a click on the button would change (the expectedCount's set).
	counted     int
	changed     int
	needsReview int
}

const markedChunkSQL = `SELECT MAX(id) FROM (SELECT id FROM reduction_clusters WHERE reduction_id = ? AND generation = ? AND bulk_op = ? AND id > ? ORDER BY id LIMIT ?) chunk`

// markedChunks walks the Clusters marked bulk_op = op by id keyset and hands each
// chunk's id range (lo, hi] to fn.
func (s *reductionStore) markedChunks(r *models.ResourceReduction, generation, op string, fn func(lo, hi uint) error) error {
	lo := uint(0)
	for {
		var hi *uint
		if err := s.db.Raw(markedChunkSQL, r.ID, generation, op, lo, reductionRowChunk).Scan(&hi).Error; err != nil {
			return err
		}
		if hi == nil {
			return nil
		}
		if err := fn(lo, *hi); err != nil {
			return err
		}
		lo = *hi
	}
}

// countMarked reads today's counts over the marked set: what the button would
// change, how many Clusters flipped, how many a check left alone for review.
func (s *reductionStore) countMarked(r *models.ResourceReduction, generation, op string, checking bool) (bulkCounts, error) {
	var out bulkCounts
	err := s.markedChunks(r, generation, op, func(lo, hi uint) error {
		var row struct{ N, Counted, Changed, NeedsReview int64 }
		var sql string
		args := []any{r.ID, generation, op, lo, hi}
		if checking {
			sql = `SELECT COUNT(*) AS n,
				COALESCE(SUM(CASE WHEN NOT checked AND NOT needs_attention AND loser_count > 0 THEN 1 ELSE 0 END), 0) AS counted,
				COALESCE(SUM(CASE WHEN NOT checked AND NOT needs_attention AND loser_count > 0 THEN 1 ELSE 0 END), 0) AS changed,
				COALESCE(SUM(CASE WHEN needs_attention AND loser_count > 0 THEN 1 ELSE 0 END), 0) AS needs_review`
		} else {
			sql = `SELECT COUNT(*) AS n, COUNT(*) AS counted,
				COALESCE(SUM(CASE WHEN checked THEN 1 ELSE 0 END), 0) AS changed, 0 AS needs_review`
		}
		if err := s.db.Raw(sql+` FROM reduction_clusters WHERE reduction_id = ? AND generation = ? AND bulk_op = ? AND id > ? AND id <= ?`, args...).Scan(&row).Error; err != nil {
			return err
		}
		out.counted += int(row.Counted)
		out.changed += int(row.Changed)
		out.needsReview += int(row.NeedsReview)
		return nil
	})
	return out, err
}

// writeMarked applies the decision to the marked rows and clears the mark in the
// same statement, chunk by chunk. A check leaves alone a Cluster with nothing to
// remove or one that needs attention (it stays exactly as it was); everything else
// marked is set to the decision and frozen as reviewed.
func (s *reductionStore) writeMarked(r *models.ResourceReduction, generation, op string, checking bool) error {
	return s.markedChunks(r, generation, op, func(lo, hi uint) error {
		return s.db.Exec(writeMarkedSQL, checking, checking, checking, r.ID, generation, op, lo, hi).Error
	})
}

// writeMarkedSQL: the parameters are the decision three times (the writable test
// twice and the value), then the chunk (reduction, generation, op, lo, hi).
const writeMarkedSQL = `UPDATE reduction_clusters SET
		checked = CASE WHEN (NOT ? OR (loser_count > 0 AND NOT needs_attention)) THEN ? ELSE checked END,
		reviewed = CASE WHEN (NOT ? OR (loser_count > 0 AND NOT needs_attention)) THEN TRUE ELSE reviewed END,
		bulk_op = ''
	WHERE reduction_id = ? AND generation = ? AND bulk_op = ? AND id > ? AND id <= ?`
