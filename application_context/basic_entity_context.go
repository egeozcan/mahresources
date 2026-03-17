package application_context

import (
	"encoding/json"
	"mahresources/server/interfaces"

	"gorm.io/gorm"
)

type EntityWriter[T interfaces.BasicEntityReader] struct {
	ctx *MahresourcesContext
}

func NewEntityWriter[T interfaces.BasicEntityReader](ctx *MahresourcesContext) *EntityWriter[T] {
	return &EntityWriter[T]{ctx: ctx}
}

func (w *EntityWriter[T]) UpdateName(id uint, name string) error {
	entity := new(T)
	return w.ctx.db.Model(entity).Where("id = ?", id).Update("name", name).Error
}

func (w *EntityWriter[T]) UpdateDescription(id uint, description string) error {
	entity := new(T)

	return w.ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(entity).Where("id = ?", id).Update("description", description).Error; err != nil {
			return err
		}

		// If this is a Note, sync description to the first text block so that
		// subsequent block operations (which sync block -> description) don't
		// overwrite the new description with stale block content.
		stmt := &gorm.Statement{DB: tx}
		_ = stmt.Parse(entity)
		if stmt.Table == "notes" {
			var blocks []struct {
				ID uint
			}
			if err := tx.Table("note_blocks").
				Select("id").
				Where("note_id = ? AND type = ?", id, "text").
				Order("position ASC").
				Limit(1).
				Find(&blocks).Error; err != nil {
				return err
			}
			if len(blocks) > 0 {
				content, _ := json.Marshal(map[string]string{"text": description})
				if err := tx.Table("note_blocks").Where("id = ?", blocks[0].ID).
					Update("content", content).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})
}
