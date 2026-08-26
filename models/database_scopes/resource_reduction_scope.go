package database_scopes

import (
	"time"

	"gorm.io/gorm"
	"mahresources/models/query_models"
)

// ResourceReductionQuery returns a GORM scope filtering Resource Reductions.
//
// The owner predicate lives here rather than in the handlers, following
// DownloadHistoryQuery for the same reason: a listing and the mutations it leads
// to drift apart the moment the predicate is written twice, and here the drift
// would mean editing or applying somebody else's pending destructive decision.
//
// This is not the saved-query pattern, which is global. A saved query is
// read-only and shared; a Reduction is a proposal to delete files, held by the
// one person reviewing it.
func ResourceReductionQuery(query *query_models.ResourceReductionQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if !ignoreSort {
			// Newest first. Two Reductions with similar names are told apart by
			// their creation date and time, so that is what the list leads with.
			dbQuery = ApplySortColumns(dbQuery, query.SortBy, "", "created_at desc")
		}

		if query.Name != "" {
			p, esc := LikePattern(query.Name)
			dbQuery = dbQuery.Where("name "+likeOperator+" ?"+esc, p)
		}

		if statuses := nonEmpty(query.Status); len(statuses) > 0 {
			// A Reduction still `computing` past its deadline *reads* as failed —
			// generic queue jobs are not drained at shutdown, so nothing else would
			// ever move it off that status. The filter has to agree with the label,
			// or the one status a reviewer would search for to find a stranded run
			// is the one that hides it.
			wantsFailed := false
			for _, status := range statuses {
				if status == "failed" {
					wantsFailed = true
				}
			}
			if wantsFailed {
				dbQuery = dbQuery.Where("status IN ? OR (status = ? AND compute_deadline IS NOT NULL AND compute_deadline < ?)",
					statuses, "computing", time.Now())
			} else {
				dbQuery = dbQuery.Where("status IN ?", statuses)
			}
		}

		if query.OwnerRestricted {
			// Fail-closed: a NULL owner is nobody's, which is what a deleted user
			// leaves behind. Administrators are the only principals that reach
			// this with OwnerRestricted unset.
			if query.OwnerUserID == nil {
				dbQuery = dbQuery.Where("1 = 0")
			} else {
				dbQuery = dbQuery.Where("created_by_user_id = ?", *query.OwnerUserID)
			}
		}

		return dbQuery
	}
}
