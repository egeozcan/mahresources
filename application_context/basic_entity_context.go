package application_context

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mahresources/constants"
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
	// Postgres: GORM transaction with SELECT ... FOR UPDATE.
	// SQLite: mattn/go-sqlite3 ignores TxOptions.Isolation, so we use a raw
	// *sql.Conn with BEGIN IMMEDIATE to acquire the write lock up front.
	if w.ctx.Config.DbType != constants.DbTypePosgres {
		sqlDB, err := w.ctx.db.DB()
		if err != nil {
			return nil, err
		}
		ctx := context.Background()
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return nil, err
		}
		defer conn.Close()

		if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
			return nil, err
		}

		updatedJSON, err = readMergeWrite(ctx, conn, tableName, id, parts, newVal)
		if err != nil {
			conn.ExecContext(ctx, "ROLLBACK")
			return nil, err
		}
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return nil, err
		}
	} else {
		sqlDB, err := w.ctx.db.DB()
		if err != nil {
			return nil, err
		}
		ctx := context.Background()
		sqlTx, err := sqlDB.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}

		// Lock the row with FOR UPDATE.
		updatedJSON, err = readMergeWriteTx(ctx, sqlTx, tableName, id, parts, newVal)
		if err != nil {
			sqlTx.Rollback()
			return nil, err
		}
		if err := sqlTx.Commit(); err != nil {
			return nil, err
		}
	}

	return updatedJSON, nil
}

// readMergeWrite performs a locked read-modify-write on a raw *sql.Conn (SQLite).
func readMergeWrite(ctx context.Context, conn *sql.Conn, tableName string, id uint, parts []string, newVal any) (json.RawMessage, error) {
	var metaStr *string
	if err := conn.QueryRowContext(ctx, "SELECT meta FROM "+tableName+" WHERE id = ?", id).Scan(&metaStr); err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	return mergeAndWrite(ctx, func(query string, args ...any) (sql.Result, error) {
		return conn.ExecContext(ctx, query, args...)
	}, "UPDATE "+tableName+" SET meta = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		parts, newVal, metaStr, id)
}

// readMergeWriteTx performs a locked read-modify-write on a *sql.Tx (Postgres).
func readMergeWriteTx(ctx context.Context, tx *sql.Tx, tableName string, id uint, parts []string, newVal any) (json.RawMessage, error) {
	var metaStr *string
	if err := tx.QueryRowContext(ctx, "SELECT meta FROM "+tableName+" WHERE id = $1 FOR UPDATE", id).Scan(&metaStr); err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	return mergeAndWrite(ctx, func(query string, args ...any) (sql.Result, error) {
		return tx.ExecContext(ctx, query, args...)
	}, "UPDATE "+tableName+" SET meta = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
		parts, newVal, metaStr, id)
}

// mergeAndWrite parses the current meta, applies setNestedValue, and executes
// the caller-provided UPDATE statement with the encoded JSON and entity ID.
func mergeAndWrite(_ context.Context, exec func(string, ...any) (sql.Result, error), updateSQL string, parts []string, newVal any, metaStr *string, id uint) (json.RawMessage, error) {
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
		return nil, err
	}

	result, err := exec(updateSQL, string(encoded), id)
	if err != nil {
		return nil, err
	}
	rows, _ := result.RowsAffected()
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
