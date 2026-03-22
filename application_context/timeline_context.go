package application_context

import (
	"fmt"
	"time"

	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
)

// BucketBoundary defines the time boundaries for a single timeline bucket.
type BucketBoundary struct {
	Label string
	Start time.Time // inclusive
	End   time.Time // exclusive
}

// GenerateBucketBoundaries creates a slice of time-based bucket boundaries.
// The rightmost bucket contains the anchor date. Buckets are ordered oldest-first.
// If columns is 0, it defaults to 15. Invalid granularity defaults to "monthly".
func GenerateBucketBoundaries(granularity string, anchor time.Time, columns int) []BucketBoundary {
	if columns <= 0 {
		columns = 15
	}

	switch granularity {
	case "yearly", "monthly", "weekly":
		// valid
	default:
		granularity = "monthly"
	}

	// Compute the start of the bucket containing the anchor
	anchorBucketStart := bucketStart(granularity, anchor)

	// Build buckets from oldest to newest
	boundaries := make([]BucketBoundary, columns)
	for i := 0; i < columns; i++ {
		// offset from the rightmost bucket: rightmost is index (columns-1)
		offset := columns - 1 - i
		start := subtractBuckets(granularity, anchorBucketStart, offset)
		end := addOneBucket(granularity, start)
		boundaries[i] = BucketBoundary{
			Label: bucketLabel(granularity, start),
			Start: start,
			End:   end,
		}
	}

	return boundaries
}

// bucketStart returns the start of the bucket containing t.
func bucketStart(granularity string, t time.Time) time.Time {
	switch granularity {
	case "yearly":
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	case "weekly":
		return mondayOf(t)
	default: // monthly
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
}

// mondayOf returns the Monday at or before t.
func mondayOf(t time.Time) time.Time {
	weekday := t.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	daysBack := int(weekday) - int(time.Monday)
	monday := t.AddDate(0, 0, -daysBack)
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}

// subtractBuckets moves n buckets back from start.
func subtractBuckets(granularity string, start time.Time, n int) time.Time {
	switch granularity {
	case "yearly":
		return start.AddDate(-n, 0, 0)
	case "weekly":
		return start.AddDate(0, 0, -7*n)
	default: // monthly
		return start.AddDate(0, -n, 0)
	}
}

// addOneBucket returns the start of the next bucket after start.
func addOneBucket(granularity string, start time.Time) time.Time {
	switch granularity {
	case "yearly":
		return start.AddDate(1, 0, 0)
	case "weekly":
		return start.AddDate(0, 0, 7)
	default: // monthly
		return start.AddDate(0, 1, 0)
	}
}

// bucketLabel returns the display label for a bucket starting at start.
func bucketLabel(granularity string, start time.Time) string {
	switch granularity {
	case "yearly":
		return fmt.Sprintf("%d", start.Year())
	case "weekly":
		return start.Format("Jan 02")
	default: // monthly
		return start.Format("2006-01")
	}
}

// fillBuckets converts BucketBoundary slices into TimelineBucket slices
// with zero counts, ready to be populated by count queries.
func fillBuckets(boundaries []BucketBoundary) []models.TimelineBucket {
	buckets := make([]models.TimelineBucket, len(boundaries))
	for i, b := range boundaries {
		buckets[i] = models.TimelineBucket{
			Label: b.Label,
			Start: b.Start,
			End:   b.End,
		}
	}
	return buckets
}

// --- Per-entity timeline count methods ---

// GetResourceTimelineCounts returns timeline bucket counts for resources matching the given query.
func (ctx *MahresourcesContext) GetResourceTimelineCounts(
	query *query_models.ResourceSearchQuery,
	boundaries []BucketBoundary,
) ([]models.TimelineBucket, error) {
	buckets := fillBuckets(boundaries)

	for i, b := range boundaries {
		var createdCount int64
		err := ctx.db.Scopes(database_scopes.ResourceQuery(query, true, ctx.db)).
			Model(&models.Resource{}).
			Where("resources.created_at >= ? AND resources.created_at < ?", b.Start, b.End).
			Count(&createdCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting created resources for bucket %q: %w", b.Label, err)
		}
		buckets[i].Created = createdCount

		var updatedCount int64
		err = ctx.db.Scopes(database_scopes.ResourceQuery(query, true, ctx.db)).
			Model(&models.Resource{}).
			Where("resources.updated_at >= ? AND resources.updated_at < ?", b.Start, b.End).
			Where("resources.updated_at > resources.created_at").
			Count(&updatedCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting updated resources for bucket %q: %w", b.Label, err)
		}
		buckets[i].Updated = updatedCount
	}

	return buckets, nil
}

// GetNoteTimelineCounts returns timeline bucket counts for notes matching the given query.
func (ctx *MahresourcesContext) GetNoteTimelineCounts(
	query *query_models.NoteQuery,
	boundaries []BucketBoundary,
) ([]models.TimelineBucket, error) {
	buckets := fillBuckets(boundaries)

	for i, b := range boundaries {
		var createdCount int64
		err := ctx.db.Scopes(database_scopes.NoteQuery(query, true, ctx.db)).
			Model(&models.Note{}).
			Where("notes.created_at >= ? AND notes.created_at < ?", b.Start, b.End).
			Count(&createdCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting created notes for bucket %q: %w", b.Label, err)
		}
		buckets[i].Created = createdCount

		var updatedCount int64
		err = ctx.db.Scopes(database_scopes.NoteQuery(query, true, ctx.db)).
			Model(&models.Note{}).
			Where("notes.updated_at >= ? AND notes.updated_at < ?", b.Start, b.End).
			Where("notes.updated_at > notes.created_at").
			Count(&updatedCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting updated notes for bucket %q: %w", b.Label, err)
		}
		buckets[i].Updated = updatedCount
	}

	return buckets, nil
}

// GetGroupTimelineCounts returns timeline bucket counts for groups matching the given query.
func (ctx *MahresourcesContext) GetGroupTimelineCounts(
	query *query_models.GroupQuery,
	boundaries []BucketBoundary,
) ([]models.TimelineBucket, error) {
	buckets := fillBuckets(boundaries)

	for i, b := range boundaries {
		var createdCount int64
		err := ctx.db.Scopes(database_scopes.GroupQuery(query, true, ctx.db)).
			Model(&models.Group{}).
			Where("groups.created_at >= ? AND groups.created_at < ?", b.Start, b.End).
			Count(&createdCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting created groups for bucket %q: %w", b.Label, err)
		}
		buckets[i].Created = createdCount

		var updatedCount int64
		err = ctx.db.Scopes(database_scopes.GroupQuery(query, true, ctx.db)).
			Model(&models.Group{}).
			Where("groups.updated_at >= ? AND groups.updated_at < ?", b.Start, b.End).
			Where("groups.updated_at > groups.created_at").
			Count(&updatedCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting updated groups for bucket %q: %w", b.Label, err)
		}
		buckets[i].Updated = updatedCount
	}

	return buckets, nil
}

// GetTagTimelineCounts returns timeline bucket counts for tags matching the given query.
func (ctx *MahresourcesContext) GetTagTimelineCounts(
	query *query_models.TagQuery,
	boundaries []BucketBoundary,
) ([]models.TimelineBucket, error) {
	buckets := fillBuckets(boundaries)

	for i, b := range boundaries {
		var createdCount int64
		err := ctx.db.Scopes(database_scopes.TagQuery(query, true)).
			Model(&models.Tag{}).
			Where("created_at >= ? AND created_at < ?", b.Start, b.End).
			Count(&createdCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting created tags for bucket %q: %w", b.Label, err)
		}
		buckets[i].Created = createdCount

		var updatedCount int64
		err = ctx.db.Scopes(database_scopes.TagQuery(query, true)).
			Model(&models.Tag{}).
			Where("updated_at >= ? AND updated_at < ?", b.Start, b.End).
			Where("updated_at > created_at").
			Count(&updatedCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting updated tags for bucket %q: %w", b.Label, err)
		}
		buckets[i].Updated = updatedCount
	}

	return buckets, nil
}

// GetCategoryTimelineCounts returns timeline bucket counts for categories matching the given query.
func (ctx *MahresourcesContext) GetCategoryTimelineCounts(
	query *query_models.CategoryQuery,
	boundaries []BucketBoundary,
) ([]models.TimelineBucket, error) {
	buckets := fillBuckets(boundaries)

	for i, b := range boundaries {
		var createdCount int64
		err := ctx.db.Scopes(database_scopes.CategoryQuery(query, true)).
			Model(&models.Category{}).
			Where("created_at >= ? AND created_at < ?", b.Start, b.End).
			Count(&createdCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting created categories for bucket %q: %w", b.Label, err)
		}
		buckets[i].Created = createdCount

		var updatedCount int64
		err = ctx.db.Scopes(database_scopes.CategoryQuery(query, true)).
			Model(&models.Category{}).
			Where("updated_at >= ? AND updated_at < ?", b.Start, b.End).
			Where("updated_at > created_at").
			Count(&updatedCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting updated categories for bucket %q: %w", b.Label, err)
		}
		buckets[i].Updated = updatedCount
	}

	return buckets, nil
}

// GetQueryTimelineCounts returns timeline bucket counts for queries matching the given query.
func (ctx *MahresourcesContext) GetQueryTimelineCounts(
	query *query_models.QueryQuery,
	boundaries []BucketBoundary,
) ([]models.TimelineBucket, error) {
	buckets := fillBuckets(boundaries)

	for i, b := range boundaries {
		var createdCount int64
		err := ctx.db.Scopes(database_scopes.QueryQuery(query, true)).
			Model(&models.Query{}).
			Where("created_at >= ? AND created_at < ?", b.Start, b.End).
			Count(&createdCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting created queries for bucket %q: %w", b.Label, err)
		}
		buckets[i].Created = createdCount

		var updatedCount int64
		err = ctx.db.Scopes(database_scopes.QueryQuery(query, true)).
			Model(&models.Query{}).
			Where("updated_at >= ? AND updated_at < ?", b.Start, b.End).
			Where("updated_at > created_at").
			Count(&updatedCount).Error
		if err != nil {
			return nil, fmt.Errorf("counting updated queries for bucket %q: %w", b.Label, err)
		}
		buckets[i].Updated = updatedCount
	}

	return buckets, nil
}
