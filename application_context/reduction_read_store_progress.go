package application_context

import (
	"time"

	"gorm.io/gorm"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
)

// The table-backed list reader (design D8): a page of Reductions, their count and
// each one's progress, from one snapshot, one GROUP BY per Reduction. Beside the
// JSON reader in resource_reduction_progress.go and not wired.

// reductionListProgress is the progress of the Reductions on a list page. A
// Reduction whose plan is still the legacy document is in Upgrading and has no
// progress: the page renders it as "being upgraded" instead of failing.
type reductionListProgress struct {
	Progress  map[uint]contracts.ReductionProgress
	Upgrading map[uint]bool
}

// progress counts one Reduction's Clusters by effective state, over the ones the
// principal may fully see: a pending Cluster naming a deleted member reads as
// stale, and an approved one is checked with a copy to remove.
func (s *reductionStore) progress(r *models.ResourceReduction, generation string) (contracts.ReductionProgress, error) {
	prefix, where, args := s.base(r, generation)
	eff := effectiveState("c")
	var rows []struct {
		State    string
		Merged   bool
		Approved bool
		N        int64
	}
	if err := s.db.Raw(prefix+"SELECT "+eff+" AS state, c.merged AS merged, "+approved("c")+" AS approved, COUNT(*) AS n FROM reduction_clusters c WHERE "+where+
		" GROUP BY "+eff+", c.merged, "+approved("c"), args...).Scan(&rows).Error; err != nil {
		return contracts.ReductionProgress{}, err
	}
	var out contracts.ReductionProgress
	for _, row := range rows {
		n := int(row.N)
		out.Clusters += n
		switch row.State {
		case models.ReductionClusterOpen:
			out.Open += n
			if row.Approved {
				out.Approved += n
			}
		case models.ReductionClusterSkipped:
			out.Skipped += n
		case models.ReductionClusterStale:
			out.Stale += n
		case models.ReductionClusterApplied:
			if row.Merged {
				out.Applied += n
			} else {
				out.Merging += n
			}
		}
	}
	return out, nil
}

// storeGetReductionProgress is GetReductionProgress over the plan tables, in one
// snapshot. The rows the caller holds were read earlier, so each one's plan
// schema and generation are re-read here, inside the snapshot.
func (ctx *MahresourcesContext) storeGetReductionProgress(reductions []models.ResourceReduction) (*reductionListProgress, error) {
	var out *reductionListProgress
	err := ctx.withReadSnapshot(func(snap *MahresourcesContext) error {
		var err error
		out, err = snap.storeProgressInSnapshot(reductions)
		return err
	})
	return out, err
}

func (ctx *MahresourcesContext) storeProgressInSnapshot(reductions []models.ResourceReduction) (*reductionListProgress, error) {
	out := &reductionListProgress{Progress: make(map[uint]contracts.ReductionProgress, len(reductions)), Upgrading: map[uint]bool{}}
	if len(reductions) == 0 {
		return out, nil
	}
	ids := make([]uint, len(reductions))
	for i := range reductions {
		ids[i] = reductions[i].ID
	}
	var fresh []models.ResourceReduction
	if err := ctx.db.Select("id, plan_schema, plan_generation, apply_lease_until").Where("id IN ?", ids).Find(&fresh).Error; err != nil {
		return nil, err
	}
	store := ctx.reductionStore()
	for i := range fresh {
		r := &fresh[i]
		if r.PlanSchema == 0 {
			out.Upgrading[r.ID] = true
			continue
		}
		progress, err := store.progress(r, reductionGenerationOf(r))
		if err != nil {
			return nil, err
		}
		out.Progress[r.ID] = progress
	}
	return out, nil
}

// storeListReductions reads a list page, the count under the same predicate and
// the page's progress in one snapshot, so the three cannot disagree.
func (ctx *MahresourcesContext) storeListReductions(offset, limit int, query *query_models.ResourceReductionQuery) ([]models.ResourceReduction, int64, *reductionListProgress, error) {
	return ctx.storeListReductionsAt(time.Now(), offset, limit, query)
}

// reductionStatusScope is the list's status filter judged against now. The shared
// scope reads the clock itself, once per statement, and the rows and the count are
// two statements: a Reduction whose computing deadline passes between them would be
// on the page and not in the count, or the reverse. The reader strips the filter
// from the shared scope and applies it here with the one instant it was given. The
// rule is the shared scope's: a computing Reduction past its deadline reads as
// failed, in both filters.
func reductionStatusScope(statuses []string, now time.Time) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		wantsFailed, wantsComputing := false, false
		var wanted []string
		for _, status := range statuses {
			if status == "" {
				continue
			}
			wanted = append(wanted, status)
			switch status {
			case models.ReductionStatusFailed:
				wantsFailed = true
			case models.ReductionStatusComputing:
				wantsComputing = true
			}
		}
		if len(wanted) == 0 {
			return db
		}
		expired := "status = 'computing' AND compute_deadline IS NOT NULL AND compute_deadline < ?"
		switch {
		case wantsFailed && !wantsComputing:
			return db.Where("status IN ? OR ("+expired+")", wanted, now)
		case wantsComputing && !wantsFailed:
			return db.Where("status IN ? AND NOT ("+expired+")", wanted, now)
		}
		return db.Where("status IN ?", wanted)
	}
}

// storeListReductionsAt is storeListReductions with the instant a computing
// deadline is judged against given once, for the rows and the count both.
func (ctx *MahresourcesContext) storeListReductionsAt(now time.Time, offset, limit int, query *query_models.ResourceReductionQuery) ([]models.ResourceReduction, int64, *reductionListProgress, error) {
	var rows []models.ResourceReduction
	var count int64
	var progress *reductionListProgress
	rest := *query
	rest.Status = nil
	err := ctx.withReadSnapshot(func(snap *MahresourcesContext) error {
		if err := snap.db.Select(snap.reductionStore().boundedReductionColumns()).
			Scopes(database_scopes.ResourceReductionQuery(&rest, false), reductionStatusScope(query.Status, now)).
			Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			rows[i].Name = trimDisplayText([]byte(rows[i].Name))
		}
		if err := snap.db.Model(&models.ResourceReduction{}).
			Scopes(database_scopes.ResourceReductionQuery(&rest, true), reductionStatusScope(query.Status, now)).
			Count(&count).Error; err != nil {
			return err
		}
		var err error
		progress, err = snap.storeProgressInSnapshot(rows)
		return err
	})
	return rows, count, progress, err
}
