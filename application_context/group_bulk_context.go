package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/constants"
	"mahresources/lib"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"mahresources/server/interfaces"
)

func (ctx *MahresourcesContext) MergeGroups(winnerId uint, loserIds []uint) error {
	if len(loserIds) == 0 {
		return errors.New("one or more losers required")
	}

	for _, id := range loserIds {
		if id == winnerId {
			return errors.New("winner cannot also be the loser")
		}
	}

	return ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		// Load losers WITHOUT associations — we only need their basic fields for backup
		var losers []*models.Group
		if loadErr := altCtx.db.Find(&losers, &loserIds).Error; loadErr != nil {
			return loadErr
		}

		// Load winner WITHOUT associations
		var winner models.Group
		if err := altCtx.db.First(&winner, winnerId).Error; err != nil {
			return err
		}

		// Batch SQL transfers — tags
		if err := altCtx.db.Exec("INSERT INTO group_tags (group_id, tag_id) SELECT ?, tag_id FROM group_tags WHERE group_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
			return err
		}

		// Batch SQL transfers — related groups (exclude self-references)
		if err := altCtx.db.Exec("INSERT INTO group_related_groups (group_id, related_group_id) SELECT ?, related_group_id FROM group_related_groups WHERE group_id IN ? AND related_group_id != ? ON CONFLICT DO NOTHING", winnerId, loserIds, winnerId).Error; err != nil {
			return err
		}

		// Batch SQL transfers — related notes
		if err := altCtx.db.Exec("INSERT INTO groups_related_notes (group_id, note_id) SELECT ?, note_id FROM groups_related_notes WHERE group_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
			return err
		}

		// Batch SQL transfers — related resources
		if err := altCtx.db.Exec("INSERT INTO groups_related_resources (group_id, resource_id) SELECT ?, resource_id FROM groups_related_resources WHERE group_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
			return err
		}

		// Batch SQL transfers — group_relations (both directions)
		if err := altCtx.db.Exec("INSERT INTO group_relations (from_group_id, to_group_id) SELECT ?, to_group_id FROM group_relations WHERE from_group_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
			return err
		}
		if err := altCtx.db.Exec("INSERT INTO group_relations (from_group_id, to_group_id) SELECT from_group_id, ? FROM group_relations WHERE to_group_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
			return err
		}

		// Batch SQL transfers — ownership updates
		if err := altCtx.db.Exec("UPDATE groups SET owner_id = ? WHERE owner_id IN ?", winnerId, loserIds).Error; err != nil {
			return err
		}
		if err := altCtx.db.Exec("UPDATE notes SET owner_id = ? WHERE owner_id IN ?", winnerId, loserIds).Error; err != nil {
			return err
		}
		if err := altCtx.db.Exec("UPDATE resources SET owner_id = ? WHERE owner_id IN ?", winnerId, loserIds).Error; err != nil {
			return err
		}

		backups := make(map[string]types.JSON)

		for _, loser := range losers {
			// Handle owner_id self-reference: if winner is owned by a loser, clear it
			if winner.OwnerId != nil && loser.ID == *winner.OwnerId {
				if err := altCtx.db.Exec(`UPDATE groups SET owner_id = NULL WHERE id = ?`, winnerId).Error; err != nil {
					return err
				}
			}

			backupData, err := json.Marshal(loser)
			if err != nil {
				return err
			}
			backups[fmt.Sprintf("group_%v", loser.ID)] = backupData

			// Merge meta
			switch altCtx.Config.DbType {
			case constants.DbTypePosgres:
				err = altCtx.db.Exec(`UPDATE groups SET meta = coalesce((SELECT meta FROM groups WHERE id = ?), '{}'::jsonb) || meta WHERE id = ?`, loser.ID, winnerId).Error
			case constants.DbTypeSqlite:
				err = altCtx.db.Exec(`UPDATE groups SET meta = json_patch(meta, coalesce((SELECT meta FROM groups WHERE id = ?), '{}')) WHERE id = ?`, loser.ID, winnerId).Error
			default:
				err = errors.New("db doesn't support merging meta")
			}
			if err != nil {
				return err
			}

			if err := altCtx.DeleteGroup(loser.ID); err != nil {
				return err
			}
		}

		// Save backups to winner's meta
		backupObj := make(map[string]any)
		backupObj["backups"] = backups
		backupsBytes, err := json.Marshal(&backupObj)
		if err != nil {
			return err
		}

		if ctx.Config.DbType == constants.DbTypePosgres {
			if err := altCtx.db.Exec("update groups set meta = meta || ? where id = ?", backupsBytes, winner.ID).Error; err != nil {
				return err
			}
		} else if ctx.Config.DbType == constants.DbTypeSqlite {
			if err := altCtx.db.Exec("update groups set meta = json_patch(meta, ?) where id = ?", backupsBytes, winner.ID).Error; err != nil {
				return err
			}
		}

		// Clean up any self-referential group relations created during the merge
		if err := altCtx.db.Exec(`DELETE FROM group_relations WHERE to_group_id = from_group_id`).Error; err != nil {
			return err
		}

		return nil
	})
}

func (ctx *MahresourcesContext) GroupMetaKeys() ([]interfaces.MetaKey, error) {
	return metaKeys(ctx, "groups")
}

func (ctx *MahresourcesContext) BulkAddTagsToGroups(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		var tagCount int64
		if err := tx.Model(&models.Tag{}).Where("id IN ?", query.EditedId).Count(&tagCount).Error; err != nil {
			return err
		}
		if int(tagCount) != len(query.EditedId) {
			return fmt.Errorf("one or more tags not found")
		}

		for _, tagID := range query.EditedId {
			if err := tx.Exec(
				"INSERT INTO group_tags (group_id, tag_id) SELECT id, ? FROM groups WHERE id IN ? ON CONFLICT DO NOTHING",
				tagID, query.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (ctx *MahresourcesContext) BulkRemoveTagsFromGroups(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(
			"DELETE FROM group_tags WHERE group_id IN ? AND tag_id IN ?",
			query.ID, query.EditedId,
		).Error
	})
}

func (ctx *MahresourcesContext) BulkAddMetaToGroups(query *query_models.BulkEditMetaQuery) error {
	if !json.Valid([]byte(query.Meta)) {
		return errors.New("invalid json")
	}

	var group models.Group
	var expr clause.Expr

	if ctx.Config.DbType == constants.DbTypePosgres {
		expr = gorm.Expr("meta || ?", query.Meta)
	} else {
		expr = gorm.Expr("json_patch(meta, ?)", query.Meta)
	}

	return ctx.db.
		Model(&group).
		Where("id in ?", query.ID).
		Update("Meta", expr).Error
}

func (ctx *MahresourcesContext) BulkDeleteGroups(query *query_models.BulkQuery) error {
	return ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		for _, id := range query.ID {
			if err := altCtx.DeleteGroup(id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (ctx *MahresourcesContext) FindParentsOfGroup(id uint) ([]models.Group, error) {
	var results []models.Group
	var ids []uint

	findIdErr := ctx.db.Raw(`
		WITH RECURSIVE cte AS (
			SELECT id, owner_id, 1 AS level FROM groups WHERE id = ?
			UNION ALL
			SELECT g.id, g.owner_id, cte.level + 1 AS level FROM groups g
			INNER JOIN cte ON cte.owner_id = g.id
			WHERE cte.level < 20
		)
		SELECT id
		FROM cte
		ORDER BY level;
	`, id).Scan(&ids).Error

	if findIdErr != nil {
		return nil, findIdErr
	}

	findIdErr = ctx.db.Find(&results, ids).Error

	if findIdErr != nil {
		return nil, findIdErr
	}

	sort.Slice(results, func(i, j int) bool {
		return lib.IndexOf(ids, results[i].ID) > lib.IndexOf(ids, results[j].ID)
	})

	return results, nil
}

func (ctx *MahresourcesContext) DuplicateGroup(id uint) (*models.Group, error) {
	var result *models.Group
	var original models.Group

	if err := ctx.db.Preload(clause.Associations).First(&original, id).Error; err != nil {
		return nil, err
	}

	result = &models.Group{
		Name:             original.Name,
		Description:      original.Description,
		URL:              original.URL,
		Meta:             original.Meta,
		OwnerId:          original.OwnerId,
		RelatedResources: original.RelatedResources,
		RelatedNotes:     original.RelatedNotes,
		RelatedGroups:    original.RelatedGroups,
		Tags:             original.Tags,
		CategoryId:       original.CategoryId,
	}

	return result, ctx.db.Save(result).Error
}
