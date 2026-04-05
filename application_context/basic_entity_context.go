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

// UpdateMetaAtPath atomically sets a single value at a dot-notation path in the
// entity's Meta column, creating intermediate objects as needed and preserving
// sibling fields at every level. Uses a single atomic UPDATE statement to avoid
// concurrent-request clobber.
//
// SQLite: json_patch (RFC 7396 recursive merge).
// Postgres: chained jsonb_set calls that ensure each intermediate path segment
// exists before setting the leaf value.
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

	var metaExpr clause.Expr
	if w.ctx.Config.DbType == constants.DbTypePosgres {
		metaExpr = buildPostgresDeepSet(parts, value)
	} else {
		patch := buildNestedJSON(parts, value)
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
	var metaStr string
	row := w.ctx.db.Table(tableName).Where("id = ?", id).Select("meta").Row()
	if err := row.Scan(&metaStr); err != nil {
		return nil, err
	}

	return json.RawMessage(metaStr), nil
}

// buildNestedJSON builds a nested JSON patch from dot-notation path parts and value.
// E.g., ["cooking","time"] + 30 → {"cooking":{"time":30}}
func buildNestedJSON(parts []string, value json.RawMessage) string {
	result := string(value)
	for i := len(parts) - 1; i >= 0; i-- {
		keyJSON, _ := json.Marshal(parts[i])
		result = fmt.Sprintf("{%s:%s}", string(keyJSON), result)
	}
	return result
}

// buildPostgresDeepSet builds an atomic jsonb_set expression chain for Postgres.
// For path [a, b, c] and value V, it generates:
//
//	jsonb_set(
//	  jsonb_set(
//	    jsonb_set(COALESCE(meta, '{}'), '{a}', COALESCE(meta->'a', '{}'), true),
//	    '{a,b}', COALESCE(meta#>'{a,b}', '{}'), true),
//	  '{a,b,c}', V::jsonb, true)
//
// Each intermediate step ensures the parent object exists (preserving its
// contents if present, creating {} if absent). The leaf sets the final value.
// Because it's a single UPDATE expression, it's atomic — concurrent requests
// to different paths don't clobber each other.
func buildPostgresDeepSet(parts []string, value json.RawMessage) clause.Expr {
	// pgPath formats a Postgres text[] literal: ["a","b"] → {a,b}
	pgPath := func(segs []string) string {
		return "{" + strings.Join(segs, ",") + "}"
	}

	// Single-segment path: just set the key directly.
	if len(parts) == 1 {
		return gorm.Expr(
			"jsonb_set(COALESCE(meta, '{}'::jsonb), ?::text[], ?::jsonb, true)",
			pgPath(parts), string(value),
		)
	}

	// Multi-segment: build the SQL and args dynamically.
	// Structure (for path a.b.c, value V):
	//   jsonb_set(                                          ← leaf
	//     jsonb_set(                                        ← ensure a.b
	//       jsonb_set(COALESCE(meta, '{}'),                 ← ensure a
	//         '{a}', COALESCE(meta->'a', '{}'), true),
	//       '{a,b}', COALESCE(meta#>'{a,b}', '{}'), true),
	//     '{a,b,c}', V::jsonb, true)

	var sql strings.Builder
	var args []any

	// Open all jsonb_set calls: one for each intermediate + one for the leaf
	for i := 0; i < len(parts); i++ {
		sql.WriteString("jsonb_set(")
	}

	// Innermost base
	sql.WriteString("COALESCE(meta, '{}'::jsonb)")

	// Close intermediate ensures (all but the last segment)
	for i := 0; i < len(parts)-1; i++ {
		p := pgPath(parts[:i+1])
		if i == 0 {
			// Single-key access: meta->'key'
			sql.WriteString(", ?::text[], COALESCE(meta->?, '{}'::jsonb), true)")
			args = append(args, p, parts[0])
		} else {
			// Multi-key access: meta#>'{a,b}'
			sql.WriteString(", ?::text[], COALESCE(meta#>?::text[], '{}'::jsonb), true)")
			args = append(args, p, p)
		}
	}

	// Close the leaf jsonb_set
	sql.WriteString(", ?::text[], ?::jsonb, true)")
	args = append(args, pgPath(parts), string(value))

	return gorm.Expr(sql.String(), args...)
}
