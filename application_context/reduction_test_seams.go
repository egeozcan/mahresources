package application_context

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/types"
)

// OverwritePlanForTest writes a plan wholesale as the Reduction's plan: a new
// generation holding exactly these Clusters (states, claims and merged flags as
// given), pointed at by plan_generation under the version compare-and-set. How a test puts a Cluster into a state that has
// no other seam, such as the same key over a different proposal.
func (ctx *MahresourcesContext) OverwritePlanForTest(reductionID, version uint, plan models.ResourceReductionPlan) error {
	s := ctx.reductionStore()
	generation := string(types.NewUUIDv7())
	r := &models.ResourceReduction{ID: reductionID}
	var base int64
	for i, planned := range plan.Clusters {
		err := s.db.Transaction(func(tx *gorm.DB) error {
			return s.in(tx).insertOneFresh(r, generation, planned, base+1+int64(i))
		})
		if err != nil {
			return err
		}
	}
	coverageJSON, err := json.Marshal(plan.Coverage)
	if err != nil {
		return err
	}
	var settingsJSON, applyJSON any
	if plan.Settings != nil {
		raw, _ := json.Marshal(plan.Settings)
		settingsJSON = string(raw)
	}
	if plan.LastApply != nil {
		raw, _ := json.Marshal(plan.LastApply)
		applyJSON = string(raw)
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE resource_reductions SET plan_generation = ?, coverage = ?, plan_settings = ?, last_apply = COALESCE(?, last_apply),
			version = version + 1, updated_at = ? WHERE id = ? AND version = ?`, generation, string(coverageJSON), settingsJSON, applyJSON, time.Now(), reductionID, version)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrReductionConflict
		}
		return nil
	})
	if err != nil {
		return err
	}
	return ctx.sweepReductionGenerations()
}

// StoreReductionPlanForTest is StoreReductionPlanForTest on the tables: it lands
// the plan's Clusters (all of them fresh, nothing carried: the plan already holds
// whatever was carried) through the same fenced landing a compute uses. The
// Reduction must be computing, as beginReductionComputeStore leaves it.
func (ctx *MahresourcesContext) StoreReductionPlanForTest(reductionID uint, jobID string, version uint, plan models.ResourceReductionPlan) error {
	s := ctx.reductionStore()
	var row struct{ ComputeGeneration *string }
	if err := s.db.Raw(`SELECT compute_generation FROM resource_reductions WHERE id = ?`, reductionID).Scan(&row).Error; err != nil {
		return err
	}
	if row.ComputeGeneration == nil {
		return ErrReductionComputeSuperseded
	}
	return ctx.landComputedPlan(&models.ResourceReduction{ID: reductionID}, jobID, *row.ComputeGeneration, version, plan.Clusters, plan.Coverage, plan.Settings, false)
}

// ClaimClusterForApplyForTest is ClaimClusterForApplyForTest on the tables: the
// per-Cluster claim of the batch that currently owns the Reduction. It returns the
// snapshot the claim committed in today's shape, or nil when the claim was refused.
func (ctx *MahresourcesContext) ClaimClusterForApplyForTest(reductionID uint, batch, key string) *models.ReductionCluster {
	claimed, gate, err := ctx.storeClaimCluster(reductionID, batch, key)
	if err != nil || gate != claimWon {
		return nil
	}
	return toPlanCluster(claimed)
}

// ReleaseClaimForTest settles a claim as a delayed writer would, with the claim
// id it holds.
func (ctx *MahresourcesContext) ReleaseClaimForTest(reductionID uint, batch, key, claimID string) {
	ctx.storeReleaseClaim(reductionID, batch, key, claimID)
}

// StrandClaimForTest claims one Cluster the way an apply batch that then died would
// have: a synthetic batch pinned to the plan generation owns the Reduction long
// enough to stamp and claim the Cluster, and its lease is then left expired. The
// Cluster stays claimed (applied, not merged) with nobody running, which is what
// settle-only and the next apply reconcile. It returns the claimed snapshot in plan
// shape, or nil when the claim was refused.
func (ctx *MahresourcesContext) StrandClaimForTest(reductionID uint, key string) *models.ReductionCluster {
	s := ctx.reductionStore()
	var r struct{ PlanGeneration *string }
	if err := s.db.Raw(`SELECT plan_generation FROM resource_reductions WHERE id = ?`, reductionID).Scan(&r).Error; err != nil || r.PlanGeneration == nil {
		return nil
	}
	batch := "stranded-" + newClaimID()
	record := fmt.Sprintf(`{"batchId":%q,"generation":%q,"startedAt":%q,"applied":0,"removed":0,"stale":0}`, batch, *r.PlanGeneration, time.Now().UTC().Format(time.RFC3339))
	if err := s.db.Exec(`UPDATE resource_reductions SET last_apply = ?, apply_lease_until = ? WHERE id = ?`, record, time.Now().Add(time.Minute).UnixNano(), reductionID).Error; err != nil {
		return nil
	}
	if err := s.db.Exec(`UPDATE reduction_clusters SET apply_batch = ?, approved_winner = winner_id, approved_seq = restore_seq
		WHERE reduction_id = ? AND generation = ? AND "key" = ?`, batch, reductionID, *r.PlanGeneration, key).Error; err != nil {
		return nil
	}
	claimed := ctx.ClaimClusterForApplyForTest(reductionID, batch, key)
	// The batch died: nothing renews its lease, and the claim is stranded.
	_ = s.db.Exec(`UPDATE resource_reductions SET apply_lease_until = 1 WHERE id = ?`, reductionID).Error
	return claimed
}

// ReleaseStrandedClaimForTest settles a claim StrandClaimForTest made as a delayed
// writer of its own batch would, with the claim id it holds.
func (ctx *MahresourcesContext) ReleaseStrandedClaimForTest(reductionID uint, key, claimID string) {
	var r struct{ LastApply types.JSON }
	if err := ctx.reductionStore().db.Raw(`SELECT last_apply FROM resource_reductions WHERE id = ?`, reductionID).Scan(&r).Error; err != nil {
		return
	}
	var record struct {
		BatchID string `json:"batchId"`
	}
	if json.Unmarshal(r.LastApply, &record) != nil {
		return
	}
	ctx.ReleaseClaimForTest(reductionID, record.BatchID, key, claimID)
}

// PlanForTest reads a Reduction's landed plan back as the in-memory document the
// build produces: every Cluster of the plan generation in plan order with its
// members, the stored coverage and settings, and the last-apply record. Tests read
// decisions through it rather than through the rows' columns.
func (ctx *MahresourcesContext) PlanForTest(reductionID uint) (models.ResourceReductionPlan, error) {
	var plan models.ResourceReductionPlan
	s := ctx.reductionStore()
	var r models.ResourceReduction
	if err := s.db.Select(reductionColumns).First(&r, reductionID).Error; err != nil {
		return plan, err
	}
	if r.PlanGeneration != nil {
		var rows []models.ReductionClusterRow
		if err := s.db.Where("reduction_id = ? AND generation = ?", reductionID, *r.PlanGeneration).Order("position, id").Find(&rows).Error; err != nil {
			return plan, err
		}
		for i := range rows {
			members, err := s.loadMembers(rows[i].ID, 1<<20)
			if err != nil {
				return plan, err
			}
			plan.Clusters = append(plan.Clusters, toPlanCluster(&storedCluster{Row: rows[i], Members: members}))
		}
	}
	if _, err := decodeStoredJSON(r.Coverage, &plan.Coverage); err != nil {
		return plan, err
	}
	var settings models.ReductionSettings
	if ok, err := decodeStoredJSON(r.PlanSettings, &settings); err != nil {
		return plan, err
	} else if ok {
		plan.Settings = &settings
	}
	var last models.ReductionLastApply
	if ok, err := decodeStoredJSON(r.LastApply, &last); err != nil {
		return plan, err
	} else if ok {
		plan.LastApply = &last
	}
	return plan, nil
}

// ExtentForTest reads a Reduction's selection rows back as the Extent document.
func (ctx *MahresourcesContext) ExtentForTest(reductionID uint) (models.ResourceReductionExtent, error) {
	r := &models.ResourceReduction{ID: reductionID}
	if err := ctx.reductionStore().db.Select("extent_all").First(r, reductionID).Error; err != nil {
		return models.ResourceReductionExtent{}, err
	}
	return ctx.storedReductionExtent(r)
}

// BeginComputeForTest puts a Reduction into computing the way a requested compute
// does (version bumped, generation reserved) without starting a worker, so a test
// can land a plan, or try an edit, against a compute that is notionally running. It
// returns the job id the landing is fenced on.
func (ctx *MahresourcesContext) BeginComputeForTest(reductionID, version uint) (jobID string, err error) {
	jobID, _, err = ctx.beginReductionComputeStore(reductionID, version)
	return jobID, err
}

// HoldApplyLeaseForTest makes a synthetic apply batch the Reduction's recorded
// apply: its last-apply record names the batch, and its lease is live (a batch
// running now, which refuses a second apply) or expired (one that died, which the
// next apply replaces). It does not bump the version.
func (ctx *MahresourcesContext) HoldApplyLeaseForTest(reductionID uint, live bool) error {
	s := ctx.reductionStore()
	var r struct{ PlanGeneration *string }
	if err := s.db.Raw(`SELECT plan_generation FROM resource_reductions WHERE id = ?`, reductionID).Scan(&r).Error; err != nil {
		return err
	}
	generation := ""
	if r.PlanGeneration != nil {
		generation = *r.PlanGeneration
	}
	record := fmt.Sprintf(`{"batchId":%q,"generation":%q,"startedAt":%q,"applied":0,"removed":0,"stale":0}`, "held-"+newClaimID(), generation, time.Now().UTC().Format(time.RFC3339))
	lease := int64(1)
	if live {
		lease = time.Now().Add(time.Minute).UnixNano()
	}
	return s.db.Exec(`UPDATE resource_reductions SET last_apply = ?, apply_lease_until = ? WHERE id = ?`, record, lease, reductionID).Error
}
