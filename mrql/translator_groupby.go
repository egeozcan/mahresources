package mrql

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// MaxBuckets is the maximum number of distinct group keys allowed in bucketed mode.
const MaxBuckets = 1000

// TranslateGroupByKeys executes a SELECT DISTINCT query to get unique bucket keys.
// Returns a slice of maps, each containing the group-by field values for one bucket.
func TranslateGroupByKeys(q *Query, db *gorm.DB) ([]map[string]any, error) {
	if q.GroupBy == nil || len(q.GroupBy.Aggregates) > 0 {
		return nil, &TranslateError{Message: "TranslateGroupByKeys requires GROUP BY without aggregates", Pos: 0}
	}

	entityType := q.EntityType
	if entityType == EntityUnspecified {
		entityType = ExtractEntityType(q)
	}
	if entityType == EntityUnspecified {
		return nil, &TranslateError{Message: "entity type is required for GROUP BY", Pos: 0}
	}

	tc := &translateContext{
		db:         db,
		entityType: entityType,
		tableName:  entityTableName(entityType),
	}

	result := db.Table(tc.tableName)

	// Apply WHERE clause
	if q.Where != nil {
		var err error
		result, err = tc.translateNode(result, q.Where)
		if err != nil {
			return nil, err
		}
	}

	// Add JOINs for relation fields
	var relationExprs map[string]groupByRelExpr
	result, relationExprs = tc.groupByRelationJoins(result, q.GroupBy.Fields)

	// Build SELECT DISTINCT for group-by fields.
	// For relation fields with non-unique names (owner, groups), we SELECT both
	// the display name and the identity (FK/ID) so bucket filtering can match by ID.
	var selectCols []string
	var groupCols []string
	for _, f := range q.GroupBy.Fields {
		fieldName := f.Name()
		if rel, ok := relationExprs[fieldName]; ok {
			if rel.selectExpr != rel.groupExpr {
				// PostgreSQL: wrap display column in MAX() since it's not in GROUP BY
				selectCols = append(selectCols, "MAX("+rel.selectExpr+`) AS "`+fieldName+`"`)
				selectCols = append(selectCols, rel.groupExpr+` AS "_gbid_`+fieldName+`"`)
			} else {
				selectCols = append(selectCols, rel.selectExpr+` AS "`+fieldName+`"`)
			}
			groupCols = append(groupCols, rel.groupExpr)
		} else {
			selectExpr, groupExpr := tc.groupByFieldExprs(fieldName)
			selectCols = append(selectCols, selectExpr+` AS "`+fieldName+`"`)
			groupCols = append(groupCols, groupExpr)
		}
	}

	result = result.Select(strings.Join(selectCols, ", "))
	for _, gc := range groupCols {
		result = result.Group(gc)
	}

	// Deterministic ORDER BY on group columns so paged key queries are stable.
	// This is safe on PostgreSQL because we only order by expressions already
	// in the GROUP BY clause.
	for _, gc := range groupCols {
		result = result.Order(gc)
	}

	// BucketLimit controls how many group keys per page. When not set,
	// cap at MaxBuckets. Never exceed MaxBuckets regardless of caller input.
	// Fetch one extra to detect truncation.
	keyLimit := MaxBuckets
	if q.BucketLimit >= 0 && q.BucketLimit < MaxBuckets {
		keyLimit = q.BucketLimit
	}
	result = result.Limit(keyLimit + 1)

	if q.Offset >= 0 {
		result = result.Offset(q.Offset)
	}

	var keys []map[string]any
	if err := result.Find(&keys).Error; err != nil {
		return nil, err
	}

	// Strip internal _gbid_ keys from the public key maps — they're used
	// internally by TranslateGroupByBucket but shouldn't appear in API responses.
	// Note: we keep them in the maps so the caller (executeBucketedQuery) passes
	// them through to TranslateGroupByBucket. The execution layer strips them
	// from the MRQLBucket.Key before serialization.
	return keys, nil
}

// TranslateGroupByBucket returns a GORM DB scoped to a specific bucket.
// The key map contains group-by field names mapped to their values.
// The caller should use the returned DB to Find entities and apply LIMIT.
func TranslateGroupByBucket(q *Query, db *gorm.DB, key map[string]any) (*gorm.DB, error) {
	entityType := q.EntityType
	if entityType == EntityUnspecified {
		entityType = ExtractEntityType(q)
	}
	if entityType == EntityUnspecified {
		return nil, &TranslateError{Message: "entity type is required for GROUP BY", Pos: 0}
	}

	tc := &translateContext{
		db:         db,
		entityType: entityType,
		tableName:  entityTableName(entityType),
	}

	// Use DISTINCT to prevent duplicates when relation JOINs multiply rows.
	result := db.Table(tc.tableName).Distinct(tc.tableName + ".*")

	// Apply WHERE clause from the original query
	if q.Where != nil {
		var err error
		result, err = tc.translateNode(result, q.Where)
		if err != nil {
			return nil, err
		}
	}

	// Add JOINs for relation fields
	var relationExprs map[string]groupByRelExpr
	result, relationExprs = tc.groupByRelationJoins(result, q.GroupBy.Fields)

	// Add bucket key constraints. For relation fields with separate identity
	// expressions (owner, groups), prefer the _gbid_ key for filtering to avoid
	// ambiguity when multiple groups share the same display name.
	for _, f := range q.GroupBy.Fields {
		fieldName := f.Name()
		var expr string
		var val any
		if rel, ok := relationExprs[fieldName]; ok {
			if rel.selectExpr != rel.groupExpr {
				// Use the identity (FK/ID) for filtering
				expr = rel.groupExpr
				val = key["_gbid_"+fieldName]
			} else {
				expr = rel.selectExpr
				val = key[fieldName]
			}
		} else {
			_, expr = tc.groupByFieldExprs(fieldName)
			val = key[fieldName]
		}

		if val == nil {
			result = result.Where(expr + " IS NULL")
		} else {
			result = result.Where(expr+" = ?", val)
		}
	}

	// Apply per-bucket LIMIT
	if q.Limit >= 0 {
		result = result.Limit(q.Limit)
	}

	// Apply ORDER BY (within bucket)
	for _, ob := range q.OrderBy {
		col := tc.resolveOrderByColumn(ob.Field)
		direction := "ASC"
		if !ob.Ascending {
			direction = "DESC"
		}
		result = result.Order(fmt.Sprintf("%s %s", col, direction))
	}

	return result, nil
}
