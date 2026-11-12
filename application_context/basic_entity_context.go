package application_context

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/models"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EntityWriter[T contracts.BasicEntityReader] struct {
	ctx *MahresourcesContext
}

func NewEntityWriter[T contracts.BasicEntityReader](ctx *MahresourcesContext) *EntityWriter[T] {
	return &EntityWriter[T]{ctx: ctx}
}

// Exists reports whether the entity is visible to this context, which for a
// principal-scoped context applies its subtree confinement. It returns
// gorm.ErrRecordNotFound when it is not.
func (w *EntityWriter[T]) Exists(id uint) error {
	var count int64
	if err := w.ctx.db.Model(new(T)).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (w *EntityWriter[T]) UpdateName(id uint, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name must not be empty")
	}
	if err := ValidateEntityName(name, "entity"); err != nil {
		return err
	}
	entity := new(T)
	result := w.ctx.db.Model(entity).Where("id = ?", id).Update("name", name)
	if result.Error != nil {
		// The inline rename editor shows this text to the user, so the driver's
		// "UNIQUE constraint failed: tags.name" must not be what it gets.
		return friendlyUniqueNameError(modelLabel(w.ctx.db, entity), name, result.Error)
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
				Order("position ASC, id ASC").
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
	parts := strings.Split(path, ".")
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("invalid path %q: empty segment", path)
		}
	}
	if !json.Valid(value) {
		return nil, fmt.Errorf("value is not valid JSON")
	}

	entity := new(T)
	stmt := &gorm.Statement{DB: w.ctx.db}
	_ = stmt.Parse(entity)
	tableName := stmt.Table

	var newVal any
	if err := json.Unmarshal(value, &newVal); err != nil {
		return nil, fmt.Errorf("invalid value JSON: %w", err)
	}

	var updatedJSON json.RawMessage
	lockedMergeWrite := func(h *gorm.DB) error {
		var err error
		updatedJSON, err = readMergeWrite[T](h, tableName, id, parts, newVal)
		return err
	}

	// Serialize concurrent writes to the same row.
	// Postgres: GORM transaction with SELECT ... FOR UPDATE.
	// SQLite: mattn/go-sqlite3 ignores TxOptions.Isolation, so we use a
	// dedicated connection with BEGIN IMMEDIATE to acquire the write lock up
	// front. Both run on w.ctx.db's handle, so its subtree scope travels with
	// them, but without its cancellation: a request ending between BEGIN and
	// ROLLBACK must not hand the pool a connection still holding the lock.
	db := w.ctx.db.WithContext(context.WithoutCancel(w.ctx.db.Statement.Context))
	var err error
	if w.ctx.Config.DbType != constants.DbTypePosgres {
		err = db.Connection(func(conn *gorm.DB) error {
			conn = conn.Session(&gorm.Session{})
			if err := conn.Exec("BEGIN IMMEDIATE").Error; err != nil {
				return err
			}
			if err := lockedMergeWrite(conn); err != nil {
				conn.Exec("ROLLBACK")
				return err
			}
			return conn.Exec("COMMIT").Error
		})
	} else {
		err = db.Transaction(lockedMergeWrite)
	}
	if err != nil {
		return nil, err
	}

	return updatedJSON, nil
}

// readMergeWrite performs the read-modify-write of UpdateMetaAtPath inside the
// transaction h is bound to. The read goes through GORM, not Scan or raw SQL, so
// the subtree scope callback applies: an entity outside a group-limited
// principal's subtree reads as not found, before anything is written and before
// any of its Meta reaches the caller. On Postgres the read also locks the row
// (FOR UPDATE; SQLite ignores the clause and already holds the writer lock),
// which pins owner_id, the column the resource and note scope tests, until the
// write commits.
func readMergeWrite[T contracts.BasicEntityReader](h *gorm.DB, tableName string, id uint, parts []string, newVal any) (json.RawMessage, error) {
	var metas []sql.NullString
	if err := h.Model(new(T)).Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Where("id = ?", id).Pluck("meta", &metas).Error; err != nil {
		return nil, err
	}
	if len(metas) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var metaStr *string
	if metas[0].Valid {
		metaStr = &metas[0].String
	}
	return mergeAndWrite(func(query string, args ...any) (int64, error) {
		result := h.Exec(query, args...)
		return result.RowsAffected, result.Error
	}, "UPDATE "+tableName+" SET meta = ?, updated_at = ? WHERE id = ?",
		parts, newVal, metaStr, id)
}

// mergeAndWrite parses the current meta, applies setNestedValue, and executes
// the caller-provided UPDATE statement with the encoded JSON, the new
// updated_at stamp, and the entity ID — in that parameter order.
func mergeAndWrite(exec func(string, ...any) (int64, error), updateSQL string, parts []string, newVal any, metaStr *string, id uint) (json.RawMessage, error) {
	var meta map[string]any
	if metaStr != nil && *metaStr != "" {
		if err := json.Unmarshal([]byte(*metaStr), &meta); err != nil {
			return nil, fmt.Errorf("existing meta is corrupt JSON: %w", err)
		}
	} else {
		meta = make(map[string]any)
	}

	setNestedValue(meta, parts, newVal)

	encoded, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}

	// Stamp updated_at from Go's clock rather than SQL's CURRENT_TIMESTAMP.
	// SQLite renders that as whole-second UTC, while every other write path
	// stores GORM's nanosecond value — so a row edited in the same second it
	// was written came back with an UpdatedAt *earlier* than before. Anything
	// ordering by that column believed the edit was the older state; the
	// [reload] shortcode used it to discard freshly rendered content as stale.
	rows, err := exec(updateSQL, string(encoded), time.Now(), id)
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	return encoded, nil
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
