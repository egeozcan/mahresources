package application_context

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// The apply-side single-Cluster writers on the tables (design D3, D10), beside
// the JSON ones in resource_reduction_apply.go. They are the pieces of an apply
// that write one Cluster row (or a keyset chunk of them): the per-Cluster claim,
// demotion of a refused claim, release of a claim, the early stale outcome, the
// reconciliation of stranded claims, and the settle-only apply with its own fence.
// The batch claim itself, the loop and the merge hooks are a later piece.
//
// Each outcome write runs under the Reduction row lock and proves, there, that the
// caller still owns the batch (see fenceBatch), so an expired or replaced worker
// can neither claim nor mark anything stale. Bookkeeping that settles a claim the
// same batch took (demote, release) stays allowed after the lease expired, while
// the batch, the pinned generation and the claim id still match: a committed merge
// is never hidden by an expired lease.

// reductionApplyOutcomeCap is how many outcome entries a result carries (D10).
const reductionApplyOutcomeCap = 200

// reconcileChunk is how many stranded claims one reconciliation transaction settles.
const reconcileChunk = 50

// applyRecord is what the store reads of the Reduction's last_apply document: the
// batch that owns the Reduction and the plan generation it pinned.
type applyRecord struct {
	BatchID    string `json:"batchId"`
	Generation string `json:"generation"`
}

// bumpReductionVersion moves the version once a Cluster write has landed, as every
// plan write moves it today.
func bumpReductionVersion(tx *gorm.DB, id uint) error {
	return tx.Exec(`UPDATE resource_reductions SET version = version + 1, updated_at = ? WHERE id = ?`, time.Now(), id).Error
}

// fenceBatch locks the Reduction row and proves the batch still owns it: the
// last_apply record names this batch, and the generation it pinned is still the
// plan's. With live set it also requires an unexpired apply lease, judged against
// a clock read after the lock was taken (a fence that waited past expiry fails).
// It returns the pinned generation, or ErrReductionLeaseLost.
func fenceBatch(tx *gorm.DB, reductionID uint, batch string, live bool) (string, error) {
	if err := lockReductionRowTx(tx, reductionID); err != nil {
		return "", err
	}
	var row struct {
		PlanGeneration  *string
		LastApply       []byte
		ApplyLeaseUntil int64
	}
	if err := tx.Raw(`SELECT plan_generation, last_apply, apply_lease_until FROM resource_reductions WHERE id = ?`, reductionID).Scan(&row).Error; err != nil {
		return "", err
	}
	now := time.Now()
	var rec applyRecord
	if len(row.LastApply) > 0 && string(row.LastApply) != "null" {
		if err := json.Unmarshal(row.LastApply, &rec); err != nil {
			return "", err
		}
	}
	if batch == "" || row.PlanGeneration == nil || rec.BatchID != batch || rec.Generation == "" || rec.Generation != *row.PlanGeneration {
		return "", ErrReductionLeaseLost
	}
	if live && row.ApplyLeaseUntil <= now.UnixNano() {
		return "", ErrReductionLeaseLost
	}
	return rec.Generation, nil
}

// claimGate is how a per-Cluster claim ended, when it did not error.
type claimGate int

const (
	// claimWon: the Cluster is claimed and the snapshot returned is what to merge.
	claimWon claimGate = iota
	// claimChanged: another decision moved the Cluster first (unchecked, skipped,
	// ejected into nothing, promoted, replaced, deleted from). Nothing was written.
	claimChanged
	// claimHidden: the Cluster reaches a Resource the caller may not see. Nothing
	// was written and nothing may be reported about it.
	claimHidden
)

// storeClaimCluster takes one Cluster out of play for the merge about to happen
// and hands back the exact snapshot it committed. It requires, under the Reduction
// row lock, the live batch and its pinned generation (ErrReductionLeaseLost
// otherwise), and on the Cluster: stamped into this batch, open, checked,
// something to remove, nothing gone, not read-only history, the Winner and
// restore_seq that were approved (an ejection during the batch is honoured; a
// promotion or a restored Loser is refused), a full star still keeping its seed,
// and fully visible to the caller.
func (ctx *MahresourcesContext) storeClaimCluster(reductionID uint, batch, key string) (*storedCluster, claimGate, error) {
	store := ctx.reductionStore()
	var claimed *storedCluster
	gate := claimChanged
	err := retryOnLockContention(reductionCASRetries, func() error {
		claimed, gate = nil, claimChanged
		return store.db.Transaction(func(tx *gorm.DB) error {
			generation, err := fenceBatch(tx, reductionID, batch, true)
			if err != nil {
				return err
			}
			st := store.in(tx)
			c, err := st.lockedCluster(&models.ResourceReduction{ID: reductionID}, generation, key)
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				return nil
			case errors.Is(err, errClusterWithheld):
				gate = claimHidden
				return nil
			case err != nil:
				return err
			}
			row := &c.Row
			if row.State != models.ReductionClusterOpen || !row.Checked || row.ReadOnly || row.GoneMembers > 0 || row.LoserCount == 0 ||
				row.ApplyBatch != batch || row.WinnerID != row.ApprovedWinner || row.RestoreSeq != row.ApprovedSeq ||
				(row.FixedSeed && row.WinnerID != row.SeedResourceID) {
				return nil
			}
			before := copyMembers(c.Members)
			now := time.Now()
			row.State = models.ReductionClusterApplied
			row.Checked = false
			row.Reviewed = true
			row.Merged = false
			row.ClaimID = newClaimID()
			row.AppliedAt = &now
			if err := st.writeCluster(c, before); err != nil {
				return err
			}
			if err := bumpReductionVersion(tx, reductionID); err != nil {
				return err
			}
			claimed, gate = c, claimWon
			return nil
		})
	})
	if err != nil {
		return nil, claimChanged, err
	}
	return claimed, gate, nil
}

// storeDemoteToStale takes back a claim whose merge was refused: the only writer
// that may move a Cluster off applied. It clears the claim id (a demoted claim
// left set would be refused by the next stamp) and records which members changed.
// It reports whether the demotion landed; a claim someone else settled, or a batch
// that no longer owns the Reduction, is not one. Bookkeeping: allowed after the
// lease expired, while the batch, generation and claim id match.
func (ctx *MahresourcesContext) storeDemoteToStale(reductionID uint, batch, key, claimID, reason string, changed []uint) bool {
	return ctx.settleOwnClaim(reductionID, batch, key, claimID, false, func(st *reductionStore, c *storedCluster) error {
		c.Row.State = models.ReductionClusterStale
		c.Row.StaleReason = reason
		c.Row.AppliedAt = nil
		c.Row.Checked = false
		c.Row.ClaimID = ""
		flagged := map[uint]bool{}
		for _, id := range changed {
			flagged[id] = true
		}
		for i := range c.Members {
			c.Members[i].Changed = flagged[c.Members[i].ResourceID]
		}
		return nil
	})
}

// storeReleaseClaim gives back this attempt's claim: the Cluster returns to open
// and approved, exactly as it was approved, with nothing merged. Before reopening
// it recounts which members still exist, under share locks on their Resources, so
// a deletion that landed while it was claimed is not lost to the reopened row.
func (ctx *MahresourcesContext) storeReleaseClaim(reductionID uint, batch, key, claimID string) bool {
	return ctx.settleOwnClaim(reductionID, batch, key, claimID, true, func(st *reductionStore, c *storedCluster) error {
		if err := st.recountGone(c); err != nil {
			return err
		}
		c.Row.State = models.ReductionClusterOpen
		c.Row.Checked = true
		c.Row.AppliedAt = nil
		c.Row.ClaimID = ""
		return nil
	})
}

// settleOwnClaim is the shape demote and release share: the bookkeeping fence, the
// locks, a re-read that must still be this claim (applied, not merged, same claim
// id), the change, the write. It returns whether it wrote. A failure is logged,
// not returned, as every bookkeeping write after a merge is.
func (ctx *MahresourcesContext) settleOwnClaim(reductionID uint, batch, key, claimID string, resources bool, change func(*reductionStore, *storedCluster) error) bool {
	store := ctx.reductionStore()
	wrote := false
	err := retryOnLockContention(reductionCASRetries, func() error {
		wrote = false
		return store.db.Transaction(func(tx *gorm.DB) error {
			generation, err := fenceBatch(tx, reductionID, batch, false)
			if err != nil {
				return err
			}
			st := store.in(tx)
			c, err := st.lockedClusterUnfiltered(reductionID, generation, key, resources)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if claimID == "" || c.Row.State != models.ReductionClusterApplied || c.Row.Merged || c.Row.ClaimID != claimID {
				return nil
			}
			before := copyMembers(c.Members)
			if err := change(st, c); err != nil {
				return err
			}
			if err := st.writeCluster(c, before); err != nil {
				return err
			}
			if err := bumpReductionVersion(tx, reductionID); err != nil {
				return err
			}
			wrote = true
			return nil
		})
	})
	if err != nil {
		if !errors.Is(err, ErrReductionLeaseLost) {
			ctx.logReductionWriteFailure(reductionID, key, err)
		}
		return false
	}
	return wrote
}

// recountGone re-reads which members' Resources exist and sets `gone` on every
// member row, ejected or not. The caller holds the members' Resources and the
// Cluster row locked.
func (s *reductionStore) recountGone(c *storedCluster) error {
	ids := make([]uint, 0, len(c.Members))
	for _, m := range c.Members {
		ids = append(ids, m.ResourceID)
	}
	exist, err := s.existingResources(ids)
	if err != nil {
		return err
	}
	for i := range c.Members {
		c.Members[i].Gone = !exist[c.Members[i].ResourceID]
	}
	return nil
}

// existingResources is the set of ids among the given that have a Resource row, read
// from storage (unscoped): "deleted" is a fact about the table, not about a viewer.
func (s *reductionStore) existingResources(ids []uint) (map[uint]bool, error) {
	out := make(map[uint]bool, len(ids))
	for _, chunk := range chunkUints(dedupeUints(ids), reductionRowChunk) {
		var found []uint
		if err := s.db.Model(&models.Resource{}).Where("id IN ?", chunk).Pluck("id", &found).Error; err != nil {
			return nil, err
		}
		for _, id := range found {
			out[id] = true
		}
	}
	return out, nil
}

// storeRecordStale records a Cluster that failed revalidation as stale, the way the
// merge does its work: the Reduction row, then the Cluster's member Resources
// (visibility judged with them locked), then the Cluster row. It writes only if the
// Cluster is still open and approved with the very proposal that failed
// (fingerprint), so a decision made in between stands; a Cluster that left the
// caller's scope is neither recorded nor reported. It needs the live batch: an
// expired worker cannot mark a successor's proposal stale (it reports superseded).
func (ctx *MahresourcesContext) storeRecordStale(reductionID uint, batch, key, failedFingerprint, reason string, changed []uint) staleWrite {
	store := ctx.reductionStore()
	outcome := staleSuperseded
	err := retryOnLockContention(reductionCASRetries, func() error {
		outcome = staleSuperseded
		return store.db.Transaction(func(tx *gorm.DB) error {
			generation, err := fenceBatch(tx, reductionID, batch, true)
			if err != nil {
				return err
			}
			st := store.in(tx)
			c, err := st.lockedCluster(&models.ResourceReduction{ID: reductionID}, generation, key)
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				return nil
			case errors.Is(err, errClusterWithheld):
				outcome = staleHidden
				return nil
			case err != nil:
				return err
			}
			row := &c.Row
			if row.State != models.ReductionClusterOpen || !row.Checked || row.LoserCount == 0 || row.Fingerprint != failedFingerprint {
				return nil
			}
			before := copyMembers(c.Members)
			row.State = models.ReductionClusterStale
			row.StaleReason = reason
			// Unchecked, so the next apply does not walk into the same refusal, and
			// frozen, so a recompute leaves it where the reviewer can look at it.
			row.Checked = false
			row.Reviewed = true
			flagged := map[uint]bool{}
			for _, id := range changed {
				flagged[id] = true
			}
			for i := range c.Members {
				c.Members[i].Changed = flagged[c.Members[i].ResourceID]
			}
			if err := st.writeCluster(c, before); err != nil {
				return err
			}
			if err := bumpReductionVersion(tx, reductionID); err != nil {
				return err
			}
			outcome = staleRecorded
			return nil
		})
	})
	if err != nil {
		if !errors.Is(err, ErrReductionLeaseLost) {
			ctx.logReductionWriteFailure(reductionID, key, err)
		}
		return staleSuperseded
	}
	return outcome
}

// ---- reconcile stranded claims ----------------------------------------------------

// claimFence starts a reconciliation chunk's transaction by proving the caller's
// right to write: an apply batch's (fenceBatch, live) or a settle-only's own
// (settleFence). It returns the generation to reconcile.
type claimFence func(tx *gorm.DB) (generation string, err error)

// batchClaimFence is the fence of an apply batch's reconciliation chunks.
func batchClaimFence(reductionID uint, batch string) claimFence {
	return func(tx *gorm.DB) (string, error) { return fenceBatch(tx, reductionID, batch, true) }
}

// storeReconcileClaims settles Clusters an earlier batch claimed and never
// finished. A merge and its confirmation commit in one transaction, so a claimed
// Cluster that is not confirmed was not merged: it goes back to open and approved
// when its members all still exist, and is stale (a Resource in it no longer
// exists) when one does not. It walks `state = 'applied' AND NOT merged` by id
// keyset, ascending, one transaction per reconcileChunk Clusters, each beginning
// with the caller's fence. A claim the caller cannot fully see is left exactly as
// it is. It returns the Clusters it made stale (capped at reductionApplyOutcomeCap
// entries) and how many there were.
func (ctx *MahresourcesContext) storeReconcileClaims(reductionID uint, fence claimFence) ([]contracts.ReductionApplyOutcome, int, error) {
	store := ctx.reductionStore()
	var stale []contracts.ReductionApplyOutcome
	total := 0
	after := uint(0)
	for {
		var chunkStale []contracts.ReductionApplyOutcome
		var last uint
		var seen int
		err := retryOnLockContention(reductionCASRetries, func() error {
			chunkStale, last, seen = nil, 0, 0
			return store.db.Transaction(func(tx *gorm.DB) error {
				generation, err := fence(tx)
				if err != nil {
					return err
				}
				var reconciled int
				chunkStale, last, seen, reconciled, err = store.in(tx).reconcileChunk(reductionID, generation, after)
				if err != nil {
					return err
				}
				if reconciled > 0 {
					return bumpReductionVersion(tx, reductionID)
				}
				return nil
			})
		})
		if err != nil {
			return stale, total, err
		}
		total += len(chunkStale)
		for _, o := range chunkStale {
			if len(stale) < reductionApplyOutcomeCap {
				stale = append(stale, o)
			}
		}
		if seen < reconcileChunk {
			return stale, total, nil
		}
		after = last
	}
}

// reconcileChunk settles the next reconcileChunk stranded claims after `after`.
// The order is the design's: their member Resources share-locked ascending, their
// Cluster rows locked ascending, and only then each re-read with visibility and
// judged. seen counts the claims the keyset passed (visible or not), last is the
// highest id seen, reconciled counts the Clusters it changed.
func (s *reductionStore) reconcileChunk(reductionID uint, generation string, after uint) (stale []contracts.ReductionApplyOutcome, last uint, seen, reconciled int, err error) {
	var ids []uint
	if err = s.db.Raw(`SELECT id FROM reduction_clusters WHERE reduction_id = ? AND generation = ? AND state = 'applied' AND NOT merged AND id > ? ORDER BY id LIMIT ?`,
		reductionID, generation, after, reconcileChunk).Scan(&ids).Error; err != nil || len(ids) == 0 {
		return nil, 0, 0, 0, err
	}
	seen, last = len(ids), ids[len(ids)-1]

	var resourceIDs []uint
	if err = s.db.Raw(`SELECT DISTINCT resource_id FROM reduction_members WHERE cluster_id IN ?`, ids).Scan(&resourceIDs).Error; err != nil {
		return nil, 0, 0, 0, err
	}
	if err = s.shareLockResources(resourceIDs); err != nil {
		return nil, 0, 0, 0, err
	}
	if err = s.lockClusterRows(ids); err != nil {
		return nil, 0, 0, 0, err
	}
	prefix, where, args := s.base(&models.ResourceReduction{ID: reductionID}, generation)
	var rows []models.ReductionClusterRow
	if err = s.db.Raw(prefix+`SELECT c.* FROM reduction_clusters c WHERE `+where+` AND c.id IN ? AND c.state = 'applied' AND NOT c.merged ORDER BY c.id`,
		append(args, ids)...).Scan(&rows).Error; err != nil {
		return nil, 0, 0, 0, err
	}
	for i := range rows {
		c := &storedCluster{Row: rows[i]}
		if c.Members, err = s.loadMembers(rows[i].ID, reductionMemberCap); err != nil {
			return nil, 0, 0, 0, err
		}
		before := copyMembers(c.Members)
		if err = s.recountGone(c); err != nil {
			return nil, 0, 0, 0, err
		}
		deriveClusterColumns(c)
		c.Row.AppliedAt = nil
		c.Row.ClaimID = ""
		if c.Row.GoneMembers > 0 {
			c.Row.State, c.Row.Checked, c.Row.StaleReason = models.ReductionClusterStale, false, staleReasonDeleted
			stale = append(stale, contracts.ReductionApplyOutcome{
				ClusterID: c.Row.Key, Tier: c.Row.Tier, WinnerID: c.Row.WinnerID, LoserIDs: c.loserIDs(), Reason: staleReasonDeleted,
			})
		} else {
			c.Row.State, c.Row.Checked = models.ReductionClusterOpen, true
		}
		if err = s.writeCluster(c, before); err != nil {
			return nil, 0, 0, 0, err
		}
		reconciled++
	}
	return stale, last, seen, reconciled, nil
}

// ---- settle-only ------------------------------------------------------------------

// storeSettleClaims is an apply that only settles: the same refusals and the same
// reconciliation as a batch claim, and nothing is merged, claimed or recorded as an
// apply. It takes its own fence (settle_op, settle_lease_until), mutually exclusive
// with an apply, another settle and a compute; it never writes last_apply, so the
// evidence of an earlier apply survives; and the version moves only in a chunk that
// actually reconciles a claim (a settle that finds only claims hidden from the
// caller changes nothing).
func (ctx *MahresourcesContext) storeSettleClaims(r *models.ResourceReduction, request *query_models.ReductionApply) (*contracts.ReductionApplyResult, error) {
	if r == nil || request == nil || r.ID == 0 {
		return nil, errors.New("no Resource Reduction given")
	}
	store := ctx.reductionStore()
	op := newClaimID()

	var generation *string
	err := retryOnLockContention(reductionCASRetries, func() error {
		return store.db.Transaction(func(tx *gorm.DB) error {
			if err := lockReductionRowTx(tx, r.ID); err != nil {
				return err
			}
			fresh, err := reloadReduction(tx, r.ID)
			if err != nil {
				return err
			}
			now := time.Now()
			switch {
			case EffectiveReductionStatus(fresh) == models.ReductionStatusComputing || fresh.PlanSchema == 0:
				return ErrReductionBusy
			case request.Version != fresh.Version || r.Version != fresh.Version:
				return ErrReductionConflict
			case fresh.ApplyLeaseUntil > now.UnixNano(), fresh.SettleOp != nil && fresh.SettleLeaseUntil > now.UnixNano():
				return ErrReductionConflict
			}
			generation = fresh.PlanGeneration
			return tx.Exec(`UPDATE resource_reductions SET settle_op = ?, settle_lease_until = ? WHERE id = ?`, op, now.Add(leaseWindow).UnixNano(), r.ID).Error
		})
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = store.db.Exec(`UPDATE resource_reductions SET settle_op = NULL, settle_lease_until = 0 WHERE id = ? AND settle_op = ?`, r.ID, op).Error
	}()
	if generation == nil {
		return &contracts.ReductionApplyResult{}, nil
	}
	if _, _, err := ctx.storeReconcileClaims(r.ID, settleFence(r.ID, op)); err != nil {
		return nil, err
	}
	return &contracts.ReductionApplyResult{}, nil
}

// settleFence is settle-only's own fence for each chunk: under the Reduction row
// lock the row still names this settle and its lease is live against a clock read
// after the lock; the lease is renewed for the next chunk.
func settleFence(reductionID uint, op string) claimFence {
	return func(tx *gorm.DB) (string, error) {
		if err := lockReductionRowTx(tx, reductionID); err != nil {
			return "", err
		}
		var row struct {
			PlanGeneration   *string
			SettleOp         *string
			SettleLeaseUntil int64
		}
		if err := tx.Raw(`SELECT plan_generation, settle_op, settle_lease_until FROM resource_reductions WHERE id = ?`, reductionID).Scan(&row).Error; err != nil {
			return "", err
		}
		now := time.Now()
		if row.PlanGeneration == nil || row.SettleOp == nil || *row.SettleOp != op || row.SettleLeaseUntil <= now.UnixNano() {
			return "", ErrReductionLeaseLost
		}
		if err := tx.Exec(`UPDATE resource_reductions SET settle_lease_until = ? WHERE id = ? AND settle_op = ?`, now.Add(leaseWindow).UnixNano(), reductionID, op).Error; err != nil {
			return "", err
		}
		return *row.PlanGeneration, nil
	}
}
