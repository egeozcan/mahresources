package application_context

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/shortcodes"
)

// relationTemplateScopedScan is how many edges [relations] reads in one
// direction for a subtree-confined viewer, before the hidden ones are dropped
// and the limit applies. Reading more than the limit lets a group whose first
// edges are mostly hidden still fill it; reading a bounded number keeps a group
// with a vast edge set from being read whole.
const relationTemplateScopedScan = 500

// relationTemplateRow is one edge joined to its type, its far group and that
// group's category.
type relationTemplateRow struct {
	ID           uint
	Name         string
	Description  string
	TypeName     string
	GroupID      uint
	GroupName    string
	CategoryName *string
}

// relationTemplateQuery builds the one statement [relations] runs: the edges
// at the carrier's end of the direction, with the type, the far group and its
// category joined in, ordered by type name and then edge id.
//
// It reads group_relations, a table the scope callback does not map, so the
// callback adds nothing to it; visibility is decided by the caller. Joining
// groups here rather than loading them through the scoped handle is what keeps
// the principal's allow-list out of the statement: that callback binds the
// whole list, which errors past SQLite's variable limit and Postgres's 65535
// parameters.
func relationTemplateQuery(db *gorm.DB, req shortcodes.RelationsRequest, fetch int) *gorm.DB {
	near, far := "from_group_id", "to_group_id"
	if req.Direction == shortcodes.RelationsDirectionIn {
		near, far = far, near
	}
	q := db.Model(&models.GroupRelation{}).
		Select("group_relations.id, group_relations.name, group_relations.description, "+
			"rt.name AS type_name, far.id AS group_id, far.name AS group_name, c.name AS category_name").
		Joins("JOIN group_relation_types rt ON rt.id = group_relations.relation_type_id").
		Joins("JOIN groups far ON far.id = group_relations."+far).
		Joins("LEFT JOIN categories c ON c.id = far.category_id").
		Where("group_relations."+near+" = ?", req.GroupID)
	if req.Type != "" {
		q = q.Where("rt.name = ?", req.Type)
	}
	return q.Order("rt.name, group_relations.id").Limit(fetch)
}

// confinedWithoutFilter reports a subtree-limited principal whose handle
// carries no scope filter: a WithMRQLPrincipal binding, which scopes MRQL in
// SQL and never materialises the allow-list. Such a handle cannot answer which
// groups the principal may see.
func (ctx *MahresourcesContext) confinedWithoutFilter() bool {
	if !ctx.isScopedPrincipal() {
		return false
	}
	return ctx.db == nil || ctx.db.Statement == nil || scopeFromContext(ctx.db.Statement.Context) == nil
}

// ListGroupRelationsForTemplate lists one group's typed relations for the
// [relations] shortcode, as this context's principal sees them: an edge is
// listed only when both of its groups are visible, the rule relationInScope
// applies to edge writes. It runs one statement, and none when the carrier is
// not visible.
//
// Visibility is read from the allow-list already on the handle
// (visibleGroupIDs), never bound into SQL, so a subtree of any size works. A
// confined viewer's statement reads up to relationTemplateScopedScan edges so
// that hidden ones do not starve the limit; an unconfined one reads exactly
// the limit.
//
// reqCtx bounds the statement (the caller's timeout); the handle is rebased
// onto it with WithParentContext, which keeps the scope filter and the actor.
func (ctx *MahresourcesContext) ListGroupRelationsForTemplate(reqCtx context.Context, req shortcodes.RelationsRequest) ([]shortcodes.GroupRelationEntry, error) {
	if req.Direction != shortcodes.RelationsDirectionOut && req.Direction != shortcodes.RelationsDirectionIn {
		return nil, fmt.Errorf("relations: unsupported direction %q", req.Direction)
	}
	limit := min(max(req.Limit, 1), shortcodes.MaxRelationsLimit)
	if ctx.confinedWithoutFilter() {
		return nil, shortcodes.ErrRelationsUnavailable
	}
	if !ctx.visibleGroupIDs([]uint{req.GroupID})[req.GroupID] {
		return nil, nil
	}
	fetch := limit
	if ctx.isScopedPrincipal() {
		fetch = relationTemplateScopedScan
	}

	var rows []relationTemplateRow
	if err := relationTemplateQuery(ctx.WithParentContext(reqCtx).db, req, fetch).Find(&rows).Error; err != nil {
		return nil, err
	}
	farIDs := make([]uint, len(rows))
	for i, r := range rows {
		farIDs[i] = r.GroupID
	}
	visible := ctx.visibleGroupIDs(farIDs)

	entries := make([]shortcodes.GroupRelationEntry, 0, min(len(rows), limit))
	for _, r := range rows {
		if len(entries) == limit {
			break
		}
		if !visible[r.GroupID] {
			continue
		}
		e := shortcodes.GroupRelationEntry{
			Direction: req.Direction, Type: r.TypeName, Name: r.Name, Description: r.Description,
			GroupID: r.GroupID, GroupName: r.GroupName,
		}
		if r.CategoryName != nil {
			e.GroupCategory = *r.CategoryName
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// LoadRelatedGroupDetails reads the ID, Description and Meta of the groups at
// the far end of the relations [relations] listed, for a block body that reads
// GroupDescription or GroupMeta. Only groups the principal may see are read; a
// group that no longer exists is simply absent. At most
// shortcodes.MaxRelationsLimit IDs are accepted, the most one [relations] can
// list.
//
// The read goes through the unscoped handle, after visibleGroupIDs has
// admitted every ID: through the scoped one the callback would bind the
// principal's whole allow-list beside these IDs.
func (ctx *MahresourcesContext) LoadRelatedGroupDetails(reqCtx context.Context, ids []uint) ([]models.Group, error) {
	if len(ids) > shortcodes.MaxRelationsLimit {
		return nil, fmt.Errorf("relations: %d group IDs, at most %d", len(ids), shortcodes.MaxRelationsLimit)
	}
	if ctx.confinedWithoutFilter() {
		return nil, shortcodes.ErrRelationsUnavailable
	}
	visible := ctx.visibleGroupIDs(ids)
	admitted := make([]uint, 0, len(ids))
	for _, id := range ids {
		if visible[id] {
			admitted = append(admitted, id)
		}
	}
	if len(admitted) == 0 {
		return nil, nil
	}
	var groups []models.Group
	err := ctx.WithParentContext(reqCtx).unscopedDB().
		Model(&models.Group{}).Select("id, description, meta").Where("id IN ?", admitted).Find(&groups).Error
	if err != nil {
		return nil, err
	}
	return groups, nil
}
