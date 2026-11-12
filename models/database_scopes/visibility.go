package database_scopes

import (
	"strconv"
	"strings"
)

// Visibility is the subtree a list filter may read through. The subtree scope
// callback confines only the rows a statement lists; a filter predicate that
// reaches another row (a parent, a related group, note or resource) through an
// aliased or raw subquery is never seen by it, and that row would answer for a
// principal that cannot see it. The builders take this as a required argument
// so every consumer has to say whose subtree it is, and so it can never come
// from the request-decoded query DTO.
//
// The zero value is unconfined. Confined with RootID 0 is deny-all. The
// predicates it produces bind no parameters: the root is a uint formatted into
// a recursive CTE, as MRQL's TranslateOptions.VisibleRootID does, so they cost
// nothing against the bind ceilings the scope allow-list already presses on.
type Visibility struct {
	Confined bool
	RootID   uint
}

// visibleGroupIDs selects the ids of the groups the caller can see.
func (v Visibility) visibleGroupIDs() string {
	return strings.Replace(groupSubtreeCTE, "?", strconv.FormatUint(uint64(v.RootID), 10), 1)
}

// groupVisible is a predicate that holds when the group id in col is visible,
// or "" when the caller is not confined. A NULL id is not visible, matching the
// scope callback's IN.
func (v Visibility) groupVisible(col string) string {
	if !v.Confined {
		return ""
	}
	if v.RootID == 0 {
		return "1 = 0"
	}
	return col + " IN (" + v.visibleGroupIDs() + ")"
}

// ownedRowVisible is a predicate that holds when col names a resource or note
// (table) owned by a visible group, or "" when the caller is not confined.
// This is the rule the scope callback applies to those tables.
func (v Visibility) ownedRowVisible(col, table string) string {
	owner := v.groupVisible("v.owner_id")
	if owner == "" || owner == "1 = 0" {
		return owner
	}
	return col + " IN (SELECT v.id FROM " + table + " v WHERE " + owner + ")"
}

// parentGuard is a predicate a group row must meet for its parent to be read,
// or "" when the caller is not confined. The scope root is the one visible
// group whose parent lies outside the subtree, so excluding it is enough;
// every other visible group's parent is itself visible.
func (v Visibility) parentGuard() string {
	if !v.Confined {
		return ""
	}
	if v.RootID == 0 {
		return "1 = 0"
	}
	return "groups.id <> " + strconv.FormatUint(uint64(v.RootID), 10)
}

// and is "(cond AND pred)", or cond unchanged when pred is "", so an unconfined
// caller's SQL stays exactly what it was. The parentheses keep it whole inside
// an OR.
func and(cond, pred string) string {
	if pred == "" {
		return cond
	}
	return "(" + cond + " AND " + pred + ")"
}

// where is " WHERE pred", or "" when pred is "".
func where(pred string) string {
	if pred == "" {
		return ""
	}
	return " WHERE " + pred
}
