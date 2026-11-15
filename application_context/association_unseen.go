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
	// entities builds the far side's model slice for Association.Delete.
	entities func(ids []uint) any
}

var (
	groupEntities    = func(ids []uint) any { return BuildAssociationSlice(ids, GroupFromID) }
	noteEntities     = func(ids []uint) any { return BuildAssociationSlice(ids, NoteFromID) }
	resourceEntities = func(ids []uint) any { return BuildAssociationSlice(ids, ResourceFromID) }

	noteGroupsLink     = joinLink{"groups_related_notes", "note_id", "group_id", "groups", &models.Group{}, groupEntities}
	noteResourcesLink  = joinLink{"resource_notes", "note_id", "resource_id", "resources", &models.Resource{}, resourceEntities}
	resourceGroupsLink = joinLink{"groups_related_resources", "resource_id", "group_id", "groups", &models.Group{}, groupEntities}
	resourceNotesLink  = joinLink{"resource_notes", "resource_id", "note_id", "notes", &models.Note{}, noteEntities}
)

// linkChunk bounds the IDs one statement binds.
const linkChunk = 500

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
// which carries the scope the update itself runs under. The rows go through
// Association.Delete, which also drops them from owner's preloaded slice, so
// a later Save cannot write them back.
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

	// Work in chunks: one entity can have tens of thousands of links, and
	// SQLite refuses a statement with more than 32766 bound variables.
	removable := make([]uint, 0, len(ids))
	for start := 0; start < len(ids); start += linkChunk {
		chunk := ids[start:min(start+linkChunk, len(ids))]

		// Raw bypasses the scope callbacks, so this sees every row that exists.
		var existing []uint
		if err := tx.Raw("SELECT id FROM "+link.farTable+" WHERE id IN ?", chunk).Scan(&existing).Error; err != nil {
			return err
		}
		// The scoped query sees only what the caller may see.
		var visible []uint
		if err := tx.Model(link.far).Where("id IN ?", chunk).Pluck("id", &visible).Error; err != nil {
			return err
		}
		exists := make(map[uint]bool, len(existing))
		for _, id := range existing {
			exists[id] = true
		}
		sees := make(map[uint]bool, len(visible))
		for _, id := range visible {
			sees[id] = true
		}
		for _, id := range chunk {
			if !exists[id] || sees[id] {
				removable = append(removable, id)
			}
		}
	}
	for start := 0; start < len(removable); start += linkChunk {
		chunk := removable[start:min(start+linkChunk, len(removable))]
		if err := tx.Model(owner).Association(assoc).Delete(link.entities(chunk)); err != nil {
			return err
		}
	}
	return nil
}
