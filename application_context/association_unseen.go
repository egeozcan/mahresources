package application_context

import (
	"gorm.io/gorm"

	"mahresources/models"
)

// joinLink names one many-to-many join table from the side being edited.
type joinLink struct {
	table    string
	ownerCol string // the column holding the entity being edited
	farCol   string // the column holding the linked entity
	far      any    // the linked entity's model, for the visibility check
}

var (
	noteGroupsLink     = joinLink{"groups_related_notes", "note_id", "group_id", &models.Group{}}
	noteResourcesLink  = joinLink{"resource_notes", "note_id", "resource_id", &models.Resource{}}
	resourceGroupsLink = joinLink{"groups_related_resources", "resource_id", "group_id", &models.Group{}}
	resourceNotesLink  = joinLink{"resource_notes", "resource_id", "note_id", &models.Note{}}
)

// unseenLinks returns the IDs linked to ownerID through link that the caller
// cannot see. A group-limited principal's update rebuilds an association from
// what its scoped reads return, and the writer clears the join rows before
// adding that list back, so without this a rename deleted every link an
// administrator had made to an entity outside the subtree. The same rule as
// deleteRelationsSparingUnseen: what the caller cannot see, it cannot remove.
//
// The join table is read directly because no scope callback maps it, and the
// visibility check runs on tx, which carries the caller's scope. For an
// unscoped principal it returns nil without a query.
func (ctx *MahresourcesContext) unseenLinks(tx *gorm.DB, link joinLink, ownerID uint) ([]uint, error) {
	if !ctx.isScopedPrincipal() || ownerID == 0 {
		return nil, nil
	}
	var ids []uint
	if err := tx.Table(link.table).Where(link.ownerCol+" = ?", ownerID).Pluck(link.farCol, &ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	var unseen []uint
	if _, isGroup := link.far.(*models.Group); isGroup {
		visible := ctx.visibleGroupIDs(ids)
		for _, id := range ids {
			if !visible[id] {
				unseen = append(unseen, id)
			}
		}
		return unseen, nil
	}
	// One query per link, bounded by this entity's own degree. An IN list
	// would have the scope callback append the subtree allow-list as well.
	for _, id := range ids {
		var n int64
		if err := tx.Model(link.far).Where("id = ?", id).Limit(1).Count(&n).Error; err != nil {
			return nil, err
		}
		if n == 0 {
			unseen = append(unseen, id)
		}
	}
	return unseen, nil
}

// restoreLinks re-inserts join rows that unseenLinks reported, after the
// writer cleared the association.
func restoreLinks(tx *gorm.DB, link joinLink, ownerID uint, ids []uint) error {
	for _, id := range ids {
		row := map[string]any{link.ownerCol: ownerID, link.farCol: id}
		if err := tx.Table(link.table).Create(row).Error; err != nil {
			return err
		}
	}
	return nil
}
