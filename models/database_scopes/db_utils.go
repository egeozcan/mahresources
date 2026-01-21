package database_scopes

import (
	"regexp"
	"strings"

	"gorm.io/gorm"
)

// SortColumnMatcher validates sort column strings to prevent SQL injection.
// Matches: column_name, column_name desc, column_name asc, meta->>'key', meta->>'key' desc
var SortColumnMatcher = regexp.MustCompile(`^(meta->>?'[a-z_]+'|[a-z_]+)(\s(desc|asc))?$`)

// GetLikeOperator returns "ILIKE" for Postgres (case-insensitive), "LIKE" for others.
func GetLikeOperator(db *gorm.DB) string {
	if db.Config.Dialector.Name() == "postgres" {
		return "ILIKE"
	}
	return "LIKE"
}

// ValidateSortColumn checks if a sort string is safe for use in ORDER BY clauses.
func ValidateSortColumn(sort string) bool {
	return sort != "" && SortColumnMatcher.MatchString(sort)
}

// ApplyDateRange adds created_at filters for the given column prefix if provided.
// The prefix should be empty string for simple table queries, or "tablename." for joined queries.
func ApplyDateRange(db *gorm.DB, prefix, before, after string) *gorm.DB {
	if before != "" {
		db = db.Where(prefix+"created_at <= ?", before)
	}
	if after != "" {
		db = db.Where(prefix+"created_at >= ?", after)
	}
	return db
}

// ApplySortColumns validates and applies multiple ORDER BY clauses.
// tablePrefix should be "tablename." for joined queries, or empty string for simple queries.
// defaultSort is applied as the final tiebreaker sort (e.g., "created_at desc").
func ApplySortColumns(db *gorm.DB, sortBy []string, tablePrefix, defaultSort string) *gorm.DB {
	for _, sort := range sortBy {
		sort = strings.TrimSpace(sort)
		if !ValidateSortColumn(sort) {
			continue
		}

		// Add table prefix for non-meta columns
		if tablePrefix != "" && !strings.HasPrefix(sort, "meta") {
			parts := strings.SplitN(sort, " ", 2)
			prefixedSort := tablePrefix + parts[0]
			if len(parts) > 1 {
				prefixedSort += " " + parts[1]
			}
			db = db.Order(prefixedSort)
		} else {
			db = db.Order(sort)
		}
	}

	// Apply default sort as final tiebreaker
	if defaultSort != "" {
		db = db.Order(defaultSort)
	}

	return db
}
