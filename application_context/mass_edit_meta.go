package application_context

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

// This file holds the mass-edit meta ops and the series/OwnMeta logic.
// Meta is the only dialect split: every join-table op above is engine-neutral.
//
// Resources have TWO meta columns. Meta is the *effective* value (series ⊕
// own); OwnMeta is the resource's overlay, and an explicit null in it is the
// only mechanism suppressing a series-inherited key (mergeMeta,
// series_context.go). Notes and Groups have no OwnMeta and no series, so all
// three meta ops are single-column for them.

// parseMetaOps builds the meta op from MetaOp/Meta/MetaKeys. An empty MetaOp
// means the op is absent.
func parseMetaOps(ctx *MahresourcesContext, spec massEditSpec, q *query_models.MassEditQuery) ([]massEditOp, error) {
	switch q.MetaOp {
	case "":
		return nil, nil

	case "merge":
		if strings.TrimSpace(q.Meta) == "" {
			return nil, fmt.Errorf("a meta JSON object is required when merging meta")
		}
		if err := ValidateMeta(q.Meta); err != nil {
			return nil, err
		}
		// On Postgres every explicit-null key adds a `- ?` removal to the merge
		// expression (per column), so the count is bounded like removeKeys.
		if ctx.db.Dialector.Name() == "postgres" {
			columns := 1
			if spec.entity == "resource" {
				columns = 2
			}
			nullKeys := len(nullValuedMetaKeys(q.Meta))
			if nullKeys > massEditMaxMetaKeys {
				return nil, fmt.Errorf("%w: %d meta keys exceeds the ceiling of %d", ErrMassEditTooLarge, nullKeys, massEditMaxMetaKeys)
			}
			if budget := ctx.massEditBindBudget(true, massEditChunkSize+16) / columns; nullKeys > budget {
				return nil, fmt.Errorf("%w: %d meta keys exceeds the database parameter budget of %d",
					ErrMassEditTooLarge, nullKeys, budget)
			}
		}
		return []massEditOp{{
			name:  "meta.merge",
			apply: mergeMassEditMetaApply(spec, q.Meta),
		}}, nil

	case "removeKeys":
		if len(q.MetaKeys) == 0 {
			return nil, fmt.Errorf("at least one meta key is required when removing meta keys")
		}
		for _, key := range q.MetaKeys {
			if err := validateMassEditMetaKey(key); err != nil {
				return nil, err
			}
		}
		// Deduplicated (a repeated key removes nothing twice) and bounded: the
		// remove statement binds one placeholder per key per COLUMN (resources
		// carry meta and own_meta), beside the target chunk and — for a scoped
		// principal — the whole scope allow-list the update callback appends.
		keys := deduplicateStrings(q.MetaKeys)
		columns := 1
		if spec.entity == "resource" {
			columns = 2
		}
		if len(keys) > massEditMaxMetaKeys {
			return nil, fmt.Errorf("%w: %d meta keys exceeds the ceiling of %d", ErrMassEditTooLarge, len(keys), massEditMaxMetaKeys)
		}
		if budget := ctx.massEditBindBudget(true, massEditChunkSize+16) / columns; len(keys) > budget {
			return nil, fmt.Errorf("%w: %d meta keys exceeds the database parameter budget of %d",
				ErrMassEditTooLarge, len(keys), budget)
		}
		return []massEditOp{{
			name:  "meta.removeKeys",
			apply: removeMassEditMetaKeysApply(spec, keys),
		}}, nil

	case "replace":
		if strings.TrimSpace(q.Meta) == "" {
			return nil, fmt.Errorf("a meta JSON object is required when replacing meta")
		}
		if err := ValidateMeta(q.Meta); err != nil {
			return nil, err
		}
		return []massEditOp{{
			name:  "meta.replace",
			apply: replaceMassEditMetaApply(spec, q.Meta),
		}}, nil

	default:
		return nil, massEditVerbError("metaOp", q.MetaOp, "merge", "removeKeys", "replace")
	}
}

// validateMassEditMetaKey refuses anything that is not a top-level key name:
// removeKeys is top-level keys only, and a key containing "." or "$" would be
// read as a JSON path by anybody maintaining the SQL; a key containing a quote
// would break the bound, quoted path spelling itself.
func validateMassEditMetaKey(key string) error {
	if key == "" {
		return fmt.Errorf("a meta key is required and must not be empty")
	}
	if strings.ContainsAny(key, `."$`) {
		return fmt.Errorf("meta key %q must be a top-level key and cannot contain '.', '$' or '\"'", key)
	}
	return nil
}

// --- dialect expressions -------------------------------------------------

// metaMergeExpr builds the merge expression for one column, WITH the
// COALESCE/NULLIF wrapper. The wrapper is not decoration: both engines return
// NULL for a NULL input, so merging onto a row whose meta is SQL NULL would
// blow the column away. Every other meta path in the tree already has the
// wrapper; BulkAddMetaToResources was the odd one out and is fixed to match.
//
// nullKeys are the keys the patch carries with an explicit null value. On
// SQLite json_patch already deletes them; Postgres's jsonb || STORES
// "k": null — the same input, a different meta. Chaining a - removal per null
// key after the merge keeps the two engines JSON-equivalent, which is what the
// series mergeMeta semantics (an explicit null deletes the key) require.
func metaMergeExpr(postgres bool, column string, nullKeys []string) (string, []any) {
	if postgres {
		// Parenthesised: without them the engine reads meta || patch - key
		// right-to-left through the operator classes and the removal never
		// lands where it is meant to.
		expr := fmt.Sprintf("(COALESCE(NULLIF(%s,'null'::jsonb),'{}'::jsonb) || ?)", column)
		vars := make([]any, 0, len(nullKeys))
		for _, k := range nullKeys {
			expr += " - ?"
			vars = append(vars, k)
		}
		return expr, vars
	}
	// json_patch already deletes explicit-null keys; no removals needed.
	return fmt.Sprintf("json_patch(COALESCE(NULLIF(%s,'null'),'{}'), ?)", column), nil
}

// metaRemoveExprArgs returns the removal expression and its bound arguments.
// SQLite paths are quoted and bound: `$."key1"`, `$."key2"`.
func metaRemoveExprArgs(postgres bool, column string, keys []string) (string, []any) {
	base := fmt.Sprintf("COALESCE(NULLIF(%s,'null'),'{}')", column)
	if postgres {
		base = fmt.Sprintf("COALESCE(NULLIF(%s,'null'::jsonb),'{}'::jsonb)", column)
	}
	args := make([]any, 0, len(keys))
	if postgres {
		expr := base
		for _, key := range keys {
			expr += " - ?"
			args = append(args, key)
		}
		return expr, args
	}
	expr := "json_remove(" + base
	for _, key := range keys {
		expr += ", ?"
		args = append(args, `$."`+key+`"`)
	}
	return expr + ")", args
}

// --- apply closures -------------------------------------------------------

func mergeMassEditMetaApply(spec massEditSpec, metaStr string) func(txCtx *MahresourcesContext, chunk []uint) (int64, error) {
	return func(txCtx *MahresourcesContext, chunk []uint) (int64, error) {
		if spec.entity == "resource" {
			return mergeResourceMassEditMeta(txCtx, chunk, metaStr)
		}
		postgres := txCtx.Config.DbType == constants.DbTypePosgres
		expr, extra := metaMergeExpr(postgres, "meta", nullValuedMetaKeys(metaStr))
		args := append([]any{metaStr}, extra...)
		res := txCtx.db.Model(spec.model()).Where("id IN ?", chunk).Updates(map[string]any{
			"meta": gorm.Expr(expr, args...),
		})
		return res.RowsAffected, res.Error
	}
}

// mergeResourceMassEditMeta patches both columns with the same expression,
// then runs the explicit-null fixup for the affected series members: today's
// behaviour, on the transaction's db this time, because json_patch (SQLite)
// drops null-valued keys from OwnMeta instead of storing them, and only the
// explicit null in OwnMeta suppresses a series-inherited key.
func mergeResourceMassEditMeta(txCtx *MahresourcesContext, chunk []uint, metaStr string) (int64, error) {
	postgres := txCtx.Config.DbType == constants.DbTypePosgres
	nullKeys := nullValuedMetaKeys(metaStr)
	expr, extra := metaMergeExpr(postgres, "meta", nullKeys)
	ownExpr, ownExtra := metaMergeExpr(postgres, "own_meta", nullKeys)
	args := append([]any{metaStr}, extra...)
	ownArgs := append([]any{metaStr}, ownExtra...)
	res := txCtx.db.Model(&models.Resource{}).Where("id IN ?", chunk).Updates(map[string]any{
		"meta":     gorm.Expr(expr, args...),
		"own_meta": gorm.Expr(ownExpr, ownArgs...),
	})
	if res.Error != nil {
		return 0, res.Error
	}
	if len(nullKeys) > 0 {
		if err := applySeriesNullOverrides(txCtx, chunk, nullKeys); err != nil {
			return 0, err
		}
	}
	return res.RowsAffected, nil
}

func removeMassEditMetaKeysApply(spec massEditSpec, keys []string) func(txCtx *MahresourcesContext, chunk []uint) (int64, error) {
	return func(txCtx *MahresourcesContext, chunk []uint) (int64, error) {
		postgres := txCtx.Config.DbType == constants.DbTypePosgres

		if spec.entity != "resource" {
			expr, args := metaRemoveExprArgs(postgres, "meta", keys)
			res := txCtx.db.Model(spec.model()).Where("id IN ?", chunk).Updates(map[string]any{
				"meta": gorm.Expr(expr, args...),
			})
			return res.RowsAffected, res.Error
		}

		// The naive json_remove(meta, …) alone is wrong for series resources:
		// meta loses the key, own_meta has no entry for it, and the next
		// mergeMeta re-inherits it from the series — the key comes back. So the
		// removal applies to BOTH columns, then the same fixup that merge uses
		// re-inserts own_meta[k] = null for exactly those removed keys the
		// resource's series actually defines.
		//
		// removeKeys cannot be implemented as "merge {k: null}":
		// json_patch(meta,'{"k":null}') on SQLite DELETES the key;
		// meta || '{"k":null}' on Postgres STORES "k": null. Same input,
		// different meta. Hence the explicit removal plus the Go-side fixup.
		metaExpr, metaArgs := metaRemoveExprArgs(postgres, "meta", keys)
		ownExpr, ownArgs := metaRemoveExprArgs(postgres, "own_meta", keys)
		res := txCtx.db.Model(&models.Resource{}).Where("id IN ?", chunk).Updates(map[string]any{
			"meta":     gorm.Expr(metaExpr, metaArgs...),
			"own_meta": gorm.Expr(ownExpr, ownArgs...),
		})
		if res.Error != nil {
			return 0, res.Error
		}
		if err := applySeriesNullOverrides(txCtx, chunk, keys); err != nil {
			return 0, err
		}
		return res.RowsAffected, nil
	}
}

func replaceMassEditMetaApply(spec massEditSpec, metaStr string) func(txCtx *MahresourcesContext, chunk []uint) (int64, error) {
	return func(txCtx *MahresourcesContext, chunk []uint) (int64, error) {
		if spec.entity != "resource" {
			res := txCtx.db.Model(spec.model()).Where("id IN ?", chunk).Updates(map[string]any{
				"meta": metaStr,
			})
			return res.RowsAffected, res.Error
		}
		return replaceResourceMassEditMeta(txCtx, chunk, metaStr)
	}
}

// replaceResourceMassEditMeta splits by series membership. meta is the
// *effective* value, so for a series member the correct overlay is
// computeOwnMeta(newMeta, series.Meta, true) — the same call EditResource
// makes. That is not expressible in SQL, so non-series rows take one
// statement per chunk and series rows are updated per row, bounded by the
// series subset rather than the whole set.
func replaceResourceMassEditMeta(txCtx *MahresourcesContext, chunk []uint, metaStr string) (int64, error) {
	// Non-series rows: the remove-from-series branch of EditResource sets
	// own_meta to "{}" beside the effective meta.
	res := txCtx.db.Model(&models.Resource{}).
		Where("id IN ? AND series_id IS NULL", chunk).
		Updates(map[string]any{"meta": metaStr, "own_meta": types.JSON("{}")})
	if res.Error != nil {
		return 0, res.Error
	}
	affected := res.RowsAffected

	var seriesResources []models.Resource
	if err := txCtx.db.Select("id", "series_id").
		Where("id IN ? AND series_id IS NOT NULL", chunk).
		Find(&seriesResources).Error; err != nil {
		return 0, err
	}
	if len(seriesResources) == 0 {
		return affected, nil
	}

	// Load the distinct series metas once.
	seriesIDSet := make(map[uint]struct{})
	for _, r := range seriesResources {
		seriesIDSet[*r.SeriesID] = struct{}{}
	}
	seriesMetas := make(map[uint]types.JSON)
	for seriesID := range seriesIDSet {
		var series models.Series
		if err := txCtx.db.Select("id", "meta").First(&series, seriesID).Error; err != nil {
			return 0, err
		}
		seriesMetas[seriesID] = series.Meta
	}

	for _, r := range seriesResources {
		ownMeta, err := computeOwnMeta(types.JSON(metaStr), seriesMetas[*r.SeriesID], true)
		if err != nil {
			return 0, err
		}
		if err := txCtx.db.Model(&models.Resource{}).Where("id = ?", r.ID).
			Updates(map[string]any{"meta": metaStr, "own_meta": ownMeta}).Error; err != nil {
			return 0, err
		}
		affected++
	}
	return affected, nil
}

// nullValuedMetaKeys returns the top-level keys whose value in the submitted
// JSON object is an explicit null — the keys a merge means to delete.
func nullValuedMetaKeys(metaStr string) []string {
	var patchMap map[string]interface{}
	if err := json.Unmarshal([]byte(metaStr), &patchMap); err != nil {
		return nil
	}
	var nullKeys []string
	for k, v := range patchMap {
		if v == nil {
			nullKeys = append(nullKeys, k)
		}
	}
	return nullKeys
}

// applySeriesNullOverrides re-inserts explicit null entries into own_meta for
// series members among ids, for the given keys, so mergeMeta keeps
// suppressing the keys the series defines that the edit meant to delete.
//
// Extracted from BulkAddMetaToResources, with two fixes while extracting: it
// runs on the TRANSACTION's db (the original ran on ctx.db after the update,
// outside any transaction, so a crash between them left a key gone from meta
// and un-suppressed in own_meta — it silently reappeared), and its WHERE id IN
// is chunked by the caller, because the scope callback appends the whole
// subtree allow-list to the statement.
func applySeriesNullOverrides(txCtx *MahresourcesContext, ids []uint, nullKeys []string) error {
	if len(ids) == 0 || len(nullKeys) == 0 {
		return nil
	}

	// Find affected resources that are in a series.
	var seriesResources []models.Resource
	if err := txCtx.db.Preload("Series").Where("id IN ? AND series_id IS NOT NULL", ids).Find(&seriesResources).Error; err != nil {
		return err
	}

	for _, res := range seriesResources {
		if res.Series == nil {
			continue
		}

		var seriesMeta map[string]interface{}
		if err := json.Unmarshal(res.Series.Meta, &seriesMeta); err != nil {
			continue
		}

		var ownMap map[string]interface{}
		if err := json.Unmarshal(res.OwnMeta, &ownMap); err != nil || ownMap == nil {
			ownMap = make(map[string]interface{})
		}

		changed := false
		for _, k := range nullKeys {
			if _, inSeries := seriesMeta[k]; inSeries {
				ownMap[k] = nil // explicit null override
				changed = true
			}
		}

		if changed {
			newOwnMeta, marshalErr := json.Marshal(ownMap)
			if marshalErr != nil {
				return marshalErr
			}
			if err := txCtx.db.Model(&models.Resource{}).Where("id = ?", res.ID).Update("own_meta", newOwnMeta).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
