package mrql

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// maxBuckets is the maximum number of distinct group keys allowed in bucketed mode.
const maxBuckets = 1000

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
	var relationExprs map[string]string
	result, relationExprs = tc.groupByRelationJoins(result, q.GroupBy.Fields)

	// Build SELECT DISTINCT for group-by fields
	var selectCols []string
	var groupCols []string
	for _, f := range q.GroupBy.Fields {
		fieldName := f.Name()
		if relExpr, ok := relationExprs[fieldName]; ok {
			selectCols = append(selectCols, relExpr+` AS "`+fieldName+`"`)
			groupCols = append(groupCols, relExpr)
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

	// ORDER BY for keys
	for _, ob := range q.OrderBy {
		direction := "ASC"
		if !ob.Ascending {
			direction = "DESC"
		}
		result = result.Order(`"` + ob.Field.Name() + `" ` + direction)
	}

	// Cap buckets
	result = result.Limit(maxBuckets)

	var keys []map[string]any
	if err := result.Find(&keys).Error; err != nil {
		return nil, err
	}
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

	result := db.Table(tc.tableName)

	// Apply WHERE clause from the original query
	if q.Where != nil {
		var err error
		result, err = tc.translateNode(result, q.Where)
		if err != nil {
			return nil, err
		}
	}

	// Add JOINs for relation fields
	var relationExprs map[string]string
	result, relationExprs = tc.groupByRelationJoins(result, q.GroupBy.Fields)

	// Add bucket key constraints
	for _, f := range q.GroupBy.Fields {
		fieldName := f.Name()
		val := key[fieldName]
		var expr string
		if relExpr, ok := relationExprs[fieldName]; ok {
			expr = relExpr
		} else {
			_, expr = tc.groupByFieldExprs(fieldName)
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
