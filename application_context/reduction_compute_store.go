package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/types"
)

// The table-backed landing of a computed plan (design D4), beside
// storeReductionPlan. The Clusters are still built in memory as today; what changes
// is where they land: a new generation of rows, written under a fence that proves
// the compute still owns the Reduction, with the Clusters a compute carries over
// (applied, frozen by a decision, read-only history) copied by INSERT...SELECT in
// bounded chunks and the fresh ones inserted one transaction apiece. Nothing calls
// these yet.

// ---- begin, admit, fail ----------------------------------------------------------------

// beginReductionComputeStore claims a Reduction for a compute: one conditional UPDATE
// that sets status computing, a fresh compute generation, and a pending reservation
// (the job may wait for a queue slot; the active lease starts when a worker admits
// it). It is refused while a compute is live, while an apply or a settle is running
// (so queued and replayed starts are refused too), while the Reduction still holds a
// legacy JSON plan, and at a version other than the caller's. It returns the nonce
// the worker later swaps for its job id, and the generation.
func (ctx *MahresourcesContext) beginReductionComputeStore(reductionID, version uint) (jobNonce, generation string, err error) {
	return ctx.beginReductionCompute(reductionID, version, nil, false)
}

// beginReductionComputeStoreAs is the claim a caller that loaded the Reduction makes:
// the CAS also carries the owner it loaded, so an owner deleted since (created_by_user_id
// nulled, no version bump) no longer matches (design D2).
func (ctx *MahresourcesContext) beginReductionComputeStoreAs(reductionID, version uint, owner *uint) (jobNonce, generation string, err error) {
	return ctx.beginReductionCompute(reductionID, version, owner, true)
}

// ownerPredicate is the SQL that pins created_by_user_id to the loaded owner, NULL-safe.
func ownerPredicate(owner *uint) (string, []any) {
	if owner == nil {
		return " AND created_by_user_id IS NULL", nil
	}
	return " AND created_by_user_id = ?", []any{*owner}
}

func (ctx *MahresourcesContext) beginReductionCompute(reductionID, version uint, owner *uint, checkOwner bool) (jobNonce, generation string, err error) {
	s := ctx.reductionStore()
	now := time.Now()
	jobNonce = "pending:" + string(types.NewUUIDv7())
	generation = string(types.NewUUIDv7())
	err = retryOnLockContention(reductionCASRetries, func() error {
		return s.db.Transaction(func(tx *gorm.DB) error {
			ownerSQL, ownerArgs := "", []any(nil)
			if checkOwner {
				ownerSQL, ownerArgs = ownerPredicate(owner)
			}
			args := append([]any{now, now.Add(ReductionComputeDeadline), jobNonce, generation, now.Add(ReductionComputeDeadline).UnixNano(), now,
				reductionID, version, now.UnixNano(), now.UnixNano(), now.UnixNano()}, ownerArgs...)
			res := tx.Exec(`UPDATE resource_reductions SET status = 'computing', computing_started_at = ?, compute_deadline = ?, compute_job_id = ?,
					compute_error = '', compute_generation = ?, compute_lease_until = ?, version = version + 1, updated_at = ?
				WHERE id = ? AND version = ? AND plan_schema = 1 AND apply_lease_until <= ? AND settle_lease_until <= ?
					AND (status <> 'computing' OR compute_lease_until <= ?)`+ownerSQL, args...)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 1 {
				return nil
			}
			current, err := reloadReduction(tx, reductionID)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrReductionNotFound
			}
			if err != nil {
				return err
			}
			switch {
			case current.PlanSchema == 0:
				return ErrReductionBusy
			case current.Status == models.ReductionStatusComputing && current.ComputeLeaseUntil > now.UnixNano():
				return ErrReductionBusy
			case current.ApplyLeaseUntil > now.UnixNano(), current.SettleLeaseUntil > now.UnixNano():
				return ErrReductionConflict
			}
			return ErrReductionConflict
		})
	})
	if err != nil {
		return "", "", err
	}
	return jobNonce, generation, nil
}

// storeAdmitComputeLease starts the active lease when a worker takes the job, under
// the generation fence: a generation that was replaced or whose reservation ran out
// is refused.
func (ctx *MahresourcesContext) storeAdmitComputeLease(reductionID uint, generation string) error {
	s := ctx.reductionStore()
	now := time.Now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := fenceComputeIn(tx, reductionID, generation, now); err != nil {
			return err
		}
		var row struct{ ComputingStartedAt *time.Time }
		if err := tx.Raw(`SELECT computing_started_at FROM resource_reductions WHERE id = ?`, reductionID).Scan(&row).Error; err != nil {
			return err
		}
		started := now
		if row.ComputingStartedAt != nil {
			started = *row.ComputingStartedAt
		}
		until := leaseDeadline(afterLock(now), started)
		return tx.Exec(`UPDATE resource_reductions SET compute_lease_until = ?, compute_deadline = ? WHERE id = ? AND compute_generation = ?`,
			until, time.Unix(0, until), reductionID, generation).Error
	})
}

// storeFailCompute records a compute that ended without landing: failed, with the
// message, and the generation released so the sweep may remove what it staged. A
// compute that was superseded (another generation now owns the row) records nothing.
func (ctx *MahresourcesContext) storeFailCompute(reductionID uint, jobID, generation string, cause error) error {
	if err := ctx.recordComputeFailure(reductionID, jobID, generation, cause); err != nil {
		return err
	}
	// The generation is released: remove what it staged now rather than leave it for
	// the next sweep, so repeated failures do not accumulate O(library) rows. The
	// sweep's guard keeps whatever another compute or the plan owns, and a failure
	// here only defers the removal to the next compute start.
	if err := ctx.sweepReductionGenerations(); err != nil {
		ctx.logReductionWriteFailure(reductionID, "sweep after a failed compute", err)
	}
	return nil
}

func (ctx *MahresourcesContext) recordComputeFailure(reductionID uint, jobID, generation string, cause error) error {
	s := ctx.reductionStore()
	return retryOnLockContention(reductionCASRetries, func() error {
		return s.db.Exec(`UPDATE resource_reductions SET status = 'failed', computing_started_at = NULL, compute_deadline = NULL, compute_error = ?,
				compute_generation = NULL, compute_lease_until = 0, version = version + 1, updated_at = ?
			WHERE id = ? AND status = 'computing' AND compute_generation = ? AND (? = '' OR compute_job_id = ?)`,
			truncateRunes(cause.Error(), 2000), time.Now(), reductionID, generation, jobID, jobID).Error
	})
}

// ---- landing -----------------------------------------------------------------------------

// reductionLandHook runs after the carried Clusters are staged and before the first
// fresh one. Only a test sets it, to move the Reduction under a compute that is
// part-way through landing.
var reductionLandHook func()

func (ctx *MahresourcesContext) landComputedPlan(r *models.ResourceReduction, jobID, generation string, version uint, fresh []*models.ReductionCluster, coverage models.ReductionCoverage, settings *models.ReductionSettings, carry bool) error {
	s := ctx.reductionStore()
	if err := s.clearGeneration(r.ID, generation); err != nil {
		return err
	}
	if carry {
		if err := s.carryOver(r.ID, generation); err != nil {
			return err
		}
	}
	if reductionLandHook != nil {
		reductionLandHook()
	}
	if err := s.insertFreshClusters(r, generation, fresh); err != nil {
		return err
	}
	return ctx.finishLandingOwned(nil, r, jobID, generation, version, coverage, settings, false)
}

// finishLanding is the landing transaction and the cleanup after it: the
// generation's staging rows are deleted and stale generations swept.
func (ctx *MahresourcesContext) finishLanding(r *models.ResourceReduction, jobID, generation string, version uint, coverage models.ReductionCoverage, settings *models.ReductionSettings) error {
	return ctx.finishLandingOwned(nil, r, jobID, generation, version, coverage, settings, true)
}

// finishLandingOwned is finishLanding with the owner predicate optional: a caller
// that holds only the Reduction's id (the test seams) cannot state an owner.
func (ctx *MahresourcesContext) finishLandingOwned(done context.Context, r *models.ResourceReduction, jobID, generation string, version uint, coverage models.ReductionCoverage, settings *models.ReductionSettings, checkOwner bool) error {
	if err := ctx.landGenerationOwned(done, r.ID, r.CreatedByUserId, checkOwner, jobID, generation, version, coverage, settings); err != nil {
		return err
	}
	if err := ctx.reductionStore().clearStaging(r.ID, generation); err != nil {
		ctx.logReductionWriteFailure(r.ID, "staging "+generation, err)
	}
	if err := ctx.sweepReductionGenerations(); err != nil {
		ctx.logReductionWriteFailure(r.ID, "sweep", err)
	}
	return nil
}

// landAfterLockHook runs in the landing transaction once it holds the Reduction row.
// A test seam: nil outside tests.
var landAfterLockHook func()

// landGeneration is the landing transaction.
func (ctx *MahresourcesContext) landGeneration(reductionID uint, jobID, generation string, version uint, coverage models.ReductionCoverage, settings *models.ReductionSettings) error {
	return ctx.landGenerationOwned(nil, reductionID, nil, false, jobID, generation, version, coverage, settings)
}

// landGenerationOwned also compares the owner the compute loaded with the row's under
// the row lock, so an owner deleted mid-compute (nulled, no version bump) no longer
// authorizes the landing (design D2).
func (ctx *MahresourcesContext) landGenerationOwned(done context.Context, reductionID uint, owner *uint, checkOwner bool, jobID, generation string, version uint, coverage models.ReductionCoverage, settings *models.ReductionSettings) error {
	s := ctx.reductionStore().boundTo(done)
	coverageJSON, err := json.Marshal(coverage)
	if err != nil {
		return err
	}
	var settingsJSON any
	if settings != nil {
		raw, err := json.Marshal(settings)
		if err != nil {
			return err
		}
		settingsJSON = string(raw)
	}
	now := time.Now()
	return retryOnLockContention(reductionCASRetries, func() error {
		return s.db.Transaction(func(tx *gorm.DB) error {
			if err := lockReductionRowTx(tx, reductionID); err != nil {
				if done != nil && done.Err() != nil {
					return done.Err()
				}
				return ErrReductionComputeSuperseded
			}
			if landAfterLockHook != nil {
				landAfterLockHook()
			}
			// A cancel that arrived while this waited for the row publishes nothing.
			if done != nil && done.Err() != nil {
				return done.Err()
			}
			var row struct {
				Status            string
				ComputeGeneration *string
				ComputeJobID      string
				Version           uint
				ComputeLeaseUntil int64
				CreatedByUserID   *uint `gorm:"column:created_by_user_id"`
			}
			if err := tx.Raw(`SELECT status, compute_generation, compute_job_id, version, compute_lease_until, created_by_user_id FROM resource_reductions WHERE id = ?`, reductionID).Scan(&row).Error; err != nil {
				return err
			}
			if row.Status != models.ReductionStatusComputing || row.ComputeGeneration == nil || *row.ComputeGeneration != generation ||
				(jobID != "" && row.ComputeJobID != jobID) {
				return ErrReductionComputeSuperseded
			}
			// Refused, not rebased: the plan describes the Extent, the Winner Rule and
			// the decisions as they were when the run began.
			if row.Version != version {
				return ErrReductionStaleCompute
			}
			if checkOwner && !sameOwner(owner, row.CreatedByUserID) {
				return ErrReductionConflict
			}
			if row.ComputeLeaseUntil <= afterLock(now).UnixNano() {
				return ErrReductionLeaseLost
			}
			return tx.Exec(`UPDATE resource_reductions SET plan_generation = ?, compute_generation = NULL, status = 'ready', computed_at = ?,
					computing_started_at = NULL, compute_deadline = NULL, compute_error = '', compute_lease_until = 0,
					coverage = ?, plan_settings = ?, version = version + 1, updated_at = ? WHERE id = ?`,
				generation, now, string(coverageJSON), settingsJSON, now, reductionID).Error
		})
	})
}

// ---- staging steps, each transaction fenced ---------------------------------------------------

// fenced runs fn in a transaction that begins with the compute fence, retried while
// it loses to lock contention.
func (s *reductionStore) fenced(reductionID uint, generation string, fn func(tx *gorm.DB) error) error {
	return retryOnLockContention(reductionCASRetries, func() error {
		return s.db.Transaction(func(tx *gorm.DB) error {
			if err := fenceComputeIn(tx, reductionID, generation, time.Now()); err != nil {
				return err
			}
			return fn(tx)
		})
	})
}

// clearGeneration deletes an earlier attempt's rows of this generation, members first,
// in id-keyset chunks, each fenced.
func (s *reductionStore) clearGeneration(reductionID uint, generation string) error {
	for _, table := range []string{"reduction_members", "reduction_clusters"} {
		for {
			var n int64
			err := s.fenced(reductionID, generation, func(tx *gorm.DB) error {
				var ids []uint
				if err := tx.Raw(`SELECT id FROM `+table+` WHERE reduction_id = ? AND generation = ? ORDER BY id LIMIT ?`, reductionID, generation, reductionRowChunk).Scan(&ids).Error; err != nil {
					return err
				}
				n = int64(len(ids))
				if n == 0 {
					return nil
				}
				return tx.Exec(`DELETE FROM `+table+` WHERE id IN ?`, ids).Error
			})
			if err != nil {
				return err
			}
			if n == 0 {
				break
			}
		}
	}
	return nil
}

// carryPredicate is the Clusters a compute carries over: applied ones, ones frozen by
// a decision (reviewed, skipped, stale) and read-only history.
const carryPredicate = `(c.state = 'applied' OR c.read_only OR c.reviewed OR c.state IN ('skipped', 'stale'))`

// carryClusterWindowSQL reads the next window of carried Cluster ids of the plan
// generation; carryMemberPageSQL pages the members of one window by (cluster,
// position) on idx_rm_page. Named so a test can EXPLAIN exactly what runs.
const (
	carryClusterWindowSQL = `SELECT c.id FROM reduction_clusters c WHERE c.reduction_id = ? AND c.generation = ? AND ` + carryPredicate + ` AND c.id > ? ORDER BY c.id LIMIT ?`
	carryMemberPageSQL    = `SELECT m.id, m.cluster_id, m.position, m.resource_id FROM reduction_members m
		WHERE m.cluster_id IN ? AND (m.cluster_id > ? OR (m.cluster_id = ? AND m.position > ?)) ORDER BY m.cluster_id, m.position LIMIT ?`
)

// columnList is a model's column names but the ones skipped, quoted.
func columnList(db *gorm.DB, model any, skip ...string) ([]string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return nil, err
	}
	skipped := map[string]bool{}
	for _, name := range skip {
		skipped[name] = true
	}
	var cols []string
	for _, f := range stmt.Schema.Fields {
		if f.DBName == "" || skipped[f.DBName] {
			continue
		}
		cols = append(cols, `"`+f.DBName+`"`)
	}
	return cols, nil
}

func prefixed(alias string, cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = alias + "." + c
	}
	return out
}

// carryOverChunkHook runs inside each carry-over chunk's transaction, before its
// insert, with the phase ("clusters" or "members"). A test seam: nil outside tests.
var carryOverChunkHook func(tx *gorm.DB, phase string) error

// errCarryIncomplete says the new generation does not hold every carried Cluster
// and member of the plan generation: landing it would drop reviewed decisions.
var errCarryIncomplete = errors.New("the carried Clusters were not all copied into the new generation")

// verifyCarried compares what the carry-over copied with what the plan generation
// carries. It runs before anything else is written into the generation, so every
// row the generation holds is a carried one; landing is refused on a mismatch.
func (s *reductionStore) verifyCarried(reductionID uint, source, generation string) error {
	var want, got struct{ Clusters, Members int64 }
	if err := s.db.Raw(`SELECT
		(SELECT COUNT(*) FROM reduction_clusters c WHERE c.reduction_id = ? AND c.generation = ? AND `+carryPredicate+`) AS clusters,
		(SELECT COUNT(*) FROM reduction_members m WHERE m.cluster_id IN (SELECT c.id FROM reduction_clusters c WHERE c.reduction_id = ? AND c.generation = ? AND `+carryPredicate+`)) AS members`,
		reductionID, source, reductionID, source).Scan(&want).Error; err != nil {
		return err
	}
	if err := s.db.Raw(`SELECT
		(SELECT COUNT(*) FROM reduction_clusters WHERE reduction_id = ? AND generation = ?) AS clusters,
		(SELECT COUNT(*) FROM reduction_members WHERE reduction_id = ? AND generation = ?) AS members`,
		reductionID, generation, reductionID, generation).Scan(&got).Error; err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%w: copied %d Clusters and %d members, the plan carries %d and %d",
			errCarryIncomplete, got.Clusters, got.Members, want.Clusters, want.Members)
	}
	return nil
}

// carryOver copies the carried Clusters of the plan generation into the new one.
func (s *reductionStore) carryOver(reductionID uint, generation string) error {
	var plan struct{ PlanGeneration *string }
	if err := s.db.Raw(`SELECT plan_generation FROM resource_reductions WHERE id = ?`, reductionID).Scan(&plan).Error; err != nil {
		return err
	}
	if plan.PlanGeneration == nil {
		return nil
	}
	source := *plan.PlanGeneration
	clusterCols, err := columnList(s.db, &models.ReductionClusterRow{}, "id", "generation")
	if err != nil {
		return err
	}
	memberCols, err := columnList(s.db, &models.ReductionMemberRow{}, "id", "cluster_id", "generation", "gone")
	if err != nil {
		return err
	}

	// Cluster rows, 500 per statement, every column.
	after := uint(0)
	for {
		// Each cursor moves only once the chunk's transaction has committed: a
		// retried transaction (lost to lock contention) must read the same chunk
		// again, not the one after it.
		var n int
		var next uint
		err := s.fenced(reductionID, generation, func(tx *gorm.DB) error {
			n, next = 0, after
			var ids []uint
			if err := tx.Raw(`SELECT c.id FROM reduction_clusters c WHERE c.reduction_id = ? AND c.generation = ? AND `+carryPredicate+` AND c.id > ? ORDER BY c.id LIMIT ?`,
				reductionID, source, after, reductionRowChunk).Scan(&ids).Error; err != nil {
				return err
			}
			n = len(ids)
			if n == 0 {
				return nil
			}
			next = ids[n-1]
			if carryOverChunkHook != nil {
				if err := carryOverChunkHook(tx, "clusters"); err != nil {
					return err
				}
			}
			return tx.Exec(`INSERT INTO reduction_clusters (generation, `+strings.Join(clusterCols, ", ")+`) SELECT ?, `+strings.Join(prefixed("c", clusterCols), ", ")+
				` FROM reduction_clusters c WHERE c.id IN ? ORDER BY c.id`, generation, ids).Error
		})
		if err != nil {
			return err
		}
		after = next
		if n < reductionRowChunk {
			break
		}
	}

	// Member rows by keyset (source cluster, position), remapped by key. `gone` comes
	// from the Resources, share-locked ascending first (Postgres; SQLite's writer
	// lock excludes a delete), so a deletion either committed before (seen) or waits
	// until the rows are committed (and then finds them).
	//
	// The carried Clusters are walked in windows of reductionRowChunk ids, and a
	// window's members are paged by (cluster, position) over that literal id list: a
	// member statement seeks idx_rm_page once per window Cluster and never rebuilds
	// the whole carried set, which a subquery over the Clusters did for every page.
	windowAfter := uint(0)
	for {
		var window []uint
		if err := s.db.Raw(carryClusterWindowSQL, reductionID, source, windowAfter, reductionRowChunk).Scan(&window).Error; err != nil {
			return err
		}
		if len(window) == 0 {
			break
		}
		windowAfter = window[len(window)-1]
		afterCluster, afterPosition := uint(0), -1
		for {
			var n int
			var nextCluster uint
			var nextPosition int
			err := s.fenced(reductionID, generation, func(tx *gorm.DB) error {
				n, nextCluster, nextPosition = 0, afterCluster, afterPosition
				var chunk []struct {
					ID         uint
					ClusterID  uint
					Position   int
					ResourceID uint
				}
				if err := tx.Raw(carryMemberPageSQL, window, afterCluster, afterCluster, afterPosition, reductionRowChunk).Scan(&chunk).Error; err != nil {
					return err
				}
				n = len(chunk)
				if n == 0 {
					return nil
				}
				ids := make([]uint, n)
				resourceIDs := make([]uint, n)
				for i, m := range chunk {
					ids[i], resourceIDs[i] = m.ID, m.ResourceID
				}
				nextCluster, nextPosition = chunk[n-1].ClusterID, chunk[n-1].Position
				if err := s.in(tx).shareLockResources(resourceIDs); err != nil {
					return err
				}
				if carryOverChunkHook != nil {
					if err := carryOverChunkHook(tx, "members"); err != nil {
						return err
					}
				}
				return tx.Exec(`INSERT INTO reduction_members (cluster_id, generation, gone, `+strings.Join(memberCols, ", ")+`)
					SELECT tc.id, ?, NOT EXISTS (SELECT 1 FROM resources r WHERE r.id = m.resource_id), `+strings.Join(prefixed("m", memberCols), ", ")+`
					FROM reduction_members m
					JOIN reduction_clusters sc ON sc.id = m.cluster_id
					JOIN reduction_clusters tc ON tc.reduction_id = sc.reduction_id AND tc.generation = ? AND tc."key" = sc."key"
					WHERE m.id IN ? ORDER BY m.id`, generation, generation, ids).Error
			})
			if err != nil {
				return err
			}
			afterCluster, afterPosition = nextCluster, nextPosition
			if n < reductionRowChunk {
				break
			}
		}
	}

	// gone_members of a Cluster whose members span chunks is derived after its last
	// chunk: a recount over the member rows, the Cluster rows locked first.
	after = 0
	for {
		var n int
		var next uint
		err := s.fenced(reductionID, generation, func(tx *gorm.DB) error {
			n, next = 0, after
			var ids []uint
			if err := tx.Raw(`SELECT id FROM reduction_clusters WHERE reduction_id = ? AND generation = ? AND id > ? ORDER BY id LIMIT ?`,
				reductionID, generation, after, reductionRowChunk).Scan(&ids).Error; err != nil {
				return err
			}
			n = len(ids)
			if n == 0 {
				return nil
			}
			next = ids[n-1]
			if err := s.in(tx).lockClusterRows(ids); err != nil {
				return err
			}
			return tx.Exec(`UPDATE reduction_clusters SET gone_members = (SELECT COUNT(*) FROM reduction_members m WHERE m.cluster_id = reduction_clusters.id AND m.gone AND NOT m.ejected) WHERE id IN ?`, ids).Error
		})
		if err != nil {
			return err
		}
		after = next
		if n < reductionRowChunk {
			return s.verifyCarried(reductionID, source, generation)
		}
	}
}

// insertFreshClusters inserts the fresh Clusters, one transaction each: the members'
// Resources (at most reductionMemberCap) share-locked in one ascending statement,
// then the rows, so no transaction locks Resources out of ascending order. `gone`
// and the file sizes come from that locked read. A key that collides with one
// already in the generation (a carried Cluster over the same members, or an earlier
// fresh one) takes the next numbered suffix, found by exact lookups.
func (s *reductionStore) insertFreshClusters(r *models.ResourceReduction, generation string, fresh []*models.ReductionCluster) error {
	var base int64
	if err := s.db.Raw(`SELECT COALESCE(MAX(position), 0) FROM reduction_clusters WHERE reduction_id = ? AND generation = ?`, r.ID, generation).Scan(&base).Error; err != nil {
		return err
	}
	for i, plan := range fresh {
		err := s.fenced(r.ID, generation, func(tx *gorm.DB) error {
			st := s.in(tx)
			return st.insertOneFresh(r, generation, plan, base+1+int64(i))
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// insertOneFresh is one fresh Cluster's insertion, inside the caller's fenced transaction.
func (s *reductionStore) insertOneFresh(r *models.ResourceReduction, generation string, plan *models.ReductionCluster, position int64) error {
	return s.insertStoredFresh(r, generation, storedFromPlanCluster(plan), position, nil)
}

// lockedResource is what a fresh Cluster's insertion reads of a member's Resource
// once it holds the share lock.
type lockedResource struct {
	FileSize int64
	Hash     string
}

// insertStoredFresh inserts one fresh Cluster inside the caller's fenced
// transaction: its members' Resources share-locked ascending, then the rows. check,
// when given, sees the locked read before anything is written and may refuse the
// Cluster (the streaming compute's re-check of what it built against what is true
// now, design D5). The caller's Cluster is not modified: a retried transaction
// starts from the same key.
func (s *reductionStore) insertStoredFresh(r *models.ResourceReduction, generation string, in *storedCluster, position int64, check func(c *storedCluster, locked map[uint]lockedResource) error) error {
	c := &storedCluster{Row: in.Row, Members: append([]models.ReductionMemberRow(nil), in.Members...)}
	ids := make([]uint, 0, len(c.Members))
	for _, m := range c.Members {
		ids = append(ids, m.ResourceID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if err := s.shareLockResources(ids); err != nil {
		return err
	}
	locked := map[uint]lockedResource{}
	for _, chunk := range chunkUints(dedupeUints(ids), reductionRowChunk) {
		var rows []struct {
			ID       uint
			FileSize int64
			Hash     string
		}
		if err := s.db.Raw(`SELECT id, file_size, COALESCE(hash, '') AS hash FROM resources WHERE id IN ?`, chunk).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			locked[row.ID] = lockedResource{FileSize: row.FileSize, Hash: row.Hash}
		}
	}
	if check != nil {
		if err := check(c, locked); err != nil {
			return err
		}
	}
	for i := range c.Members {
		l, found := locked[c.Members[i].ResourceID]
		c.Members[i].FileSize = l.FileSize
		c.Members[i].Gone = !found
	}
	c.Row.Position = position

	// A fresh Cluster holding a member of read-only history arrives unchecked: the
	// decision about those copies is unknown, so the new proposal needs fresh approval.
	var history int64
	if err := s.db.Raw(`SELECT COUNT(*) FROM (SELECT 1 FROM reduction_members m JOIN reduction_clusters c ON c.id = m.cluster_id
		WHERE c.reduction_id = ? AND c.generation = ? AND c.read_only AND m.resource_id IN ? LIMIT 1) h`, r.ID, generation, dedupeUints(ids)).Scan(&history).Error; err != nil {
		return err
	}
	if history > 0 {
		c.Row.Checked = false
	}

	key := c.Row.Key
	for suffix := 2; ; suffix++ {
		var taken int64
		if err := s.db.Raw(`SELECT COUNT(*) FROM reduction_clusters WHERE reduction_id = ? AND generation = ? AND "key" = ?`, r.ID, generation, c.Row.Key).Scan(&taken).Error; err != nil {
			return err
		}
		if taken == 0 {
			break
		}
		c.Row.Key = fmt.Sprintf("%s-%d", key, suffix)
	}
	return s.insertCluster(r, generation, c)
}

// storedFromPlanCluster is a Cluster as the in-memory build (and today's JSON plan)
// holds it, as rows. File sizes and `gone` are filled in by the caller from a locked
// read of the Resources.
func storedFromPlanCluster(plan *models.ReductionCluster) *storedCluster {
	c := &storedCluster{Row: models.ReductionClusterRow{
		Key: plan.ID, Tier: plan.Tier, State: plan.State, WinnerID: plan.WinnerID, Checked: plan.Checked,
		Reviewed: plan.Reviewed, Oversized: plan.Oversized, DecidedBy: plan.DecidedBy, Margin: plan.Margin,
		Undecided: plan.Undecided, KeepsSmaller: plan.KeepsSmaller, StaleReason: plan.StaleReason,
		Merged: plan.Merged, ClaimID: plan.ClaimID, AppliedAt: plan.AppliedAt,
	}}
	if len(plan.Lossy) > 0 {
		if raw, err := json.Marshal(plan.Lossy); err == nil {
			c.Row.Lossy = raw
		}
	}
	changed := map[uint]bool{}
	for _, id := range plan.ChangedMembers {
		changed[id] = true
	}
	for _, m := range plan.Members {
		row := models.ReductionMemberRow{
			ResourceID: m.ResourceID, Hash: m.Hash, InExtent: m.InExtent, Ejected: m.Ejected, EjectedReason: m.EjectedReason,
			OwnerAtMerge: plan.LoserOwners[m.ResourceID], Changed: changed[m.ResourceID],
		}
		if m.Distance != nil {
			d := int(*m.Distance)
			row.Distance = &d
		}
		c.Members = append(c.Members, row)
	}
	return c
}

// toPlanCluster is the inverse, for the test seams that hand a claimed Cluster back
// in the shape today's tests read.
func toPlanCluster(c *storedCluster) *models.ReductionCluster {
	out := &models.ReductionCluster{
		ID: c.Row.Key, Tier: c.Row.Tier, WinnerID: c.Row.WinnerID, DecidedBy: c.Row.DecidedBy, Margin: c.Row.Margin,
		Undecided: c.Row.Undecided, KeepsSmaller: c.Row.KeepsSmaller, State: c.Row.State, Checked: c.Row.Checked,
		Reviewed: c.Row.Reviewed, Oversized: c.Row.Oversized, StaleReason: c.Row.StaleReason, AppliedAt: c.Row.AppliedAt,
		Merged: c.Row.Merged, ClaimID: c.Row.ClaimID,
	}
	if len(c.Row.Lossy) > 0 {
		_ = json.Unmarshal(c.Row.Lossy, &out.Lossy)
	}
	for _, m := range c.Members {
		member := &models.ReductionMember{ResourceID: m.ResourceID, Hash: m.Hash, InExtent: m.InExtent, Ejected: m.Ejected, EjectedReason: m.EjectedReason}
		if m.Distance != nil {
			d := uint8(*m.Distance)
			member.Distance = &d
		}
		if m.Changed {
			out.ChangedMembers = append(out.ChangedMembers, m.ResourceID)
		}
		out.Members = append(out.Members, member)
	}
	return out
}

// ---- staging tables and the sweep ------------------------------------------------------------------

// clearStaging deletes a generation's staging rows (the staged Extent and the Near
// seeds) in keyset chunks, once the generation has landed.
func (s *reductionStore) clearStaging(reductionID uint, generation string) error {
	for _, table := range []string{"reduction_extent", "reduction_seeds"} {
		for {
			var ids []uint
			if err := s.db.Raw(`SELECT resource_id FROM `+table+` WHERE reduction_id = ? AND generation = ? ORDER BY resource_id LIMIT ?`, reductionID, generation, reductionRowChunk).Scan(&ids).Error; err != nil {
				return err
			}
			if len(ids) == 0 {
				break
			}
			if err := s.db.Exec(`DELETE FROM `+table+` WHERE reduction_id = ? AND generation = ? AND resource_id IN ?`, reductionID, generation, ids).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// planGuard is true when a plan-table row `x` is still protected: its generation is
// the Reduction's plan or its running compute, or a migration is writing it. Used
// as NOT EXISTS over a subquery so a NULL generation column never makes the whole
// predicate unknown (a NOT IN over a NULL would keep everything).
func planGuard(x string) string {
	return `NOT EXISTS (SELECT 1 FROM resource_reductions r WHERE r.id = ` + x + `.reduction_id AND (` + x + `.generation = r.plan_generation OR ` + x + `.generation = r.compute_generation OR ` +
		`EXISTS (SELECT 1 FROM reduction_migrations mg WHERE mg.reduction_id = ` + x + `.reduction_id AND mg.generation = ` + x + `.generation)))`
}

// stagingGuard protects staging rows more narrowly: only a running compute's own
// generation, while it computes.
func stagingGuard(x string) string {
	return `NOT EXISTS (SELECT 1 FROM resource_reductions r WHERE r.id = ` + x + `.reduction_id AND r.status = 'computing' AND ` + x + `.generation = r.compute_generation)`
}

// sweepBetweenReadAndDeleteHook runs after a sweep chunk read its ids and before it
// deletes them. Only a test sets it, to start a compute in between.
var sweepBetweenReadAndDeleteHook func()

// SweepReductionGenerations is the startup entry to the sweep.
func (ctx *MahresourcesContext) SweepReductionGenerations() error {
	return ctx.sweepReductionGenerations()
}

// sweepReductionGenerations deletes every generation of plan rows nobody owns, and
// the staging rows of any compute that is not running. Generations are discovered
// through the (reduction_id, generation) indexes one distinct pair at a time, then
// each is deleted by id keyset in chunks of reductionRowChunk, with the guard
// restated in the delete itself so a compute that claims the generation after the
// read keeps it. Rows whose Reduction no longer exists are removed by the same
// guard. It runs after landing, at compute start and at startup.
func (ctx *MahresourcesContext) sweepReductionGenerations() error {
	return ctx.reductionStore().sweepGenerations()
}

// sweepGenerations is the sweep over this store's handle, so a worker's compute-start
// sweep can carry the worker's context.
func (s *reductionStore) sweepGenerations() error {
	// A compute whose worker died holds its generation for as long as its row says
	// computing. Its lease has run out, so no worker can still write under it (every
	// write fences on the lease): it is failed first, which releases the generation
	// to the sweep below.
	if err := s.failExpiredComputes(); err != nil {
		return err
	}
	for _, table := range []string{"reduction_members", "reduction_clusters"} {
		if err := s.sweepTable(table, "id", planGuard, planPairProtected); err != nil {
			return err
		}
	}
	for _, table := range []string{"reduction_extent", "reduction_seeds"} {
		if err := s.sweepTable(table, "resource_id", stagingGuard, stagingPairProtected); err != nil {
			return err
		}
	}
	for _, spec := range []struct{ table, key string }{{"reduction_extent_resources", "resource_id"}, {"reduction_extent_groups", "group_id"}} {
		if err := s.sweepOrphans(spec.table, spec.key); err != nil {
			return err
		}
	}
	return nil
}

// reductionComputeExpiredMessage is the error a compute is failed with when its
// lease ran out without the worker finishing or failing it.
const reductionComputeExpiredMessage = "the clustering run stopped before it finished (its lease expired); recompute to try again"

// failExpiredComputes fails every Reduction whose compute lease has expired: status
// failed, the generation cleared (so planGuard and stagingGuard no longer protect
// it), the version bumped. The lease is the same instant beginReductionCompute
// treats as claimable, and a live worker renews it, so a running compute is never
// touched. One statement over the rows that qualify, which are few.
func (s *reductionStore) failExpiredComputes() error {
	now := time.Now()
	return retryOnLockContention(reductionCASRetries, func() error {
		return s.db.Exec(`UPDATE resource_reductions SET status = 'failed', computing_started_at = NULL, compute_deadline = NULL, compute_error = ?,
				compute_generation = NULL, compute_lease_until = 0, version = version + 1, updated_at = ?
			WHERE status = 'computing' AND compute_generation IS NOT NULL AND compute_lease_until <= ?`,
			reductionComputeExpiredMessage, now, now.UnixNano()).Error
	})
}

// planPairProtected is planGuard asked once for a whole (reduction, generation) pair
// instead of once per row: ownership is a property of the pair, so a live
// generation of any size costs one point query, not a scan of its rows.
func planPairProtected(reductionID uint, generation string) (string, []any) {
	return `SELECT 1 FROM resource_reductions r WHERE r.id = ? AND (? = r.plan_generation OR ? = r.compute_generation OR ` +
			`EXISTS (SELECT 1 FROM reduction_migrations mg WHERE mg.reduction_id = r.id AND mg.generation = ?))`,
		[]any{reductionID, generation, generation, generation}
}

func stagingPairProtected(reductionID uint, generation string) (string, []any) {
	return `SELECT 1 FROM resource_reductions r WHERE r.id = ? AND r.status = 'computing' AND ? = r.compute_generation`,
		[]any{reductionID, generation}
}

// sweepTable walks the distinct (reduction_id, generation) pairs of table through
// its index. A pair that is protected is skipped without reading any of its rows;
// a stale pair is deleted in chunks of reductionRowChunk, each chunk's ids read by
// the pair's index prefix alone (what was deleted is gone, so no keyset is needed)
// with the guard restated in the delete itself, so a compute that claims the
// generation after the read keeps it: the delete then removes nothing and the pair
// is left. The work is proportional to the stale rows plus one seek per pair.
func (s *reductionStore) sweepTable(table, keyCol string, guard func(x string) string, protected func(uint, string) (string, []any)) error {
	lastReduction, lastGeneration := uint(0), ""
	first := true
	for {
		var pair []struct {
			ReductionID uint
			Generation  string
		}
		cond := "1 = 1"
		args := []any{}
		if !first {
			cond = "(reduction_id, generation) > (?, ?)"
			args = []any{lastReduction, lastGeneration}
		}
		if err := s.db.Raw(`SELECT reduction_id, generation FROM `+table+` WHERE `+cond+` ORDER BY reduction_id, generation LIMIT 1`, args...).Scan(&pair).Error; err != nil {
			return err
		}
		if len(pair) == 0 {
			return nil
		}
		first = false
		lastReduction, lastGeneration = pair[0].ReductionID, pair[0].Generation
		guardSQL, guardArgs := protected(lastReduction, lastGeneration)
		var owned []int
		if err := s.db.Raw(guardSQL+` LIMIT 1`, guardArgs...).Scan(&owned).Error; err != nil {
			return err
		}
		if len(owned) > 0 {
			continue
		}
		for {
			var ids []uint
			if err := s.db.Raw(`SELECT x.`+keyCol+` FROM `+table+` x WHERE x.reduction_id = ? AND x.generation = ? LIMIT ?`, lastReduction, lastGeneration, reductionRowChunk).Scan(&ids).Error; err != nil {
				return err
			}
			if len(ids) == 0 {
				break
			}
			if sweepBetweenReadAndDeleteHook != nil {
				sweepBetweenReadAndDeleteHook()
			}
			res := s.db.Exec(`DELETE FROM `+table+` WHERE reduction_id = ? AND generation = ? AND `+keyCol+` IN ? AND `+guard(table), lastReduction, lastGeneration, ids)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				break // claimed since the read: the pair is protected now
			}
		}
	}
}

// sweepOrphans removes selection rows whose Reduction no longer exists. The key
// column is unique only within a Reduction, so a chunk is named by (reduction id,
// key) pairs: deleting by key alone would take the same key out of every orphaned
// Reduction in one statement, however many that is.
func (s *reductionStore) sweepOrphans(table, keyCol string) error {
	// Each read continues after the last key of the one before, so the live rows in
	// front of the orphans are walked once, not once per chunk.
	afterReduction, afterKey := uint(0), uint(0)
	for {
		var pairs []struct {
			ReductionID uint
			Key         uint
		}
		if err := s.db.Raw(`SELECT x.reduction_id AS reduction_id, x.`+keyCol+` AS "key" FROM `+table+` x WHERE (x.reduction_id, x.`+keyCol+`) > (?, ?) AND NOT EXISTS (SELECT 1 FROM resource_reductions r WHERE r.id = x.reduction_id) ORDER BY x.reduction_id, x.`+keyCol+` LIMIT ?`,
			afterReduction, afterKey, reductionRowChunk).Scan(&pairs).Error; err != nil {
			return err
		}
		if len(pairs) == 0 {
			return nil
		}
		afterReduction, afterKey = pairs[len(pairs)-1].ReductionID, pairs[len(pairs)-1].Key
		tuples := make([][]any, len(pairs))
		for i, p := range pairs {
			tuples[i] = []any{p.ReductionID, p.Key}
		}
		if err := s.db.Exec(`DELETE FROM `+table+` WHERE (reduction_id, `+keyCol+`) IN ? AND NOT EXISTS (SELECT 1 FROM resource_reductions r WHERE r.id = `+table+`.reduction_id)`, tuples).Error; err != nil {
			return err
		}
	}
}

// storeDeleteReduction deletes an authorized Reduction: its row in one small
// transaction (so a compute's next fence finds nothing to write under), then its
// children by reduction_id in chunks.
func (ctx *MahresourcesContext) storeDeleteReduction(r *models.ResourceReduction) error {
	s := ctx.reductionStore()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		q := tx.Where("id = ?", r.ID)
		if r.CreatedByUserId == nil {
			q = q.Where("created_by_user_id IS NULL")
		} else {
			q = q.Where("created_by_user_id = ?", *r.CreatedByUserId)
		}
		res := q.Delete(&models.ResourceReduction{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrReductionNotFound
		}
		// The migration's own row goes with its Reduction, in the same small
		// transaction (the Reduction row first, then this one: the order
		// publication takes them in).
		return tx.Exec(`DELETE FROM reduction_migrations WHERE reduction_id = ?`, r.ID).Error
	})
	if err != nil {
		return err
	}
	// A legacy Reduction deleted after its spool was written and before it was
	// published leaves spool rows no surviving parent points at; they go in chunks.
	if err := s.deleteSpoolWhere(r.ID, "1 = ?", 1); err != nil {
		return err
	}
	for _, table := range []string{"reduction_members", "reduction_clusters"} {
		for {
			var ids []uint
			if err := s.db.Raw(`SELECT id FROM `+table+` WHERE reduction_id = ? ORDER BY id LIMIT ?`, r.ID, reductionRowChunk).Scan(&ids).Error; err != nil {
				return err
			}
			if len(ids) == 0 {
				break
			}
			if err := s.db.Exec(`DELETE FROM `+table+` WHERE id IN ?`, ids).Error; err != nil {
				return err
			}
		}
	}
	// The compute staging tables are keyed (reduction, generation, resource), so a
	// chunk is read and deleted as exact (generation, resource) tuples: naming the
	// resource alone would also take it out of every other generation, and the
	// statement would remove more rows than the chunk it read.
	for _, table := range []string{"reduction_extent", "reduction_seeds"} {
		for {
			var pairs []struct {
				Generation string
				ResourceID uint
			}
			if err := s.db.Raw(`SELECT generation, resource_id FROM `+table+` WHERE reduction_id = ? ORDER BY generation, resource_id LIMIT ?`, r.ID, reductionRowChunk).Scan(&pairs).Error; err != nil {
				return err
			}
			if len(pairs) == 0 {
				break
			}
			tuples := make([][]any, len(pairs))
			for i, p := range pairs {
				tuples[i] = []any{p.Generation, p.ResourceID}
			}
			if err := s.db.Exec(`DELETE FROM `+table+` WHERE reduction_id = ? AND (generation, resource_id) IN ?`, r.ID, tuples).Error; err != nil {
				return err
			}
		}
	}
	for _, spec := range []struct{ table, key string }{
		{"reduction_extent_resources", "resource_id"}, {"reduction_extent_groups", "group_id"},
	} {
		for {
			var ids []uint
			if err := s.db.Raw(`SELECT `+spec.key+` FROM `+spec.table+` WHERE reduction_id = ? ORDER BY `+spec.key+` LIMIT ?`, r.ID, reductionRowChunk).Scan(&ids).Error; err != nil {
				return err
			}
			if len(ids) == 0 {
				break
			}
			// These selection tables are keyed (reduction, key): the chunk's keys are the
			// chunk's rows.
			if err := s.db.Exec(`DELETE FROM `+spec.table+` WHERE reduction_id = ? AND `+spec.key+` IN ?`, r.ID, ids).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
