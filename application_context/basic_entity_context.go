package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/server/interfaces"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// buildNestedJSON builds a nested JSON object from a dot-separated path and a JSON value.
// For example, path="cooking.time" and value=30 produces {"cooking":{"time":30}}.
func buildNestedJSON(path string, value json.RawMessage) (string, error) {
	parts := strings.Split(path, ".")
	if !json.Valid(value) {
		return "", fmt.Errorf("value is not valid JSON")
	}
	result := string(value)
	for i := len(parts) - 1; i >= 0; i-- {
		keyJSON, _ := json.Marshal(parts[i])
		result = fmt.Sprintf("{%s:%s}", string(keyJSON), result)
	}
	return result, nil
}

// UpdateMetaAtPath performs a deep-merge of a single value at a dot-notation path
// into the entity's Meta column. It returns the full updated meta JSON.
func (w *EntityWriter[T]) UpdateMetaAtPath(id uint, path string, value json.RawMessage) (json.RawMessage, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("path must not be empty")
	}

	patch, err := buildNestedJSON(path, value)
	if err != nil {
		return nil, err
	}

	entity := new(T)
	stmt := &gorm.Statement{DB: w.ctx.db}
	_ = stmt.Parse(entity)
	tableName := stmt.Table

	var metaExpr clause.Expr
	if w.ctx.Config.DbType == constants.DbTypePosgres {
		metaExpr = gorm.Expr("COALESCE(meta, '{}'::jsonb) || ?::jsonb", patch)
	} else {
		metaExpr = gorm.Expr("json_patch(COALESCE(meta, '{}'), ?)", patch)
	}

	result := w.ctx.db.Table(tableName).Where("id = ?", id).Update("meta", metaExpr)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	// Read back the full updated meta.
	// SQLite returns text columns as string, not []byte, so scan into string first.
	var metaStr string
	row := w.ctx.db.Table(tableName).Where("id = ?", id).Select("meta").Row()
	if err := row.Scan(&metaStr); err != nil {
		return nil, err
	}

	return json.RawMessage(metaStr), nil
}
