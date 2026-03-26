package application_context

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

func (ctx *MahresourcesContext) BulkAddTagsToNotes(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 {
		return fmt.Errorf("at least one note ID is required")
	}
	if len(query.EditedId) == 0 {
		return fmt.Errorf("at least one tag ID is required")
	}

	uniqueEditedIds := deduplicateUints(query.EditedId)

	uniqueNoteIds := deduplicateUints(query.ID)

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		// Verify all note IDs exist
		var noteCount int64
		if err := tx.Model(&models.Note{}).Where("id IN ?", uniqueNoteIds).Count(&noteCount).Error; err != nil {
			return err
		}
		if int(noteCount) != len(uniqueNoteIds) {
			return fmt.Errorf("one or more notes not found")
		}

		var tagCount int64
		if err := tx.Model(&models.Tag{}).Where("id IN ?", uniqueEditedIds).Count(&tagCount).Error; err != nil {
			return err
		}
		if int(tagCount) != len(uniqueEditedIds) {
			return fmt.Errorf("one or more tags not found")
		}

		for _, tagID := range uniqueEditedIds {
			if err := tx.Exec(
				"INSERT INTO note_tags (note_id, tag_id) SELECT id, ? FROM notes WHERE id IN ? ON CONFLICT DO NOTHING",
				tagID, query.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (ctx *MahresourcesContext) BulkRemoveTagsFromNotes(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 {
		return fmt.Errorf("at least one note ID is required")
	}
	if len(query.EditedId) == 0 {
		return fmt.Errorf("at least one tag ID is required")
	}

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(
			"DELETE FROM note_tags WHERE note_id IN ? AND tag_id IN ?",
			query.ID, query.EditedId,
		).Error
	})
}

func (ctx *MahresourcesContext) BulkAddGroupsToNotes(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 {
		return fmt.Errorf("at least one note ID is required")
	}
	if len(query.EditedId) == 0 {
		return fmt.Errorf("at least one group ID is required")
	}

	uniqueEditedIds := deduplicateUints(query.EditedId)

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		var groupCount int64
		if err := tx.Model(&models.Group{}).Where("id IN ?", uniqueEditedIds).Count(&groupCount).Error; err != nil {
			return err
		}
		if int(groupCount) != len(uniqueEditedIds) {
			return fmt.Errorf("one or more groups not found")
		}

		for _, groupID := range uniqueEditedIds {
			if err := tx.Exec(
				"INSERT INTO groups_related_notes (note_id, group_id) SELECT id, ? FROM notes WHERE id IN ? ON CONFLICT DO NOTHING",
				groupID, query.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (ctx *MahresourcesContext) BulkAddMetaToNotes(query *query_models.BulkEditMetaQuery) error {
	if len(query.ID) == 0 {
		return fmt.Errorf("at least one note ID is required")
	}

	if err := ValidateMeta(query.Meta); err != nil {
		return err
	}

	// Verify all note IDs exist
	var count int64
	if err := ctx.db.Model(&models.Note{}).Where("id IN ?", query.ID).Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(deduplicateUints(query.ID)) {
		return fmt.Errorf("one or more notes not found")
	}

	var note models.Note
	var expr clause.Expr

	if ctx.Config.DbType == constants.DbTypePosgres {
		expr = gorm.Expr("meta || ?", query.Meta)
	} else {
		expr = gorm.Expr("json_patch(meta, ?)", query.Meta)
	}

	return ctx.db.
		Model(&note).
		Where("id in ?", query.ID).
		Update("Meta", expr).Error
}

func (ctx *MahresourcesContext) BulkDeleteNotes(query *query_models.BulkQuery) error {
	return ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		for _, id := range query.ID {
			if err := altCtx.DeleteNote(id); err != nil {
				return err
			}
		}
		return nil
	})
}
