package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// The table-backed apply (design D10), beside ApplyResourceReduction. Same request,
// same response semantics, except as D10 and D15 list: the apply is one batch with a
// server-generated id, claimed in one transaction that stamps the Clusters it may
// act on and checks the token and counts against the stamp; the loop reads the
// stamped Clusters fifty at a time in plan order; every outcome write is fenced by
// the batch and a lease judged after the row lock; and the response caps its lists
// and carries totals. Nothing calls it yet.

// staleReasonLeftSelection is a Loser that is no longer inside the Reduction's
// selection: an out-of-selection copy may be kept but never removed.
const staleReasonLeftSelection = "a Resource in this Cluster is no longer inside this Reduction's selection"

// errLeftSelection is the same finding inside the merge precondition.
var errLeftSelection = errors.New("a Loser left the Reduction's selection since it was computed")

// reductionWinnerNameBytes bounds the Winner name an outcome entry carries (D8).
const reductionWinnerNameBytes = 1024

// applyRun is one batch as the loop and its hooks know it.
type applyRun struct {
	reductionID   uint
	batch         string
	generation    string
	startedAt     time.Time
	keepIdentical bool
	keepNear      bool
}

// storeApplyReduction merges every checked Cluster of an authorized Reduction. The
// caller has loaded r through its owner check and passes the request's version: a
// stale one is refused before anything is claimed.
func (ctx *MahresourcesContext) storeApplyReduction(r *models.ResourceReduction, request *query_models.ReductionApply) (*contracts.ReductionApplyResult, error) {
	if r == nil || request == nil || r.ID == 0 || request.ID != r.ID {
		return nil, errors.New("no Resource Reduction given")
	}
	if len(request.RequestID) > reductionRequestIDMax {
		return nil, fmt.Errorf("requestId is longer than %d characters", reductionRequestIDMax)
	}
	// Detached from the request, as the JSON apply is: a cancellation landing between
	// a Cluster's claim and its merge would strand it. The ceiling bounds a wedged batch.
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx.db.Statement.Context), ReductionApplyCeiling)
	defer cancel()
	ctx = ctx.WithParentContext(detached)

	if err := reductionEditable(r); err != nil {
		return nil, err
	}
	if request.Version != r.Version {
		return nil, ErrReductionConflict
	}
	if request.SettleOnly {
		return ctx.storeSettleClaims(r, request)
	}
	if r.PlanGeneration == nil {
		return &contracts.ReductionApplyResult{}, nil
	}

	run, err := ctx.storeClaimApplyBatch(r, request)
	if err != nil {
		return nil, err
	}
	run.keepIdentical, run.keepNear = r.KeepAsVersionIdentical, r.KeepAsVersionNear

	result := &contracts.ReductionApplyResult{}
	collector := &applyCollector{ctx: ctx, result: result}

	// Claims an earlier batch stranded are settled first, each chunk fenced by this
	// batch; a Cluster they reopen was not stamped, so it is not in this batch.
	settled, settledTotal, reconcileErr := ctx.storeReconcileClaimsFor(r.ID, batchClaimFence(r.ID, run.batch), run.batch)
	for _, o := range settled {
		collector.addStale(o)
	}
	result.StaleCount += settledTotal - len(settled)

	// On its own bounded context: the ceiling may be what ended the loop, and the
	// record must still say the batch finished.
	settle, settleCancel := ctx.settlementContext()
	defer settleCancel()
	if reconcileErr != nil && !errors.Is(reconcileErr, ErrReductionLeaseLost) {
		// The batch did not do what it set out to do: it is interrupted, not
		// finished. The lease is given back so the next apply is not held off for
		// the rest of its window, and the record keeps no finish time and no counts.
		settle.storeAbandonApply(r.ID, run)
		return nil, reconcileErr
	}
	if reconcileErr == nil {
		ctx.storeApplyLoop(r, run, collector, detached)
	}
	collector.seal()
	settle.storeFinishApply(r.ID, run, result)
	return result, nil
}

// applyCollector keeps the response bounded: both lists stop growing at the cap,
// the counts keep going, and each entry carries the Winner's bounded name.
type applyCollector struct {
	ctx    *MahresourcesContext
	result *contracts.ReductionApplyResult
}

func (c *applyCollector) addApplied(o contracts.ReductionApplyOutcome) {
	c.result.AppliedCount++
	c.result.Destroyed += len(o.LoserIDs)
	if len(c.result.Applied) < reductionApplyOutcomeCap {
		o.WinnerName = c.ctx.storeWinnerName(o.WinnerID)
		c.result.Applied = append(c.result.Applied, o)
	}
}

func (c *applyCollector) addStale(o contracts.ReductionApplyOutcome) {
	c.result.StaleCount++
	if len(c.result.Stale) < reductionApplyOutcomeCap {
		o.WinnerName = c.ctx.storeWinnerName(o.WinnerID)
		c.result.Stale = append(c.result.Stale, o)
	}
}

func (c *applyCollector) seal() {
	if c.result.Applied == nil {
		c.result.Applied = []contracts.ReductionApplyOutcome{}
	}
	if c.result.Stale == nil {
		c.result.Stale = []contracts.ReductionApplyOutcome{}
	}
	c.result.More = c.result.AppliedCount > len(c.result.Applied) || c.result.StaleCount > len(c.result.Stale)
}

// storeWinnerName is a Resource's name as the caller may see it, cut to
// reductionWinnerNameBytes in SQL, read as bytes and trimmed to a rune boundary. A
// Resource the caller cannot see (or that no longer exists) has no name here.
func (ctx *MahresourcesContext) storeWinnerName(id uint) string {
	if id == 0 {
		return ""
	}
	cut := fmt.Sprintf("SUBSTR(CAST(name AS BLOB), 1, %d)", reductionWinnerNameBytes)
	if ctx.db.Dialector.Name() == "postgres" {
		cut = fmt.Sprintf("SUBSTRING(convert_to(name, 'UTF8') FROM 1 FOR %d)", reductionWinnerNameBytes)
	}
	var rows []struct {
		ID    uint
		Bytes []byte
	}
	if err := ctx.db.Model(&models.Resource{}).Select("id, "+cut+" AS bytes").Where("resources.id = ?", id).Find(&rows).Error; err != nil || len(rows) == 0 {
		return ""
	}
	b := rows[0].Bytes
	for len(b) > 0 && !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return string(b)
}

// ---- the batch claim ---------------------------------------------------------------

// storeClaimApplyBatch is the batch claim, one transaction: the version
// compare-and-set, the refusal of a live apply or settle, the stamp of every eligible
// Cluster, the token and counts recomputed over the stamp and required to match the
// request, and the record naming the batch with its lease. Any mismatch rolls the
// whole stamp back.
func (ctx *MahresourcesContext) storeClaimApplyBatch(r *models.ResourceReduction, request *query_models.ReductionApply) (*applyRun, error) {
	store := ctx.reductionStore()
	var run *applyRun
	err := retryOnLockContention(reductionCASRetries, func() error {
		run = nil
		return store.db.Transaction(func(tx *gorm.DB) error {
			ok, err := casReductionTx(tx, r)
			if err != nil {
				return err
			}
			if !ok {
				return ErrReductionConflict
			}
			if err := ctx.accountStillBound(tx); err != nil {
				return err
			}
			fresh, err := reloadReduction(tx, r.ID)
			if err != nil {
				return err
			}
			now := time.Now()
			switch {
			case reductionEditable(fresh) != nil:
				return ErrReductionBusy
			case fresh.PlanGeneration == nil:
				return ErrReductionConflict
			case fresh.ApplyLeaseUntil > now.UnixNano(), fresh.SettleOp != nil && fresh.SettleLeaseUntil > now.UnixNano():
				return ErrReductionConflict
			}
			generation := *fresh.PlanGeneration
			batch := newClaimID()
			st := store.in(tx)
			if err := st.stampBatch(fresh, generation, batch); err != nil {
				return err
			}
			token, clusters, losers, err := st.batchToken(fresh.ID, generation, batch)
			if err != nil {
				return err
			}
			if request.ApplyToken != "" && request.ApplyToken != token {
				return ErrReductionApplyFiguresChanged
			}
			if (request.ExpectedClusters != nil && int(*request.ExpectedClusters) != clusters) ||
				(request.ExpectedLosers != nil && int(*request.ExpectedLosers) != losers) {
				return ErrReductionApplyFiguresChanged
			}
			record, err := json.Marshal(models.ReductionLastApply{
				StartedAt: now, RequestID: request.RequestID, BatchID: batch, Generation: generation,
				ExpectedClusters: clusters, ExpectedLosers: losers,
			})
			if err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE resource_reductions SET last_apply = ?, apply_lease_until = ? WHERE id = ?`,
				string(record), leaseDeadline(now, now), r.ID).Error; err != nil {
				return err
			}
			run = &applyRun{reductionID: r.ID, batch: batch, generation: generation, startedAt: now}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return run, nil
}

// stampBatch marks every Cluster this batch may act on, in id-ordered chunks of
// reductionRowChunk: open, not read-only history, nothing gone, unclaimed, checked
// with a Loser to remove, not a full star that lost its seed, and not withheld (the
// filter sits inside each chunk's inner select, so the chunk is the visible ones).
func (s *reductionStore) stampBatch(r *models.ResourceReduction, generation, batch string) error {
	prefix, where, args := s.base(r, generation)
	lock := ""
	if s.postgres() {
		lock = " FOR UPDATE OF c"
	}
	after := uint(0)
	for {
		var ids []uint
		stepArgs := append(append([]any(nil), args...), after, reductionRowChunk)
		if err := s.db.Raw(prefix+`SELECT c.id FROM reduction_clusters c WHERE `+where+
			` AND `+actionable("c")+` AND `+approved("c")+` AND c.claim_id = '' AND NOT (c.fixed_seed AND c.winner_id <> c.seed_resource_id)`+
			` AND c.id > ? ORDER BY c.id LIMIT ?`+lock, stepArgs...).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := s.db.Exec(`UPDATE reduction_clusters SET apply_batch = ?, approved_winner = winner_id, approved_seq = restore_seq WHERE id IN ?`, batch, ids).Error; err != nil {
			return err
		}
		after = ids[len(ids)-1]
		if len(ids) < reductionRowChunk {
			return nil
		}
	}
}

// batchToken is the apply token over the Clusters stamped with the batch, in key
// order (the order the review's token is taken in), with their counts.
func (s *reductionStore) batchToken(reductionID uint, generation, batch string) (token string, clusters, losers int, err error) {
	digest := newTokenDigest()
	last := ""
	for {
		var rows []models.ReductionClusterRow
		if err = s.db.Raw(`SELECT id, "key", loser_count, fingerprint FROM reduction_clusters WHERE reduction_id = ? AND generation = ? AND apply_batch = ? AND "key" > ? ORDER BY "key" LIMIT ?`,
			reductionID, generation, batch, last, eligibleChunk).Scan(&rows).Error; err != nil {
			return "", 0, 0, err
		}
		for _, row := range rows {
			digest.add(row.Fingerprint)
			clusters++
			losers += row.LoserCount
		}
		if len(rows) < eligibleChunk {
			return digest.sum(), clusters, losers, nil
		}
		last = rows[len(rows)-1].Key
	}
}

// ---- the loop ------------------------------------------------------------------------

// applyReadChunk is how many stamped Clusters one loop read returns.
const applyReadChunk = 50

// storeApplyLoop walks the stamped Clusters in plan order, fifty keys per read, and
// applies each. It stops when the context ends, or when the batch no longer owns the
// Reduction (an expired or replaced lease): nothing new starts then.
func (ctx *MahresourcesContext) storeApplyLoop(r *models.ResourceReduction, run *applyRun, out *applyCollector, detached context.Context) {
	store := ctx.reductionStore()
	lastPos, lastID := int64(-1)<<62, uint(0)
	lastRenew := time.Now()
	for {
		var rows []struct {
			ID       uint
			Key      string
			Position int64
		}
		if err := store.db.Raw(`SELECT id, "key", position FROM reduction_clusters WHERE reduction_id = ? AND generation = ? AND apply_batch = ? AND (position > ? OR (position = ? AND id > ?)) ORDER BY position, id LIMIT ?`,
			run.reductionID, run.generation, run.batch, lastPos, lastPos, lastID, applyReadChunk).Scan(&rows).Error; err != nil {
			out.result.Errors++
			return
		}
		for _, row := range rows {
			// Past the ceiling nothing new starts; what was not reached stays approved.
			if detached.Err() != nil {
				return
			}
			if time.Since(lastRenew) >= leaseRenewEvery {
				held, err := ctx.storeRenewApplyLease(run)
				if err == nil && !held {
					return
				}
				lastRenew = time.Now()
			}
			outcome, lost := ctx.storeApplyOneCluster(r, run, row.Key)
			switch {
			case outcome.Omit:
			case outcome.Changed:
				out.result.Changed++
			case outcome.Failed:
				out.result.Errors++
			case outcome.Applied:
				out.addApplied(outcome.Report)
			default:
				out.addStale(outcome.Report)
			}
			if lost {
				return
			}
			lastPos, lastID = row.Position, row.ID
		}
		if len(rows) < applyReadChunk {
			return
		}
	}
}

// storeRenewApplyLease pushes a live apply lease forward without touching version or
// updated_at: it takes the row without changing it, requires the record to still
// name this batch, reads the old lease and compares it with a clock read after the
// lock, and only then writes. An expired lease is never revived.
func (ctx *MahresourcesContext) storeRenewApplyLease(run *applyRun) (bool, error) {
	store := ctx.reductionStore()
	held := false
	err := store.db.Transaction(func(tx *gorm.DB) error {
		held = false
		if _, err := fenceBatch(tx, run.reductionID, run.batch, true); err != nil {
			if errors.Is(err, ErrReductionLeaseLost) {
				return nil
			}
			return err
		}
		held = true
		return renewApplyLeaseTx(tx, run)
	})
	return held, err
}

// renewApplyLeaseTx writes the next lease; the caller holds the row and has just
// proved the batch live.
func renewApplyLeaseTx(tx *gorm.DB, run *applyRun) error {
	return tx.Exec(`UPDATE resource_reductions SET apply_lease_until = ? WHERE id = ?`, leaseDeadline(leaseNow(), run.startedAt), run.reductionID).Error
}

// storeFinishApply completes the record the claim wrote and gives the lease back.
// It is bookkeeping, so it is allowed after the lease expired, while the batch and
// the pinned generation still match; a later apply that replaced the record is
// never overwritten. Failure is logged, as every bookkeeping write after a merge is.
func (ctx *MahresourcesContext) storeFinishApply(reductionID uint, run *applyRun, result *contracts.ReductionApplyResult) {
	store := ctx.reductionStore()
	err := retryOnLockContention(reductionCASRetries, func() error {
		return store.db.Transaction(func(tx *gorm.DB) error {
			if _, err := fenceBatch(tx, reductionID, run.batch, false); err != nil {
				return err
			}
			var current struct{ LastApply []byte }
			if err := tx.Raw(`SELECT last_apply FROM resource_reductions WHERE id = ?`, reductionID).Scan(&current).Error; err != nil {
				return err
			}
			var rec models.ReductionLastApply
			if err := json.Unmarshal(current.LastApply, &rec); err != nil {
				return err
			}
			finished := time.Now()
			rec.FinishedAt = &finished
			rec.Applied, rec.Removed, rec.Stale = result.AppliedCount, result.Destroyed, result.StaleCount
			encoded, err := json.Marshal(rec)
			if err != nil {
				return err
			}
			// The version moves unless a compute is running (see bumpReductionVersion).
			return tx.Exec(`UPDATE resource_reductions SET last_apply = ?, apply_lease_until = 0, version = version + CASE WHEN status = 'computing' THEN 0 ELSE 1 END, updated_at = ? WHERE id = ?`,
				string(encoded), finished, reductionID).Error
		})
	})
	if err != nil {
		ctx.logReductionWriteFailure(reductionID, "apply "+run.batch, err)
	}
}

// storeAbandonApply gives the lease back without finishing the record: the batch
// stays unfinished, which the reader shows as interrupted, and the next apply may
// start at once. Bookkeeping like the finish, so it too is allowed after the lease
// expired, while the batch and the pinned generation still match.
func (ctx *MahresourcesContext) storeAbandonApply(reductionID uint, run *applyRun) {
	store := ctx.reductionStore()
	err := retryOnLockContention(reductionCASRetries, func() error {
		return store.db.Transaction(func(tx *gorm.DB) error {
			if _, err := fenceBatch(tx, reductionID, run.batch, false); err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE resource_reductions SET apply_lease_until = 0 WHERE id = ?`, reductionID).Error; err != nil {
				return err
			}
			return bumpReductionVersion(tx, reductionID)
		})
	})
	if err != nil && !errors.Is(err, ErrReductionLeaseLost) {
		ctx.logReductionWriteFailure(reductionID, "abandon "+run.batch, err)
	}
}

// storeApplyOneCluster revalidates and merges one stamped Cluster and records what
// happened to it. The second result says the batch no longer owns the Reduction:
// the loop stops.
func (ctx *MahresourcesContext) storeApplyOneCluster(r *models.ResourceReduction, run *applyRun, key string) (clusterApplyOutcome, bool) {
	st := ctx.reductionStore()
	omitted := func() (clusterApplyOutcome, bool) {
		if ctx.db.Statement.Context.Err() != nil {
			return clusterApplyOutcome{Omit: true}, false
		}
		return clusterApplyOutcome{Failed: true}, false
	}
	// Re-read, with visibility: a Cluster this caller may not fully see is neither
	// merged, nor marked, nor named.
	c, err := st.loadCluster(r, run.generation, key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return clusterApplyOutcome{Omit: true}, false
	}
	if err != nil {
		return omitted()
	}
	row := &c.Row
	if row.State != models.ReductionClusterOpen || !row.Checked || row.ReadOnly || row.LoserCount == 0 || row.ApplyBatch != run.batch ||
		row.WinnerID != row.ApprovedWinner || row.RestoreSeq != row.ApprovedSeq ||
		(row.FixedSeed && row.WinnerID != row.SeedResourceID) {
		return clusterApplyOutcome{Changed: true}, false
	}
	report := contracts.ReductionApplyOutcome{ClusterID: row.Key, Tier: row.Tier, WinnerID: row.WinnerID, LoserIDs: c.loserIDs()}

	if reason, changed, err := ctx.storeRevalidate(c, false); err != nil {
		return omitted()
	} else if reason != "" {
		if ctx.db.Statement.Context.Err() != nil {
			return clusterApplyOutcome{Omit: true}, false
		}
		// A member that vanished since the visibility read is that case caught late:
		// left alone and unreported for a scoped caller, a deleted copy otherwise.
		if reason == staleReasonMemberGone {
			if ctx.isScopedPrincipal() {
				return clusterApplyOutcome{Omit: true}, false
			}
			reason = staleReasonDeleted
		}
		report.Reason = reason
		if reductionApplyBeforeStaleHook != nil {
			reductionApplyBeforeStaleHook()
		}
		switch ctx.storeRecordStale(run.reductionID, run.batch, key, row.Fingerprint, reason, changed) {
		case staleSuperseded:
			return clusterApplyOutcome{Changed: true}, false
		case staleHidden:
			return clusterApplyOutcome{Omit: true}, false
		case staleFailed:
			// Nothing was written, and no reviewer changed anything.
			return clusterApplyOutcome{Failed: true}, false
		}
		return clusterApplyOutcome{Report: report}, false
	}

	if ctx.db.Statement.Context.Err() != nil {
		return clusterApplyOutcome{Omit: true}, false
	}
	// Claimed before it is merged: every override on it refuses from here, so the
	// ids about to be merged are frozen. What is merged is the snapshot the claim
	// committed, never the one read above.
	claimed, gate, err := ctx.storeClaimCluster(run.reductionID, run.batch, key)
	if errors.Is(err, errAccountChanged) {
		// The caller lost the visibility this batch was started under: nothing was
		// claimed, nothing may be reported, and every later Cluster would say the same.
		return clusterApplyOutcome{Omit: true}, true
	}
	if err != nil {
		return clusterApplyOutcome{Failed: true}, errors.Is(err, ErrReductionLeaseLost)
	}
	switch gate {
	case claimChanged:
		return clusterApplyOutcome{Changed: true}, false
	case claimHidden:
		return clusterApplyOutcome{Omit: true}, false
	}
	if reductionApplyAfterClaimHook != nil {
		reductionApplyAfterClaimHook()
	}
	report.WinnerID = claimed.Row.WinnerID
	report.LoserIDs = claimed.loserIDs()

	keepAsVersion := run.keepIdentical
	if claimed.Row.Tier == models.ReductionTierNear {
		keepAsVersion = run.keepNear
	}
	var held []uint
	for _, m := range claimed.Members {
		if m.ResourceID != claimed.Row.WinnerID && m.Ejected {
			held = append(held, m.ResourceID)
		}
	}
	err = ctx.MergeResourcesLocking(claimed.Row.WinnerID, report.LoserIDs, keepAsVersion, held, !ctx.isScopedPrincipal(), ctx.storeMergeHooks(run, claimed))
	if err == nil {
		return clusterApplyOutcome{Applied: true, Report: report}, false
	}
	settle, cancel := ctx.settlementContext()
	defer cancel()
	switch {
	case errors.Is(err, errClaimLost):
		// Another writer settled or replaced this claim; nothing was deleted.
		return clusterApplyOutcome{Changed: true}, false
	case errors.Is(err, ErrReductionLeaseLost):
		// The batch expired or was replaced: nothing was destroyed. The claim is handed
		// back (bookkeeping, still allowed for the same batch) and the loop stops.
		settle.storeReleaseClaim(run.reductionID, run.batch, key, claimed.Row.ClaimID)
		return clusterApplyOutcome{Omit: true}, true
	case errors.Is(err, errAccountChanged):
		// The account changed under the request: every later Cluster would say the same,
		// so the loop stops, as it does when the claim itself notices.
		settle.storeReleaseClaim(run.reductionID, run.batch, key, claimed.Row.ClaimID)
		return clusterApplyOutcome{Omit: true}, true
	case errors.Is(err, errMergeVisibilityLost),
		ctx.isScopedPrincipal() && errors.Is(err, ErrMergeParticipantUnavailable):
		settle.storeReleaseClaim(run.reductionID, run.batch, key, claimed.Row.ClaimID)
		return clusterApplyOutcome{Omit: true}, false
	}
	reason := staleReasonMergeRefused + ": " + err.Error()
	switch {
	case errors.Is(err, ErrMergePairGone):
		reason = staleReasonPairGone
	case errors.Is(err, ErrMergeContentChanged):
		reason = staleReasonHashChanged
	case errors.Is(err, errLeftSelection):
		reason = staleReasonLeftSelection
	}
	report.Reason = reason
	var refusal *mergeRefusal
	var changedIDs []uint
	if errors.As(err, &refusal) {
		changedIDs = refusal.ids
	}
	switch settle.storeDemote(run.reductionID, run.batch, key, claimed.Row.ClaimID, reason, changedIDs) {
	case demoteRecorded:
		return clusterApplyOutcome{Report: report}, false
	case demoteReleased:
		// The lease ran out: the claim went back to approved, there is no outcome
		// to record, and the loop stops as it does for any lost lease.
		return clusterApplyOutcome{Omit: true}, true
	case demoteHidden:
		// The account changed: as at the claim, every later Cluster would say the same.
		return clusterApplyOutcome{Omit: true}, true
	}
	return clusterApplyOutcome{Changed: true}, false
}

// ---- revalidation ----------------------------------------------------------------------

// storeRevalidate answers "is this still the thing that was reviewed": every
// non-ejected member exists for this caller, its bytes are what was reviewed, every
// Loser is still inside the Reduction's selection (a live predicate over at most
// reductionMemberCap ids), and on the Near tier each Loser still has its stored pair
// to the Winner (a named-pair probe, never every pair of the Winner). It returns the
// reason a Cluster is refused, and the members that changed. lock takes the pair rows
// share-locked, for the merge's precondition.
func (ctx *MahresourcesContext) storeRevalidate(c *storedCluster, lock bool) (string, []uint, error) {
	losers := c.loserIDs()
	if len(losers) == 0 {
		return staleReasonNothingToDo, nil, nil
	}
	var ids []uint
	expected := map[uint]string{}
	for _, m := range c.Members {
		if !m.Ejected {
			ids = append(ids, m.ResourceID)
			expected[m.ResourceID] = m.Hash
		}
	}
	// Through the scoped handle, so "no longer exists" and "no longer one this
	// principal may see" are one answer.
	have, err := ctx.narrowResources(ids)
	if err != nil {
		return "", nil, err
	}
	var changed []uint
	for _, m := range c.Members {
		if m.Ejected {
			continue
		}
		hash, found := have[m.ResourceID]
		if !found {
			return staleReasonMemberGone, nil, nil
		}
		if hash != expected[m.ResourceID] {
			changed = append(changed, m.ResourceID)
		}
	}
	if len(changed) > 0 {
		return staleReasonHashChanged, changed, nil
	}

	store := ctx.reductionStore()
	inside, err := store.extentContains(c.Row.ReductionID, losers)
	if err != nil {
		return "", nil, err
	}
	var left []uint
	for _, id := range losers {
		if !inside[id] {
			left = append(left, id)
		}
	}
	if len(left) > 0 {
		return staleReasonLeftSelection, left, nil
	}

	if c.Row.Tier != models.ReductionTierNear {
		return "", nil, nil
	}
	_, aThreshold := ctx.similarityThresholds()
	paired, err := store.pairProbe(c.Row.WinnerID, losers, ctx.similarityThreshold(), aThreshold, lock)
	if err != nil {
		return "", nil, err
	}
	var unpaired []uint
	for _, id := range losers {
		if _, ok := paired[id]; !ok {
			unpaired = append(unpaired, id)
		}
	}
	if len(unpaired) > 0 {
		return staleReasonPairGone, unpaired, nil
	}
	return "", nil, nil
}

// narrowResources reads id and hash of the named Resources through the caller's
// scoped handle: the hash is all a revalidation compares, so nothing else crosses.
func (ctx *MahresourcesContext) narrowResources(ids []uint) (map[uint]string, error) {
	out := make(map[uint]string, len(ids))
	for _, chunk := range chunkUints(dedupeUints(ids), idChunk) {
		var rows []struct {
			ID   uint
			Hash string
		}
		if err := ctx.db.Model(&models.Resource{}).Select("resources.id AS id, resources.hash AS hash").Where("resources.id IN ?", chunk).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			out[row.ID] = row.Hash
		}
	}
	return out, nil
}

// extentContains is which of the named Resources are inside the Reduction's
// selection right now: the whole library when extent_all is set, an explicitly
// selected Resource, or one owned by or related to a selected Group or any Group
// below it. A selected Group the principal may not see selects nothing. One live
// predicate over the selections tables, never a set held in Go.
func (s *reductionStore) extentContains(reductionID uint, ids []uint) (map[uint]bool, error) {
	out := make(map[uint]bool, len(ids))
	if len(ids) == 0 || s.scope.mode == scopeDeny {
		return out, nil
	}
	var all bool
	if err := s.db.Raw(`SELECT extent_all FROM resource_reductions WHERE id = ?`, reductionID).Scan(&all).Error; err != nil {
		return nil, err
	}
	prefix, args := s.with()
	visibleRoot := ""
	if s.scope.mode == scopeSubtree {
		visibleRoot = " AND eg.group_id IN " + subtreeIDs
	}
	ext := `ext_tree(id) AS (
		SELECT eg.group_id FROM reduction_extent_groups eg WHERE eg.reduction_id = ?` + visibleRoot + ` AND EXISTS (SELECT 1 FROM groups g WHERE g.id = eg.group_id)
		UNION
		SELECT g.id FROM groups g INNER JOIN ext_tree et ON g.owner_id = et.id
	)`
	if prefix == "" {
		prefix = "WITH RECURSIVE " + ext + " "
	} else {
		prefix = strings.TrimSpace(prefix) + ", " + ext + " "
	}
	for _, chunk := range chunkUints(dedupeUints(ids), idChunk) {
		var found []uint
		query := prefix + `SELECT r.id FROM resources r WHERE r.id IN ? AND (? OR
			EXISTS (SELECT 1 FROM reduction_extent_resources er WHERE er.reduction_id = ? AND er.resource_id = r.id) OR
			r.owner_id IN (SELECT id FROM ext_tree) OR
			EXISTS (SELECT 1 FROM groups_related_resources grr WHERE grr.resource_id = r.id AND grr.group_id IN (SELECT id FROM ext_tree)))`
		callArgs := append(append([]any(nil), args...), reductionID, chunk, all, reductionID)
		if err := s.db.Raw(query, callArgs...).Scan(&found).Error; err != nil {
			return nil, err
		}
		for _, id := range found {
			out[id] = true
		}
	}
	return out, nil
}

// ---- the merge hooks -----------------------------------------------------------------------

// storeMergeHooks ties the Cluster rows to the merge's own transaction.
//
// BeforeLocks takes the Reduction row first (every writer's first lock) and
// re-verifies, there, that this batch still owns the Reduction with a live lease
// judged after the lock, that the pinned generation is still the plan's, and that
// the claim on the Cluster is still this attempt's; nothing destructive has run. It
// then renews the lease, so the merge begins with a full window, and Renew does the
// same between the merge's own work units (a merge of many Losers may outlast one
// window; the Reduction row stays locked by this transaction, so the renewal is what
// InTx's check reads, and an expired lease is never revived). Precondition, behind the participant locks, repeats the Cluster's
// own validity. InTx reads a fresh clock and checks the lease again, so a worker
// parked past expiry rolls back instead of deleting; then marks the Cluster merged
// with its outcome batch and records each Loser's owning Group, in the transaction
// that deleted them.
func (ctx *MahresourcesContext) storeMergeHooks(run *applyRun, claimed *storedCluster) MergeHooks {
	owners := map[uint]uint{}
	loserIDs := claimed.loserIDs()
	return MergeHooks{
		Renew: func(txCtx *MahresourcesContext) error {
			tx := txCtx.reductionStore().db
			var lease int64
			if err := tx.Raw(`SELECT apply_lease_until FROM resource_reductions WHERE id = ?`, run.reductionID).Scan(&lease).Error; err != nil {
				return err
			}
			now := leaseNow()
			if lease <= now.UnixNano() {
				// An expired lease is never revived: the merge rolls back whole.
				return ErrReductionLeaseLost
			}
			if time.Duration(lease-now.UnixNano()) > leaseWindow/2 {
				return nil
			}
			return renewApplyLeaseTx(tx, run)
		},
		BeforeLocks: func(txCtx *MahresourcesContext) error {
			tx := txCtx.reductionStore().db
			if _, err := fenceBatch(tx, run.reductionID, run.batch, true); err != nil {
				return err
			}
			if err := claimStillOurs(tx, claimed); err != nil {
				return err
			}
			return renewApplyLeaseTx(tx, run)
		},
		Precondition: func(txCtx *MahresourcesContext) error {
			if reductionPreconditionHook != nil {
				reductionPreconditionHook()
			}
			if txCtx.isScopedPrincipal() {
				// Every member, ejected ones included, must be there and visible.
				var all []uint
				for _, m := range claimed.Members {
					all = append(all, m.ResourceID)
				}
				have, err := txCtx.narrowResources(all)
				if err != nil {
					return err
				}
				if len(have) != len(dedupeUints(all)) {
					return errMergeVisibilityLost
				}
			}
			// Where each Loser lives, read while it still exists, for the record a scoped
			// viewer's visibility of this Cluster will rest on once it is gone.
			var rows []struct {
				ID      uint
				OwnerID *uint `gorm:"column:owner_id"`
			}
			if err := txCtx.unscopedDB().Model(&models.Resource{}).Select("id", "owner_id").Where("id IN ?", loserIDs).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				owners[row.ID] = 0
				if row.OwnerID != nil {
					owners[row.ID] = *row.OwnerID
				}
			}
			reason, changed, err := txCtx.storeRevalidate(claimed, true)
			if err != nil {
				return err
			}
			sort.Slice(changed, func(i, j int) bool { return changed[i] < changed[j] })
			switch reason {
			case "":
				return nil
			case staleReasonMemberGone, staleReasonHashChanged:
				// A vanished member is a changed one here, as it always was.
				return &mergeRefusal{cause: fmt.Errorf("%w: resources %v", ErrMergeContentChanged, changed), ids: changed}
			case staleReasonPairGone:
				return &mergeRefusal{cause: fmt.Errorf("%w: resources %v", ErrMergePairGone, changed), ids: changed}
			case staleReasonLeftSelection:
				return &mergeRefusal{cause: fmt.Errorf("%w: resources %v", errLeftSelection, changed), ids: changed}
			}
			return &mergeRefusal{cause: errors.New(reason), ids: changed}
		},
		InTx: func(txCtx *MahresourcesContext) error {
			tx := txCtx.reductionStore().db
			var lease int64
			if err := tx.Raw(`SELECT apply_lease_until FROM resource_reductions WHERE id = ?`, run.reductionID).Scan(&lease).Error; err != nil {
				return err
			}
			if lease <= leaseNow().UnixNano() {
				return ErrReductionLeaseLost
			}
			if err := claimStillOurs(tx, claimed); err != nil {
				return err
			}
			st := txCtx.reductionStore()
			if err := st.lockClusterRows([]uint{claimed.Row.ID}); err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE reduction_clusters SET merged = ?, outcome_batch = ? WHERE id = ? AND claim_id = ?`,
				true, run.batch, claimed.Row.ID, claimed.Row.ClaimID).Error; err != nil {
				return err
			}
			for id, owner := range owners {
				if err := tx.Exec(`UPDATE reduction_members SET owner_at_merge = ? WHERE cluster_id = ? AND resource_id = ?`, owner, claimed.Row.ID, id).Error; err != nil {
					return err
				}
			}
			return bumpReductionVersion(tx, run.reductionID)
		},
	}
}

// claimStillOurs requires the Cluster to be applied, unconfirmed and carrying this
// attempt's claim id; anything else is another writer's settlement.
func claimStillOurs(tx *gorm.DB, claimed *storedCluster) error {
	var row struct {
		State   string
		Merged  bool
		ClaimID string
	}
	if err := tx.Raw(`SELECT state, merged, claim_id FROM reduction_clusters WHERE id = ?`, claimed.Row.ID).Scan(&row).Error; err != nil {
		return err
	}
	if row.State != models.ReductionClusterApplied || row.Merged || row.ClaimID != claimed.Row.ClaimID {
		return errClaimLost
	}
	return nil
}
