package application_context

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/shortcodes"
)

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
// group_relations is not mapped by the ORM scope callback. For a confined
// request the far endpoint must match the same scope snapshot as the carrier,
// before LIMIT: hidden rows must not displace visible rows.
func relationTemplateQuery(db *gorm.DB, req shortcodes.RelationsRequest, limit int, scope *scopeFilter) *gorm.DB {
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
	if scope != nil {
		endpoint := "group_relations." + far
		ids := "SELECT value FROM json_each(?)"
		if db.Dialector.Name() == "postgres" {
			ids = "SELECT CAST(value AS bigint) FROM jsonb_array_elements_text(CAST(? AS jsonb))"
		} else {
			// Keep SQLite on the carrier's index range. Without the unary +,
			// incoming reads seek unique_rel once per ID in the entire set.
			endpoint = "+" + endpoint
		}
		q = q.Where(endpoint+" IN ("+ids+")", scope.relationScopeJSON())
	}
	return q.Order("rt.name, group_relations.id").Limit(limit)
}

// relationScopeJSON binds the existing snapshot as one set parameter instead
// of one parameter per ID, which can exceed either driver's bind limit.
func (sf *scopeFilter) relationScopeJSON() string {
	sf.allowedJSONOnce.Do(func() {
		if len(sf.allowed) == 0 {
			sf.allowedJSON = "[]"
			return
		}
		encoded, _ := json.Marshal(sf.allowed) // []uint always encodes
		sf.allowedJSON = string(encoded)
	})
	return sf.allowedJSON
}

// IsGroupVisibleForTemplateRelations checks whether a lookup will reach SQL,
// so the render layer need not spend query budget on a refused lookup. An
// invisible carrier has no edges; a confined binding without an ORM scope
// (e.g. WithMRQLPrincipal) cannot answer and is unavailable.
func (ctx *MahresourcesContext) IsGroupVisibleForTemplateRelations(groupID uint) (bool, error) {
	if scope := ctx.groupScopeFilter(); scope != nil {
		return scope.containsGroup(groupID), nil
	}
	if ctx.isScopedPrincipal() {
		return false, shortcodes.ErrRelationsUnavailable
	}
	return true, nil
}

// ListGroupRelationsForTemplate lists one group's typed relations for the
// [relations] shortcode, as this context's principal sees them: an edge is
// listed only when both of its groups are visible, the rule relationInScope
// applies to edge writes. It runs one statement, and none when the carrier is
// not visible.
//
// reqCtx bounds the statement (the caller's timeout); the handle is rebased
// onto it with WithParentContext, which keeps the scope filter and the actor.
func (ctx *MahresourcesContext) ListGroupRelationsForTemplate(reqCtx context.Context, req shortcodes.RelationsRequest) ([]shortcodes.GroupRelationEntry, error) {
	if req.Direction != shortcodes.RelationsDirectionOut && req.Direction != shortcodes.RelationsDirectionIn {
		return nil, fmt.Errorf("relations: unsupported direction %q", req.Direction)
	}
	limit := min(max(req.Limit, 1), shortcodes.MaxRelationsLimit)
	if ok, err := ctx.IsGroupVisibleForTemplateRelations(req.GroupID); !ok {
		return nil, err
	}

	var rows []relationTemplateRow
	if err := relationTemplateQuery(ctx.WithParentContext(reqCtx).db, req, limit, ctx.groupScopeFilter()).Find(&rows).Error; err != nil {
		return nil, err
	}
	entries := make([]shortcodes.GroupRelationEntry, 0, len(rows))
	for _, r := range rows {
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
	if ctx.groupScopeFilter() == nil && ctx.isScopedPrincipal() {
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
