package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"mahresources/models"
	"mahresources/server/interfaces"
	"strings"

	"gorm.io/gorm"
)

type EntityWriter[T interfaces.BasicEntityReader] struct {
	ctx *MahresourcesContext
}

func NewEntityWriter[T interfaces.BasicEntityReader](ctx *MahresourcesContext) *EntityWriter[T] {
	return &EntityWriter[T]{ctx: ctx}
}

func (w *EntityWriter[T]) UpdateName(id uint, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name must not be empty")
	}
	entity := new(T)
	result := w.ctx.db.Model(entity).Where("id = ?", id).Update("name", name)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (w *EntityWriter[T]) UpdateDescription(id uint, description string) error {
	entity := new(T)

	// Detect table name for entity-specific behavior
	stmt := &gorm.Statement{DB: w.ctx.db}
	_ = stmt.Parse(entity)
	tableName := stmt.Table

	err := w.ctx.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(entity).Where("id = ?", id).Update("description", description)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		// If this is a Note, sync description to the first text block so that
		// subsequent block operations (which sync block -> description) don't
		// overwrite the new description with stale block content.
		if tableName == "notes" {
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

	if err != nil {
		return err
	}

	// Sync @-mentions after successful commit
	switch tableName {
	case "notes":
		var note models.Note
		if e := w.ctx.db.First(&note, id).Error; e == nil {
			w.ctx.syncMentionsForNote(&note)
		}
	case "groups":
		var group models.Group
		if e := w.ctx.db.First(&group, id).Error; e == nil {
			w.ctx.syncMentionsForGroup(&group)
		}
	case "resources":
		var resource models.Resource
		if e := w.ctx.db.First(&resource, id).Error; e == nil {
			w.ctx.syncMentionsForResource(&resource)
		}
	}

	return nil
}

// UpdateMetaAtPath performs a deep-merge of a single value at a dot-notation path
// into the entity's Meta column. It reads the current meta, sets the value at the
// path (creating intermediate objects as needed), and writes back the result.
// Returns the full updated meta JSON.
func (w *EntityWriter[T]) UpdateMetaAtPath(id uint, path string, value json.RawMessage) (json.RawMessage, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("path must not be empty")
	}
	if !json.Valid(value) {
		return nil, fmt.Errorf("value is not valid JSON")
	}

	entity := new(T)
	stmt := &gorm.Statement{DB: w.ctx.db}
	_ = stmt.Parse(entity)
	tableName := stmt.Table

	// Read current meta.
	var metaStr *string
	row := w.ctx.db.Table(tableName).Where("id = ?", id).Select("meta").Row()
	if err := row.Scan(&metaStr); err != nil {
		return nil, gorm.ErrRecordNotFound
	}

	var meta map[string]any
	if metaStr != nil && *metaStr != "" {
		if err := json.Unmarshal([]byte(*metaStr), &meta); err != nil {
			meta = make(map[string]any)
		}
	} else {
		meta = make(map[string]any)
	}

	// Parse the new value.
	var newVal any
	if err := json.Unmarshal(value, &newVal); err != nil {
		return nil, fmt.Errorf("invalid value JSON: %w", err)
	}

	// Set the value at the path, creating intermediate objects as needed.
	parts := strings.Split(path, ".")
	setNestedValue(meta, parts, newVal)

	updatedJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}

	// Write back.
	result := w.ctx.db.Table(tableName).Where("id = ?", id).Update("meta", string(updatedJSON))
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	return updatedJSON, nil
}

// setNestedValue sets a value at a dot-notation path in a map,
// creating intermediate objects as needed and preserving siblings.
func setNestedValue(m map[string]any, parts []string, value any) {
	current := m
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[part] = next
		}
		current = next
	}
}
