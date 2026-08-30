package database_scopes

import (
	"strings"

	"gorm.io/gorm"
	"mahresources/models/query_models"
)

// DownloadHistoryQuery returns a GORM scope filtering the download history.
//
// The owner predicate is part of the scope rather than something the handler
// bolts on afterwards, so every read and every mutation goes through one place:
// a listing that forgets it leaks other users' download URLs, and a delete that
// forgets it removes their rows. OwnerRestricted is set from the principal, never
// decoded from the request.
func DownloadHistoryQuery(query *query_models.DownloadHistoryQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if !ignoreSort {
			// Newest first, matching the jobs panel. completed_at would order the
			// history by when each attempt ended; created_at orders it by when the
			// user asked for the download, which is what they are looking for.
			dbQuery = ApplySortColumns(dbQuery, query.SortBy, "", "created_at desc")
		}

		if statuses := nonEmpty(query.Status); len(statuses) > 0 {
			dbQuery = dbQuery.Where("status IN ?", statuses)
		}

		if query.URL != "" {
			// One box over both columns: the user searches for "that pdf", and
			// whether the word they remember was in the file name or in the URL is
			// not a distinction worth making them draw.
			if LikeTermIsUnmatchable(query.URL) {
				dbQuery = dbQuery.Where("1 = 0")
			} else {
				p, esc := LikePattern(query.URL)
				dbQuery = dbQuery.Where("(url "+likeOperator+" ?"+esc+" OR name "+likeOperator+" ?"+esc+")", p, p)
			}
		}

		// "Has been retried" covers both shapes a rerun takes: an in-place retry,
		// which bumps attempts on the row it already has, and a resubmission,
		// which starts a new job and links back through last_retry_job_id. The
		// transient "claiming-" marker a retry writes before it submits counts as
		// retried too — a claim means a rerun was initiated, and a stale one is
		// the record of an attempt that died mid-submit. COALESCE, because the
		// column is nullable for rows written before it existed, and a plain
		// `<> ''` would drop those from *both* answers rather than one.
		switch query.Retried {
		case query_models.DownloadRetriedYes:
			dbQuery = dbQuery.Where("(attempts > 1 OR COALESCE(last_retry_job_id, '') <> '')")
		case query_models.DownloadRetriedNo:
			dbQuery = dbQuery.Where("(attempts <= 1 AND COALESCE(last_retry_job_id, '') = '')")
		}

		if query.Error != "" {
			// The same shape as the URL box, over the column that holds the failure:
			// the category filter above narrows to a kind of failure, this narrows to
			// a particular one, and they are AND-ed.
			if LikeTermIsUnmatchable(query.Error) {
				dbQuery = dbQuery.Where("1 = 0")
			} else {
				p, esc := LikePattern(query.Error)
				dbQuery = dbQuery.Where("error "+likeOperator+" ?"+esc, p)
			}
		}

		dbQuery = applyDownloadReason(dbQuery, likeOperator, query.Reason)

		if query.OwnerRestricted {
			// Fail-closed: a NULL owner is nobody's, so a non-admin sees none of
			// them. Matches jobVisibleToPrincipal, which the in-memory queue and the
			// SSE stream already apply.
			if query.OwnerUserID == nil {
				dbQuery = dbQuery.Where("1 = 0")
			} else {
				dbQuery = dbQuery.Where("created_by_user_id = ?", *query.OwnerUserID)
			}
		}

		dbQuery = ApplyDateRange(dbQuery, "", query.CreatedBefore, query.CreatedAfter)
		dbQuery = ApplyCompletedDateRange(dbQuery, "", query.CompletedBefore, query.CompletedAfter)

		return dbQuery
	}
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// applyDownloadReason narrows the listing to one failure-reason bucket.
//
// The buckets are query_models.DownloadFailureReasons, and "other" is their
// complement rather than a bucket of its own: it keeps every row that carries an
// error no bucket claims, which is what makes it a useful place to look for a
// failure the classification has not learned about yet. A row with no error text
// belongs to no reason at all — a completed download is not an unclassified
// failure — so it is excluded from "other" too.
//
// An unrecognised key matches everything, the same lenience the Retried filter
// applies to input it does not recognise.
func applyDownloadReason(db *gorm.DB, likeOperator, reason string) *gorm.DB {
	if reason == "" {
		return db
	}

	for _, bucket := range query_models.DownloadFailureReasons {
		if bucket.Key != reason {
			continue
		}
		clause, args := likeAnyOf(likeOperator, "error", bucket.Patterns)
		if clause == "" {
			return db
		}
		return db.Where(clause, args...)
	}

	if reason != query_models.DownloadReasonOther {
		return db
	}

	var all []string
	for _, bucket := range query_models.DownloadFailureReasons {
		all = append(all, bucket.Patterns...)
	}
	clause, args := likeAnyOf(likeOperator, "error", all)
	if clause == "" {
		return db.Where("error <> ''")
	}
	return db.Where("error <> '' AND NOT "+clause, args...)
}

// likeAnyOf builds `(col LIKE ? ESCAPE ... OR col LIKE ? ESCAPE ...)` over the
// given patterns. They are developer-authored constants carrying their own
// wildcards — unlike a user's search term, which LikePattern escapes — so they
// are bound as parameters rather than concatenated, and the ESCAPE clause is
// emitted so one can escape a wildcard of its own if it ever needs to.
func likeAnyOf(likeOperator, column string, patterns []string) (string, []any) {
	if len(patterns) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(patterns))
	args := make([]any, 0, len(patterns))
	for _, pattern := range patterns {
		parts = append(parts, column+" "+likeOperator+" ?"+likeEscapeClause)
		args = append(args, pattern)
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}
