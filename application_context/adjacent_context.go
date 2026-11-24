package application_context

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/shortcodes"
)

// adjacentSpec names, per entity type, the model and table [adjacent] reads and
// the category column that partitions siblings. It is a fixed map so no
// attribute text ever reaches the SQL.
type adjacentSpec struct {
	model          func() any
	table          string
	categoryColumn string
}

var adjacentSpecs = map[string]adjacentSpec{
	"resource": {func() any { return &models.Resource{} }, "resources", "resource_category_id"},
	"note":     {func() any { return &models.Note{} }, "notes", "note_type_id"},
	"group":    {func() any { return &models.Group{} }, "groups", "category_id"},
}

// adjacentRow is the current row's partition: its owner, its category and, for
// a resource, its Series.
type adjacentRow struct {
	ID       uint
	OwnerId  *uint
	Category *uint
	SeriesID *uint
}

type adjacentLinkRow struct {
	ID   uint
	Name string
}

// AdjacentEntities returns the previous and next siblings of one entity in
// created_at, id order, as this context's principal sees them. Siblings are
// the same Series (q.Series, resources only), or the rows with the same owner
// (or none) and the same category column (or none).
//
// Every read is a Find on the principal-bound handle, so the scope callback
// confines all three to the principal's subtree; Scan and Row would bypass it.
// The current row is read first, and when the principal cannot see it nothing
// else runs: the seeks read its creation time through an unscoped subquery,
// which is safe only because that row was just proved visible.
//
// reqCtx bounds every query (the caller's timeout). The handle is rebased onto
// it with WithParentContext, which carries the scope filter and actor over.
func (ctx *MahresourcesContext) AdjacentEntities(reqCtx context.Context, q shortcodes.AdjacentQuery) (shortcodes.AdjacentResult, error) {
	spec, ok := adjacentSpecs[q.EntityType]
	if !ok {
		return shortcodes.AdjacentResult{}, fmt.Errorf("adjacent: unsupported entity type %q", q.EntityType)
	}
	if q.Series && q.EntityType != "resource" {
		return shortcodes.AdjacentResult{}, fmt.Errorf("adjacent: series mode is for resources only")
	}

	// Fail closed on a handle that should carry a subtree filter and does not:
	// a WithMRQLPrincipal binding scopes in SQL, not through the ORM, so its
	// Finds would read the whole library. This runs before the rebase, which
	// keeps an absent filter absent.
	if _, forced, deny := ctx.principalForcedScope(); (forced || deny) && scopeFromContext(ctx.db.Statement.Context) == nil {
		return shortcodes.AdjacentResult{}, nil
	}
	db := ctx.WithParentContext(reqCtx).db

	columns := "id, owner_id, " + spec.categoryColumn + " AS category"
	if q.Series {
		columns += ", series_id"
	}
	var cur []adjacentRow
	if err := db.Model(spec.model()).Select(columns).Where(spec.table+".id = ?", q.EntityID).Limit(1).Find(&cur).Error; err != nil {
		return shortcodes.AdjacentResult{}, err
	}
	if len(cur) == 0 || (q.Series && cur[0].SeriesID == nil) {
		return shortcodes.AdjacentResult{}, nil
	}

	var result shortcodes.AdjacentResult
	for _, next := range []bool{false, true} {
		var found []adjacentLinkRow
		if err := adjacentSeek(db, spec, cur[0], q.Series, next).Find(&found).Error; err != nil {
			return shortcodes.AdjacentResult{}, err
		}
		if len(found) == 0 {
			continue
		}
		link := &shortcodes.AdjacentLink{ID: found[0].ID, Name: found[0].Name}
		if next {
			result.Next = link
		} else {
			result.Prev = link
		}
	}
	return result, nil
}

// adjacentSeek builds the one-row keyset seek for the sibling after (next) or
// before cur. The key is written as created_at >= K AND (created_at > K OR
// id > cur.id) rather than a row-value comparison, so both planners read it as
// a range on the (owner_id, category, created_at, id) index after the
// equality prefix and stop at the first row; the OR only filters ties on K.
// K is read in SQL from the current row, so a timestamp never makes a bind
// round trip and ties compare exactly. The outer columns are qualified so the
// subquery's alias cannot capture them.
func adjacentSeek(db *gorm.DB, spec adjacentSpec, cur adjacentRow, series, next bool) *gorm.DB {
	t := spec.table
	q := db.Model(spec.model()).Select(t + ".id, " + t + ".name")
	if series {
		q = q.Where(t+".series_id = ?", cur.SeriesID)
	} else {
		if cur.OwnerId == nil {
			q = q.Where(t + ".owner_id IS NULL")
		} else {
			q = q.Where(t+".owner_id = ?", *cur.OwnerId)
		}
		if cur.Category == nil {
			q = q.Where(t + "." + spec.categoryColumn + " IS NULL")
		} else {
			q = q.Where(t+"."+spec.categoryColumn+" = ?", *cur.Category)
		}
	}
	key := "(SELECT cur.created_at FROM " + t + " cur WHERE cur.id = ?)"
	cmp, strict, dir := ">=", ">", "ASC"
	if !next {
		cmp, strict, dir = "<=", "<", "DESC"
	}
	// In default mode the partition columns lead the ORDER BY. They are
	// constant within the partition, so the order is the same, but PostgreSQL
	// only treats an = as a constant: with owner_id IS NULL (or a NULL
	// category) in the prefix it no longer saw that the index is already in
	// created_at, id order, and walked idx_*_created_at with a filter and a
	// sort instead. Spelled out, the ORDER BY is exactly the index's order.
	order := t + ".created_at " + dir + ", " + t + ".id " + dir
	if !series {
		order = t + ".owner_id " + dir + ", " + t + "." + spec.categoryColumn + " " + dir + ", " + order
	}
	return q.Where(
		fmt.Sprintf("%[1]s.created_at %[2]s %[4]s AND (%[1]s.created_at %[3]s %[4]s OR %[1]s.id %[3]s ?)", t, cmp, strict, key),
		cur.ID, cur.ID, cur.ID,
	).Order(order).Limit(1)
}
