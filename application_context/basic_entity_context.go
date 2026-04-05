package application_context

import (
	"database/sql"
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

// UpdateMetaAtPath sets a single value at a dot-notation path in the entity's
// Meta column, creating intermediate objects as needed and preserving sibling
// fields at every level. Uses a transaction with row-level locking (Postgres
// FOR UPDATE, SQLite implicit write lock) to prevent concurrent clobber.
// Behavior is identical on both databases — null stores JSON null, intermediate
// scalars/arrays are overwritten to objects.
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
	parts := strings.Split(path, ".")

	var newVal any
	if err := json.Unmarshal(value, &newVal); err != nil {
		return nil, fmt.Errorf("invalid value JSON: %w", err)
	}

	var updatedJSON json.RawMessage

	// Serialize concurrent writes to the same row.
	// Postgres: FOR UPDATE on the SELECT locks the row.
	// SQLite: BEGIN IMMEDIATE (via LevelSerializable) acquires the write lock
	// up front so overlapping transactions queue instead of racing.
	txOpts := &sql.TxOptions{}
	if w.ctx.Config.DbType != constants.DbTypePosgres {
		txOpts.Isolation = sql.LevelSerializable
	}

	tx := w.ctx.db.Begin(txOpts)
	if tx.Error != nil {
		return nil, tx.Error
	}

	err := func() error {
		var metaStr *string
		q := tx.Table(tableName).Where("id = ?", id).Select("meta")
		if w.ctx.Config.DbType == constants.DbTypePosgres {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Row().Scan(&metaStr); err != nil {
			return gorm.ErrRecordNotFound
		}

		var meta map[string]any
		if metaStr != nil && *metaStr != "" {
			if err := json.Unmarshal([]byte(*metaStr), &meta); err != nil {
				meta = make(map[string]any)
			}
		} else {
			meta = make(map[string]any)
		}

		setNestedValue(meta, parts, newVal)

		encoded, err := json.Marshal(meta)
		if err != nil {
			return err
		}

		result := tx.Table(tableName).Where("id = ?", id).Update("meta", string(encoded))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		updatedJSON = encoded
		return nil
	}()

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	return updatedJSON, tx.Commit().Error
}

// setNestedValue sets a value at a dot-notation path in a map,
// creating intermediate objects as needed and preserving siblings.
// If an intermediate key exists as a non-object (scalar, array), it is
// overwritten with an empty object so the deeper path can be created.
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
