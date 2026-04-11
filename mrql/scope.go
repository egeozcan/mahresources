package mrql

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ResolveScope resolves a Query's Scope clause to a concrete group ID.
// Returns 0 if no scope is set (global/no filter).
// For numeric scope: verifies group exists (error if not).
// For string scope: case-insensitive lookup (error if not found or ambiguous).
func ResolveScope(q *Query, db *gorm.DB) (uint, error) {
	if q.Scope == nil {
		return 0, nil
	}

	switch v := q.Scope.Value.(type) {
	case *NumberLiteral:
		if v.Unit != "" {
			return 0, &ScopeError{
				Message: fmt.Sprintf("SCOPE does not accept unit suffixes, got %q", v.Token.Value),
				Pos:     v.Token.Pos,
				Length:  v.Token.Length,
			}
		}
		if v.Value != float64(int64(v.Value)) {
			return 0, &ScopeError{
				Message: fmt.Sprintf("SCOPE requires an integer group ID, got %v", v.Value),
				Pos:     v.Token.Pos,
				Length:  v.Token.Length,
			}
		}
		id := uint(v.Value)
		if id == 0 {
			return 0, nil
		}
		var count int64
		if err := db.Table("groups").Where("id = ?", id).Count(&count).Error; err != nil {
			return 0, fmt.Errorf("scope resolution failed: %w", err)
		}
		if count == 0 {
			return 0, &ScopeError{
				Message: fmt.Sprintf("scope group not found: id %d", id),
				Pos:     v.Token.Pos,
				Length:  v.Token.Length,
			}
		}
		return id, nil

	case *StringLiteral:
		return resolveScopeByName(v, db)

	default:
		return 0, fmt.Errorf("unexpected scope value type: %T", q.Scope.Value)
	}
}

func resolveScopeByName(v *StringLiteral, db *gorm.DB) (uint, error) {
	type scopeMatch struct {
		ID         uint
		Name       string
		CategoryID *uint
		OwnerID    *uint
	}

	var matches []scopeMatch
	err := db.Table("groups").
		Select("id, name, category_id, owner_id").
		Where("LOWER(name) = LOWER(?)", v.Value).
		Find(&matches).Error
	if err != nil {
		return 0, fmt.Errorf("scope resolution failed: %w", err)
	}

	if len(matches) == 0 {
		return 0, &ScopeError{
			Message: fmt.Sprintf("scope group not found: %q", v.Value),
			Pos:     v.Token.Pos,
			Length:  v.Token.Length,
		}
	}

	if len(matches) == 1 {
		return matches[0].ID, nil
	}

	var lines []string
	for _, m := range matches {
		line := fmt.Sprintf("  - id=%d", m.ID)
		if m.CategoryID != nil {
			line += fmt.Sprintf(", categoryId=%d", *m.CategoryID)
		}
		if m.OwnerID != nil {
			line += fmt.Sprintf(", parentId=%d", *m.OwnerID)
		}
		lines = append(lines, line)
	}
	return 0, &ScopeError{
		Message: fmt.Sprintf("ambiguous scope %q: found %d groups:\n%s\nUse SCOPE <id> to specify which group.",
			v.Value, len(matches), strings.Join(lines, "\n")),
		Pos:    v.Token.Pos,
		Length: v.Token.Length,
	}
}

// UnresolvedScopeSentinel is a scope ID that guarantees empty results.
// Used when scope resolution fails for internal callers (ownerless entities).
const UnresolvedScopeSentinel = ^uint(0) >> 1

// ScopeError is returned when scope resolution fails.
type ScopeError struct {
	Message string
	Pos     int
	Length  int
}

func (e *ScopeError) Error() string { return e.Message }

// scopeCTE is the recursive CTE SQL that collects all group IDs in a subtree.
const scopeCTE = `WITH RECURSIVE scope_tree(id, depth) AS (
	SELECT id, 0 FROM groups WHERE id = ?
	UNION ALL
	SELECT g.id, st.depth + 1 FROM groups g
	INNER JOIN scope_tree st ON g.owner_id = st.id
	WHERE st.depth < 50
) SELECT id FROM scope_tree`

// ApplyScopeCTE injects an inline recursive CTE into a GORM query.
// For EntityGroup: WHERE id IN (CTE) — includes the scoped group itself.
// For other types: WHERE owner_id IN (CTE) — entities owned by subtree groups.
// When scopeGroupID doesn't exist, the CTE returns zero rows = no results.
func ApplyScopeCTE(db *gorm.DB, entityType EntityType, scopeGroupID uint) *gorm.DB {
	if entityType == EntityGroup {
		return db.Where(fmt.Sprintf("id IN (%s)", scopeCTE), scopeGroupID)
	}
	return db.Where(fmt.Sprintf("owner_id IN (%s)", scopeCTE), scopeGroupID)
}
