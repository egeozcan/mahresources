package application_context

import (
	"encoding/json"
	"errors"
	"fmt"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

func (ctx *MahresourcesContext) MergeTags(winnerId uint, loserIds []uint) error {
	if len(loserIds) == 0 {
		return errors.New("one or more losers required")
	}

	for _, id := range loserIds {
		if id == 0 {
			return errors.New("invalid tag ID")
		}
		if id == winnerId {
			return errors.New("winner cannot also be the loser")
		}
	}

	if winnerId == 0 {
		return errors.New("invalid winner ID")
	}

	return ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		var losers []*models.Tag
		if err := altCtx.db.Find(&losers, &loserIds).Error; err != nil {
			return err
		}

		var winner models.Tag
		if err := altCtx.db.First(&winner, winnerId).Error; err != nil {
			return err
		}

		// Transfer resource_tags
		if err := altCtx.db.Exec(
			"INSERT INTO resource_tags (resource_id, tag_id) SELECT resource_id, ? FROM resource_tags WHERE tag_id IN ? ON CONFLICT DO NOTHING",
			winnerId, loserIds,
		).Error; err != nil {
			return err
		}

		// Transfer note_tags
		if err := altCtx.db.Exec(
			"INSERT INTO note_tags (note_id, tag_id) SELECT note_id, ? FROM note_tags WHERE tag_id IN ? ON CONFLICT DO NOTHING",
			winnerId, loserIds,
		).Error; err != nil {
			return err
		}

		// Transfer group_tags
		if err := altCtx.db.Exec(
			"INSERT INTO group_tags (group_id, tag_id) SELECT group_id, ? FROM group_tags WHERE tag_id IN ? ON CONFLICT DO NOTHING",
			winnerId, loserIds,
		).Error; err != nil {
			return err
		}

		// Build backup data
		backups := make(map[string]types.JSON)
		for _, loser := range losers {
			backupData, err := json.Marshal(loser)
			if err != nil {
				return err
			}
			backups[fmt.Sprintf("tag_%v", loser.ID)] = backupData
		}

		backupObj := map[string]any{"backups": backups}
		backupsBytes, err := json.Marshal(&backupObj)
		if err != nil {
			return err
		}

		// Save backups to winner's meta (DB-specific)
		switch altCtx.Config.DbType {
		case constants.DbTypePosgres:
			if err := altCtx.db.Exec(
				"UPDATE tags SET meta = COALESCE(meta, '{}'::jsonb) || ? WHERE id = ?",
				backupsBytes, winner.ID,
			).Error; err != nil {
				return err
			}
		case constants.DbTypeSqlite:
			if err := altCtx.db.Exec(
				"UPDATE tags SET meta = json_patch(COALESCE(meta, '{}'), ?) WHERE id = ?",
				string(backupsBytes), winner.ID,
			).Error; err != nil {
				return err
			}
		default:
			return errors.New("db doesn't support merging meta")
		}

		// Delete losers
		for _, loser := range losers {
			if err := altCtx.DeleteTag(loser.ID); err != nil {
				return err
			}
		}

		return nil
	})
}

func (ctx *MahresourcesContext) BulkDeleteTags(query *query_models.BulkQuery) error {
	return ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		for _, id := range query.ID {
			if err := altCtx.DeleteTag(id); err != nil {
				return err
			}
		}
		return nil
	})
}
