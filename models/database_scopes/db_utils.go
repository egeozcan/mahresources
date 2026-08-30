package database_scopes

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

// ErrInvalidDateFilter is a sentinel error for invalid date filter values.
// Handlers should check for this error using errors.Is and return HTTP 400.
var ErrInvalidDateFilter = fmt.Errorf("invalid date filter value")

// ValidateDateString checks whether a string is a valid date for use in date range filters.
// Accepted formats: "2006-01-02" (date only) and RFC 3339 (e.g., "2006-01-02T15:04:05Z07:00").
func ValidateDateString(s string) bool {
	if s == "" {
		return false
	}
	_, _, ok := parseDateBound(s)
	return ok
}

// SortColumnMatcher validates sort column strings to prevent SQL injection.
// Matches: column_name, column_name desc, column_name asc, meta->>'key', meta->>'key' desc
var SortColumnMatcher = regexp.MustCompile(`^(meta->>?'[a-z_]+'|[a-z_]+)(\s(desc|asc))?$`)

// metaSortMatcher extracts the key from meta sort expressions like meta->>'key_name'
var metaSortMatcher = regexp.MustCompile(`^meta->>?'([a-z_]+)'(\s+(desc|asc))?$`)

// groupSubtreeCTE selects all group IDs in the subtree rooted at the single
// placeholder ID (including the root itself). UNION (not UNION ALL)
// deduplicates, so ownership cycles terminate. Works on SQLite and Postgres.
const groupSubtreeCTE = `WITH RECURSIVE group_subtree(id) AS (
	SELECT id FROM groups WHERE id = ?
	UNION
	SELECT g.id FROM groups g INNER JOIN group_subtree gs ON g.owner_id = gs.id
) SELECT id FROM group_subtree`

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

// convertMetaSortForPostgres changes the public/meta-sort syntax from text
// extraction (->>) to type-preserving JSONB extraction (->). Keeping the JSON
// scalar type is important for schema-defined numeric fields: ->> would sort
// values such as 9 and 10 as strings.
func convertMetaSortForPostgres(sort, tablePrefix string) string {
	matches := metaSortMatcher.FindStringSubmatch(sort)
	if matches == nil {
		return sort
	}
	key := matches[1]
	direction := strings.TrimSpace(matches[2])
	result := tablePrefix + "meta->'" + key + "'"
	if direction != "" {
		result += " " + direction
	}
	return result
}

// likeEscapeClause is the suffix that makes `\` the escape character of a LIKE
// expression, so a caller can pass a pattern containing an escaped wildcard.
const likeEscapeClause = ` ESCAPE '\'`

// LikeTermIsUnmatchable reports whether a search term can match no stored text
// at all, so a caller can answer it without asking the database.
//
// Both cases are bytes Postgres refuses inside a text parameter outright
// (SQLSTATE 22021, "invalid byte sequence for encoding UTF8"): a NUL, and any
// sequence that is not valid UTF-8. A term carrying either answered HTTP 500
// rather than "no matches" — and stored text is valid UTF-8 containing no NUL,
// so "no matches" is both the safe answer and the true one.
func LikeTermIsUnmatchable(term string) bool {
	return strings.ContainsRune(term, 0) || !utf8.ValidString(term)
}

// LikePattern builds a LIKE pattern with proper escaping of wildcard characters.
// Returns the escaped pattern and the ESCAPE clause suffix to append to the LIKE expression.
func LikePattern(term string) (pattern string, escapeClause string) {
	escaped := strings.ReplaceAll(term, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `%`, `\%`)
	escaped = strings.ReplaceAll(escaped, `_`, `\_`)
	return "%" + escaped + "%", likeEscapeClause
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

// applyDateRangeOn adds a date range filter on one column. label names the
// filter in the error message, so a caller sees the parameter it actually sent
// rather than the column the scope happens to use.
// Invalid date strings cause an error on the *gorm.DB via AddError, which propagates through the chain.
func applyDateRangeOn(db *gorm.DB, column, label, before, after string) *gorm.DB {
	if before != "" {
		if !ValidateDateString(before) {
			_ = db.AddError(fmt.Errorf("%w: %sBefore=%q is not a valid date (expected YYYY-MM-DD or RFC 3339)", ErrInvalidDateFilter, label, before))
			return db
		}
		db = applyDateBound(db, column, before, upperBound)
	}
	if after != "" {
		if !ValidateDateString(after) {
			_ = db.AddError(fmt.Errorf("%w: %sAfter=%q is not a valid date (expected YYYY-MM-DD or RFC 3339)", ErrInvalidDateFilter, label, after))
			return db
		}
		db = applyDateBound(db, column, after, lowerBound)
	}
	return db
}

// Which end of the range a bound is. It decides what a bound outside every
// storable year means, and the two answers are opposites.
type dateBoundEnd int

const (
	upperBound dateBoundEnd = iota
	lowerBound
)

// applyDateBound adds one end of a date range.
//
// A bound outside the years the databases can hold is answered here rather than
// sent to the engine, because Go's RFC 3339 is wider than both of them: year
// zero exists to Go and not to Postgres (SQLSTATE 22008), and
// `9999-12-31T23:00:00-14:00` is year 10000 once it is in UTC, whose five-digit
// string destroys the fixed-width lexical ordering SQLite compares on.
//
// The answer depends on which end it is, which is why it is not a clamp: a lower
// bound below every stored row is no restriction at all, and the *same string*
// as an upper bound excludes everything. Clamping to the nearest representable
// instant gets both wrong at the boundary, since a row sitting exactly on the
// clamped instant then satisfies a bound that names something beyond it.
//
// "No restriction" is still `IS NOT NULL` rather than no predicate: completed_at
// is nullable, and a download that never finished is outside every window that
// names a finish time — a property the ordinary comparison gives for free and
// that dropping the predicate would quietly reverse.
func applyDateBound(db *gorm.DB, column, bound string, end dateBoundEnd) *gorm.DB {
	switch outOfStorableRange(bound) {
	case below:
		if end == lowerBound {
			return db.Where(column + " IS NOT NULL")
		}
		return db.Where("1 = 0")
	case above:
		if end == upperBound {
			return db.Where(column + " IS NOT NULL")
		}
		return db.Where("1 = 0")
	}

	lhs, value := dateComparison(db, column, bound)
	if end == upperBound {
		return db.Where(lhs+" <= ?", value)
	}
	return db.Where(lhs+" >= ?", value)
}

// Where a bound sits relative to the four-digit years both engines can express.
type storableRangePosition int

const (
	within storableRangePosition = iota
	below
	above
)

// outOfStorableRange reports whether a bound names a year no database here can
// hold. The comparison is made in UTC, because that is the frame both the
// Postgres parameter and the SQLite normalisation end up in: an offset can move
// a four-digit local year out of range and a five-digit one in.
func outOfStorableRange(bound string) storableRangePosition {
	at, _, ok := parseDateBound(bound)
	if !ok {
		return within
	}
	switch {
	case at.UTC().Year() < 1:
		return below
	case at.UTC().Year() > 9999:
		return above
	}
	return within
}

// parseDateBound parses either shape a date filter admits, and reports which one
// it was. Which layout parsed is the answer to "is this a bare day or an
// instant?"; sniffing the string for a "T" was not, because the two questions
// come apart on any input Go's layouts happen to read differently.
//
// The grammar is deliberately Go's RFC 3339 and no wider, although §5.6 of the
// RFC permits a lower-case separator and zone. ValidateDateString is shared with
// scopes that validate a bound and then compare the caller's *raw string*
// lexically (note_scope's four start/end bounds), so admitting a spelling here
// admits it into a comparison that reads it wrong and answers no error at all.
// A stricter validator refuses it, which is the safe half of that trade.
func parseDateBound(bound string) (at time.Time, isInstant bool, ok bool) {
	if day, err := time.Parse(dateOnlyFormat, bound); err == nil {
		return day, false, true
	}
	if instant, err := time.Parse(time.RFC3339, bound); err == nil {
		return instant, true, true
	}
	return time.Time{}, false, false
}

// sqliteInstantFormat is how both sides of an RFC 3339 comparison are written on
// SQLite: UTC, fixed width, whole seconds.
//
// Whole seconds because strftime's millisecond field cannot be matched from Go.
// It rounds — .9009 reads back as .901 — except at a second boundary, where it
// clamps instead: .9999 reads back as .999, not the .000 of the next second that
// rounding would give. Go has no formatting verb with that behaviour, and a
// bound formatted with either rule disagrees with the column under the other.
// %S truncates unconditionally, on both sides, so the two agree everywhere.
//
// The cost is that a bound is accurate to the second and the second it names is
// wholly inside the range at either end — the same shape as a bare date naming a
// whole day, and finer than anything this app's date inputs can express.
const sqliteInstantFormat = "2006-01-02 15:04:05"

// dateOnlyFormat is the bare-day shape the date inputs emit.
const dateOnlyFormat = "2006-01-02"

// dateComparison decides how one side of a date bound is compared, returning the
// expression to compare and the value to bind.
//
// SQLite has no date type: go-sqlite3 stores a time as `2006-01-02 15:04:05...`
// with a space, and the comparison is then lexical. A bare YYYY-MM-DD bound
// sorts correctly against that, so it is left alone — bare column, index intact,
// and it is the only shape the date inputs emit.
//
// An RFC 3339 bound does not. It carries a `T` where the stored text has a
// space, and a space sorts *below* every digit, so
// `2026-03-03 14:00:00+00:00 <= 2026-03-03T13:00:00Z` is true and an afternoon
// download is reported as having finished before lunch. Both bounds were wrong,
// in opposite directions, and neither reported an error.
//
// Such a bound is therefore normalised on both sides: strftime rewrites the
// column into UTC, and the parameter is rendered the same way *in Go*, at the
// granularity sqliteInstantFormat explains. Parsing it here rather than handing
// the caller's text to the database matters on both engines — Go's RFC 3339 accepts a comma fraction
// (`13:00:00,5Z`) that SQLite's date functions answer NULL for, silently
// matching no rows, and that Postgres rejects outright with a 500. Wrapping the
// column costs its index, which is why only this path pays it.
//
// Postgres has a real timestamp type, so there the bound is only re-emitted in
// canonical UTC form and the column is left bare and indexed. UTC, not the
// offset the caller wrote: Go's RFC 3339 admits offsets out to ±23:59 and
// Postgres refuses anything past ±15:59 with SQLSTATE 22009, so a valid bound
// answered 500.
//
// One asymmetry remains by choice: a bare date is compared against the stored
// text, an RFC 3339 bound against a normalised instant. The two diverge only
// where the database stores a non-UTC offset, and closing it would mean putting
// the common, indexed path through strftime as well.
func dateComparison(db *gorm.DB, column, bound string) (lhs, value string) {
	at, isInstant, ok := parseDateBound(bound)
	if !ok {
		// ValidateDateString admitted it, so this is unreachable; comparing the
		// raw text is what the caller would have got anyway.
		return column, bound
	}
	if !isInstant {
		// A bare date is compared as written — bare column, index intact.
		return column, bound
	}
	at = at.UTC()
	if db.Config.Dialector.Name() == "postgres" {
		return column, at.Format(time.RFC3339Nano)
	}
	return "strftime('%Y-%m-%d %H:%M:%S', " + column + ")", at.Format(sqliteInstantFormat)
}

// ApplyDateRange adds created_at filters for the given column prefix if provided.
// The prefix should be empty string for simple table queries, or "tablename." for joined queries.
func ApplyDateRange(db *gorm.DB, prefix, before, after string) *gorm.DB {
	return applyDateRangeOn(db, prefix+"created_at", "created", before, after)
}

// ApplyUpdatedDateRange adds updated_at filters for the given column prefix if provided.
// The prefix should be empty string for simple table queries, or "tablename." for joined queries.
func ApplyUpdatedDateRange(db *gorm.DB, prefix, before, after string) *gorm.DB {
	return applyDateRangeOn(db, prefix+"updated_at", "updated", before, after)
}

// ApplyCompletedDateRange adds completed_at filters for the given column prefix
// if provided. A NULL completed_at fails both comparisons, which is what an
// unfinished row deserves: it is outside every window that names a finish time.
func ApplyCompletedDateRange(db *gorm.DB, prefix, before, after string) *gorm.DB {
	return applyDateRangeOn(db, prefix+"completed_at", "completed", before, after)
}

// ApplyMetaQuery applies MetaQuery filters to a GORM query. Same-key EQ entries
// are grouped into an OR clause (for multi-select enum semantics), while
// different-key entries and non-EQ operators are AND'd as before.
func ApplyMetaQuery(db *gorm.DB, metaQuery []query_models.ColumnMeta, column string) *gorm.DB {
	if len(metaQuery) == 0 {
		return db
	}

	// Group same-key EQ entries for OR semantics
	type eqGroup struct {
		values []any
	}
	eqGroups := make(map[string]*eqGroup)
	var nonGrouped []query_models.ColumnMeta

	for _, v := range metaQuery {
		if v.Key == "" {
			continue
		}
		if v.Operation == "EQ" || v.Operation == "" {
			g, ok := eqGroups[v.Key]
			if !ok {
				g = &eqGroup{}
				eqGroups[v.Key] = g
			}
			g.values = append(g.values, v.Value)
		} else {
			nonGrouped = append(nonGrouped, v)
		}
	}

	// Apply grouped EQ entries: if a key has multiple values, OR them
	for key, g := range eqGroups {
		if len(g.values) == 1 {
			db = db.Where(types.JSONQuery(column).Operation(types.OperatorEquals, g.values[0], key))
		} else {
			// Build OR clause for multiple values
			orDB := db.Session(&gorm.Session{NewDB: true})
			for i, val := range g.values {
				if i == 0 {
					orDB = orDB.Where(types.JSONQuery(column).Operation(types.OperatorEquals, val, key))
				} else {
					orDB = orDB.Or(types.JSONQuery(column).Operation(types.OperatorEquals, val, key))
				}
			}
			db = db.Where(orDB)
		}
	}

	// Apply non-grouped entries normally (AND)
	for _, v := range nonGrouped {
		db = db.Where(types.JSONQuery(column).Operation(getOperationType(v.Operation), v.Value, v.Key))
	}

	return db
}

// ApplyPrefixedMetaQuery applies parent./child. prefixed MetaQuery entries
// using subqueries on the "groups p" alias table. Same-key EQ entries are
// grouped into OR within a single subquery, matching ApplyMetaQuery behavior.
//
// joinCondition links alias "p" to the main "groups" table
// (e.g., "groups.owner_id = p.id"). countOp compares the count
// (e.g., "= 1" or ">= 1").
func ApplyPrefixedMetaQuery(db, originalDB *gorm.DB, entries []query_models.ColumnMeta, joinCondition, countOp string) *gorm.DB {
	if len(entries) == 0 {
		return db
	}

	// Group same-key EQ entries for OR semantics
	type eqGroup struct{ values []any }
	eqGroups := make(map[string]*eqGroup)
	var nonGrouped []query_models.ColumnMeta

	for _, e := range entries {
		if e.Operation == "EQ" || e.Operation == "" {
			g, ok := eqGroups[e.Key]
			if !ok {
				g = &eqGroup{}
				eqGroups[e.Key] = g
			}
			g.values = append(g.values, e.Value)
		} else {
			nonGrouped = append(nonGrouped, e)
		}
	}

	// Apply grouped EQ entries
	for key, g := range eqGroups {
		if len(g.values) == 1 {
			subSelect := originalDB.
				Table("groups p").
				Select("count(*)").
				Where(types.JSONQuery("p.meta").Operation(types.OperatorEquals, g.values[0], key)).
				Where(joinCondition)
			db = db.Where("(?) "+countOp, subSelect)
		} else {
			subSelect := originalDB.
				Table("groups p").
				Select("count(*)")
			orDB := originalDB.Session(&gorm.Session{NewDB: true})
			for i, val := range g.values {
				if i == 0 {
					orDB = orDB.Where(types.JSONQuery("p.meta").Operation(types.OperatorEquals, val, key))
				} else {
					orDB = orDB.Or(types.JSONQuery("p.meta").Operation(types.OperatorEquals, val, key))
				}
			}
			subSelect = subSelect.Where(orDB).Where(joinCondition)
			db = db.Where("(?) "+countOp, subSelect)
		}
	}

	// Apply non-grouped entries
	for _, e := range nonGrouped {
		subSelect := originalDB.
			Table("groups p").
			Select("count(*)").
			Where(types.JSONQuery("p.meta").Operation(getOperationType(e.Operation), e.Value, e.Key)).
			Where(joinCondition)
		db = db.Where("(?) "+countOp, subSelect)
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
			// Meta sort: preserve the JSON scalar type so schema-defined numbers
			// are ordered numerically rather than lexicographically.
			if isSQLite {
				sort = convertMetaSortForSQLite(sort, tablePrefix)
			} else {
				sort = convertMetaSortForPostgres(sort, tablePrefix)
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
