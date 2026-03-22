package database_scopes

import (
	"regexp"
	"strings"

	"gorm.io/gorm"
)

// SortColumnMatcher validates sort column strings to prevent SQL injection.
// Matches: column_name, column_name desc, column_name asc, meta->>'key', meta->>'key' desc
var SortColumnMatcher = regexp.MustCompile(`^(meta->>?'[a-z_]+'|[a-z_]+)(\s(desc|asc))?$`)

// metaSortMatcher extracts the key from meta sort expressions like meta->>'key_name'
var metaSortMatcher = regexp.MustCompile(`^meta->>?'([a-z_]+)'(\s+(desc|asc))?$`)

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

// convertMetaSortForSQLite converts meta->>'key' to json_extract(meta, '$.key') for SQLite.
// SQLite 3.38+ supports ->> but older versions (like the one bundled with go-sqlite3) don't.
// tablePrefix (e.g., "groups.") is prepended to disambiguate the meta column in JOINed queries.
func convertMetaSortForSQLite(sort, tablePrefix string) string {
	matches := metaSortMatcher.FindStringSubmatch(sort)
	if matches == nil {
		return sort
	}
	// matches[1] is the key name, matches[2] is the direction (with leading space) or empty
	key := matches[1]
	direction := strings.TrimSpace(matches[2])
	result := "json_extract(" + tablePrefix + "meta, '$." + key + "')"
	if direction != "" {
		result += " " + direction
	}
	return result
}

// LikePattern builds a LIKE pattern with proper escaping of wildcard characters.
// Returns the escaped pattern and the ESCAPE clause suffix to append to the LIKE expression.
func LikePattern(term string) (pattern string, escapeClause string) {
	escaped := strings.ReplaceAll(term, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `%`, `\%`)
	escaped = strings.ReplaceAll(escaped, `_`, `\_`)
	return "%" + escaped + "%", ` ESCAPE '\'`
}

// deduplicateUints returns a new slice with duplicate values removed, preserving order.
func deduplicateUints(ids []uint) []uint {
	seen := make(map[uint]bool, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
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

// ApplyUpdatedDateRange adds updated_at filters for the given column prefix if provided.
// The prefix should be empty string for simple table queries, or "tablename." for joined queries.
func ApplyUpdatedDateRange(db *gorm.DB, prefix, before, after string) *gorm.DB {
	if before != "" {
		db = db.Where(prefix+"updated_at <= ?", before)
	}
	if after != "" {
		db = db.Where(prefix+"updated_at >= ?", after)
	}
	return db
}

// ApplySortColumns validates and applies multiple ORDER BY clauses.
// tablePrefix should be "tablename." for joined queries, or empty string for simple queries.
// defaultSort is applied as the final tiebreaker sort (e.g., "created_at desc").
func ApplySortColumns(db *gorm.DB, sortBy []string, tablePrefix, defaultSort string) *gorm.DB {
	isSQLite := db.Config.Dialector.Name() == "sqlite"

	for _, sort := range sortBy {
		sort = strings.TrimSpace(sort)
		if !ValidateSortColumn(sort) {
			continue
		}

		if strings.HasPrefix(sort, "meta") {
			// Meta sort: convert for SQLite and add table prefix to disambiguate
			if isSQLite {
				sort = convertMetaSortForSQLite(sort, tablePrefix)
			} else if tablePrefix != "" {
				// Postgres: prefix meta column directly (e.g., groups.meta->>'key')
				sort = tablePrefix + sort
			}
			db = db.Order(sort)
		} else if tablePrefix != "" {
			// Regular column: add table prefix
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
