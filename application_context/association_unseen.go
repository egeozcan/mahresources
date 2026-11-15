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
	farTable string // the linked entity's table
	far      any    // the linked entity's model, for the scoped visibility check
}

var (
	noteGroupsLink     = joinLink{"groups_related_notes", "note_id", "group_id", "groups", &models.Group{}}
	noteResourcesLink  = joinLink{"resource_notes", "note_id", "resource_id", "resources", &models.Resource{}}
	resourceGroupsLink = joinLink{"groups_related_resources", "resource_id", "group_id", "groups", &models.Group{}}
	resourceNotesLink  = joinLink{"resource_notes", "resource_id", "note_id", "notes", &models.Note{}}
)

// clearLinks removes ownerID's links through link before the writer adds
// the caller's list back. assoc is the GORM association the link belongs to.
//
// For a group-limited principal it removes only the links the caller could
// have seen, and keeps the rest. The update handlers rebuild an omitted
// association list from the caller's scoped reads, so clearing every join
// row deleted each link an administrator had made to an entity outside the
// subtree, without naming it. This is the rule deleteRelationsSparingUnseen
// applies to group relations: what the caller cannot see, it cannot remove.
// A link whose far row no longer exists protects nothing and is removed too.
//
// Only rows are deleted, never re-inserted, so a link that someone else
// deletes while this update runs stays deleted. Visibility is decided on tx,
// which carries the scope the update itself runs under.
func (ctx *MahresourcesContext) clearLinks(tx *gorm.DB, owner any, assoc string, link joinLink, ownerID uint) error {
	if !ctx.isScopedPrincipal() {
		return tx.Model(owner).Association(assoc).Clear()
	}

	var ids []uint
	if err := tx.Table(link.table).Where(link.ownerCol+" = ?", ownerID).Pluck(link.farCol, &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	// Raw bypasses the scope callbacks, so this sees every row that exists.
	var existing []uint
	if err := tx.Raw("SELECT id FROM "+link.farTable+" WHERE id IN ?", ids).Scan(&existing).Error; err != nil {
		return err
	}
	exists := make(map[uint]bool, len(existing))
	for _, id := range existing {
		exists[id] = true
	}

	removable := make([]uint, 0, len(ids))
	for _, id := range ids {
		if !exists[id] {
			removable = append(removable, id)
			continue
		}
		// One scoped query per link, bounded by this entity's own degree. An
		// IN list would have the scope callback append the whole subtree
		// allow-list as well.
		var n int64
		if err := tx.Model(link.far).Where("id = ?", id).Limit(1).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			removable = append(removable, id)
		}
	}
	if len(removable) == 0 {
		return nil
	}
	return tx.Exec("DELETE FROM "+link.table+" WHERE "+link.ownerCol+" = ? AND "+link.farCol+" IN ?",
		ownerID, removable).Error
}
