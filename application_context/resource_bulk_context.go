package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"mahresources/server/interfaces"
	"path"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (ctx *MahresourcesContext) DeleteResource(resourceId uint) error {
	resource := models.Resource{ID: resourceId}

	if err := ctx.db.Model(&resource).First(&resource).Error; err != nil {
		return err
	}

	fs, storageErr := ctx.GetFsForStorageLocation(resource.StorageLocation)

	if storageErr != nil {
		return storageErr
	}

	subFolder := "deleted"

	if resource.StorageLocation != nil && *resource.StorageLocation != "" {
		subFolder = *resource.StorageLocation
	}

	folder := fmt.Sprintf("/deleted/%v/", subFolder)

	if err := ctx.fs.MkdirAll(folder, 0777); err != nil {
		return err
	}

	ownerIdStr := "nil"
	if resource.OwnerId != nil {
		ownerIdStr = fmt.Sprintf("%v", *resource.OwnerId)
	}
	filePath := path.Join(folder, fmt.Sprintf("%v__%v__%v___%v", resource.Hash, resource.ID, ownerIdStr, strings.ReplaceAll(path.Clean(path.Base(resource.GetCleanLocation())), "\\", "_")))

	file, openErr := fs.Open(resource.GetCleanLocation())

	if openErr == nil {
		backup, createErr := ctx.fs.Create(filePath)

		if createErr != nil {
			_ = file.Close()
			return createErr
		}

		defer backup.Close()

		_, copyErr := io.Copy(backup, file)

		if copyErr != nil {
			_ = file.Close()
			return copyErr
		}

		_ = file.Close()
	}

	// Clear CurrentVersionID to break circular reference before deletion
	// This prevents foreign key constraint errors when deleting resources with versions
	if resource.CurrentVersionID != nil {
		if err := ctx.db.Model(&resource).Update("current_version_id", nil).Error; err != nil {
			return err
		}
	}

	if err := ctx.db.Select(clause.Associations).Delete(&resource).Error; err != nil {
		return err
	}

	// Auto-delete empty series if this resource was in one.
	// Uses a conditional delete to avoid race conditions: only deletes the series
	// if no other resources reference it at the time of deletion.
	if resource.SeriesID != nil {
		seriesID := *resource.SeriesID
		result := ctx.db.Where("id = ? AND NOT EXISTS (SELECT 1 FROM resources WHERE series_id = ?)", seriesID, seriesID).Delete(&models.Series{})
		if result.Error != nil {
			ctx.Logger().Warning(models.LogActionDelete, "series", &seriesID, "Failed to auto-delete empty series", result.Error.Error(), nil)
		} else if result.RowsAffected > 0 {
			ctx.Logger().Info(models.LogActionDelete, "series", &seriesID, "", "Auto-deleted empty series", nil)
		}
	}

	// Check if any other resources or versions reference this hash
	refCount, countErr := ctx.CountHashReferences(resource.Hash)
	if countErr != nil {
		ctx.Logger().Warning(models.LogActionDelete, "resource", &resourceId, "Failed to count hash references", countErr.Error(), nil)
		refCount = 1 // Assume referenced to be safe
	}

	// Only delete file if no other references exist
	if refCount == 0 {
		_ = fs.Remove(resource.GetCleanLocation())
	}

	ctx.Logger().Info(models.LogActionDelete, "resource", &resourceId, resource.Name, "Deleted resource", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeResource)
	return nil
}

func (ctx *MahresourcesContext) ResourceMetaKeys() ([]interfaces.MetaKey, error) {
	return metaKeys(ctx, "resources")
}

func (ctx *MahresourcesContext) BulkRemoveTagsFromResources(query *query_models.BulkEditQuery) error {
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		tags := make([]*models.Tag, 0, len(query.EditedId))
		for _, editedId := range query.EditedId {
			tag, err := ctx.GetTag(editedId)
			if err != nil {
				return err
			}
			tags = append(tags, tag)
		}

		for _, id := range query.ID {
			if deleteErr := tx.Model(&models.Resource{ID: id}).Association("Tags").Delete(tags); deleteErr != nil {
				return deleteErr
			}
		}

		return nil
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "resource", nil, "", "Bulk removed tags from resources", map[string]interface{}{
			"resourceIds": query.ID,
			"tagIds":      query.EditedId,
		})
	}

	return err
}

func (ctx *MahresourcesContext) BulkReplaceTagsFromResources(query *query_models.BulkEditQuery) error {
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		tags := make([]*models.Tag, len(query.EditedId))

		for i, editedId := range query.EditedId {
			tag, err := ctx.GetTag(editedId)

			if err != nil {
				return err
			}

			tags[i] = tag
		}

		for _, id := range query.ID {
			appendErr := tx.Model(&models.Resource{ID: id}).Association("Tags").Replace(tags)

			if appendErr != nil {
				return appendErr
			}
		}

		return nil
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "resource", nil, "", "Bulk replaced tags on resources", map[string]interface{}{
			"resourceIds": query.ID,
			"tagIds":      query.EditedId,
		})
	}

	return err
}

func (ctx *MahresourcesContext) BulkAddMetaToResources(query *query_models.BulkEditMetaQuery) error {
	var resource models.Resource

	var expr clause.Expr

	if ctx.Config.DbType == constants.DbTypePosgres {
		expr = gorm.Expr("meta || ?", query.Meta)
	} else {
		expr = gorm.Expr("json_patch(meta, ?)", query.Meta)
	}

	err := ctx.db.
		Model(&resource).
		Where("id in ?", query.ID).
		Update("Meta", expr).Error

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "resource", nil, "", "Bulk added meta to resources", map[string]interface{}{
			"resourceIds": query.ID,
		})
	}

	return err
}

func (ctx *MahresourcesContext) BulkAddTagsToResources(query *query_models.BulkEditQuery) error {
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		tags := make([]*models.Tag, 0, len(query.EditedId))
		for _, editedId := range query.EditedId {
			tag, err := ctx.GetTag(editedId)
			if err != nil {
				return err
			}
			tags = append(tags, tag)
		}

		for _, id := range query.ID {
			if appendErr := tx.Model(&models.Resource{ID: id}).Association("Tags").Append(tags); appendErr != nil {
				return appendErr
			}
		}

		return nil
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "resource", nil, "", "Bulk added tags to resources", map[string]interface{}{
			"resourceIds": query.ID,
			"tagIds":      query.EditedId,
		})
	}

	return err
}

func (ctx *MahresourcesContext) BulkAddGroupsToResources(query *query_models.BulkEditQuery) error {
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		groups := make([]*models.Group, 0, len(query.EditedId))
		for _, editedId := range query.EditedId {
			group, err := ctx.GetGroup(editedId)
			if err != nil {
				return err
			}
			groups = append(groups, group)
		}

		for _, id := range query.ID {
			if appendErr := tx.Model(&models.Resource{ID: id}).Association("Groups").Append(groups); appendErr != nil {
				return appendErr
			}
		}

		return nil
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "resource", nil, "", "Bulk added groups to resources", map[string]interface{}{
			"resourceIds": query.ID,
			"groupIds":    query.EditedId,
		})
	}

	return err
}

func (ctx *MahresourcesContext) BulkDeleteResources(query *query_models.BulkQuery) error {
	return ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		for _, id := range query.ID {
			if err := altCtx.DeleteResource(id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (ctx *MahresourcesContext) GetPopularResourceTags(query *query_models.ResourceSearchQuery) ([]PopularTag, error) {
	var res []PopularTag

	db := ctx.db.Table("resources").
		Scopes(database_scopes.ResourceQuery(query, true, ctx.db)).
		Joins("INNER JOIN resource_tags pt ON pt.resource_id = resources.id").
		Joins("INNER JOIN tags t ON t.id = pt.tag_id").
		Select("t.id AS id, t.name AS name, count(*) AS count").
		Group("t.id, t.name").
		Order("count DESC").
		Limit(20)

	return res, db.Scan(&res).Error
}

func (ctx *MahresourcesContext) MergeResources(winnerId uint, loserIds []uint) error {
	if len(loserIds) == 0 || winnerId == 0 {
		return errors.New("incorrect parameters")
	}

	for i, id := range loserIds {
		if id == 0 {
			return fmt.Errorf("loser number %v has 0 id", i+1)
		}

		if id == winnerId {
			return errors.New("winner cannot be one of the losers")
		}
	}

	return ctx.WithTransaction(func(transactionCtx *MahresourcesContext) error {
		var losers []*models.Resource

		tx := transactionCtx.db

		if loadResourcesErr := tx.Preload(clause.Associations).Find(&losers, &loserIds).Error; loadResourcesErr != nil {
			return loadResourcesErr
		}

		if winnerId == 0 || loserIds == nil || len(loserIds) == 0 {
			return nil
		}

		var winner models.Resource

		if err := tx.Preload(clause.Associations).First(&winner, winnerId).Error; err != nil {
			return err
		}

		deletedResBackups := make(map[string]types.JSON)

		for _, loser := range losers {

			for _, tag := range loser.Tags {
				if err := tx.Exec(`INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, tag.ID).Error; err != nil {
					return err
				}
			}
			for _, note := range loser.Notes {
				if err := tx.Exec(`INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, note.ID).Error; err != nil {
					return err
				}
			}
			for _, group := range loser.Groups {
				if err := tx.Exec(`INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, group.ID).Error; err != nil {
					return err
				}
			}
			if err := tx.Exec(`INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, loser.OwnerId).Error; err != nil {
				return err
			}

			backupData, err := json.Marshal(loser)

			if err != nil {
				return err
			}

			deletedResBackups[fmt.Sprintf("resource_%v", loser.ID)] = backupData


			switch transactionCtx.Config.DbType {
			case constants.DbTypePosgres:
				err = tx.Exec(`
				UPDATE resources
				SET meta = coalesce((SELECT meta FROM resources WHERE id = ?), '{}'::jsonb) || meta
				WHERE id = ?
			`, loser.ID, winnerId).Error
			case constants.DbTypeSqlite:
				err = tx.Exec(`
				UPDATE resources
				SET meta = json_patch(meta, coalesce((SELECT meta FROM resources WHERE id = ?), '{}'))
				WHERE id = ?
			`, loser.ID, winnerId).Error
			default:
				err = errors.New("db doesn't support merging meta")
			}

			if err != nil {
				return err
			}

			err = transactionCtx.DeleteResource(loser.ID)

			if err != nil {
				return err
			}
		}

		backupObj := make(map[string]any)
		backupObj["backups"] = deletedResBackups

		backups, err := json.Marshal(&backupObj)

		if err != nil {
			return err
		}

		if transactionCtx.Config.DbType == constants.DbTypePosgres {
			if err := tx.Exec("update resources set meta = meta || ? where id = ?", backups, winner.ID).Error; err != nil {
				return err
			}
		} else if transactionCtx.Config.DbType == constants.DbTypeSqlite {
			if err := tx.Exec("update resources set meta = json_patch(meta, ?) where id = ?", backups, winner.ID).Error; err != nil {
				return err
			}
		}

		transactionCtx.Logger().Info(models.LogActionUpdate, "resource", &winnerId, winner.Name, "Merged resources", map[string]interface{}{
			"winnerId": winnerId,
			"loserIds": loserIds,
		})

		return nil
	})
}
