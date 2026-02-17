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

	"github.com/spf13/afero"
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
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(
			"DELETE FROM resource_tags WHERE resource_id IN ? AND tag_id IN ?",
			query.ID, query.EditedId,
		).Error
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
	if len(query.ID) == 0 {
		return nil
	}

	uniqueEditedIds := deduplicateUints(query.EditedId)

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		// Validate all tags exist
		if len(uniqueEditedIds) > 0 {
			var tagCount int64
			if err := tx.Model(&models.Tag{}).Where("id IN ?", uniqueEditedIds).Count(&tagCount).Error; err != nil {
				return err
			}
			if int(tagCount) != len(uniqueEditedIds) {
				return fmt.Errorf("one or more tags not found")
			}
		}

		// Remove all existing tags for these resources
		if err := tx.Exec("DELETE FROM resource_tags WHERE resource_id IN ?", query.ID).Error; err != nil {
			return err
		}

		// Add the new tags
		for _, tagID := range uniqueEditedIds {
			if err := tx.Exec(
				"INSERT INTO resource_tags (resource_id, tag_id) SELECT id, ? FROM resources WHERE id IN ? ON CONFLICT DO NOTHING",
				tagID, query.ID,
			).Error; err != nil {
				return err
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
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	uniqueEditedIds := deduplicateUints(query.EditedId)

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		// Validate all tags exist (single query, no preloads)
		var tagCount int64
		if err := tx.Model(&models.Tag{}).Where("id IN ?", uniqueEditedIds).Count(&tagCount).Error; err != nil {
			return err
		}
		if int(tagCount) != len(uniqueEditedIds) {
			return fmt.Errorf("one or more tags not found")
		}

		// Batch insert: one INSERT per tag, skip conflicts
		for _, tagID := range uniqueEditedIds {
			if err := tx.Exec(
				"INSERT INTO resource_tags (resource_id, tag_id) SELECT id, ? FROM resources WHERE id IN ? ON CONFLICT DO NOTHING",
				tagID, query.ID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionUpdate, "resource", nil, "", "Bulk added tags to resources", map[string]interface{}{
			"resourceIds": query.ID,
			"tagIds":      uniqueEditedIds,
		})
	}

	return err
}

func (ctx *MahresourcesContext) BulkAddGroupsToResources(query *query_models.BulkEditQuery) error {
	if len(query.ID) == 0 || len(query.EditedId) == 0 {
		return nil
	}

	uniqueEditedIds := deduplicateUints(query.EditedId)

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		var groupCount int64
		if err := tx.Model(&models.Group{}).Where("id IN ?", uniqueEditedIds).Count(&groupCount).Error; err != nil {
			return err
		}
		if int(groupCount) != len(uniqueEditedIds) {
			return fmt.Errorf("one or more groups not found")
		}

		for _, groupID := range uniqueEditedIds {
			if err := tx.Exec(
				"INSERT INTO groups_related_resources (resource_id, group_id) SELECT id, ? FROM resources WHERE id IN ? ON CONFLICT DO NOTHING",
				groupID, query.ID,
			).Error; err != nil {
				return err
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

// FileCleanupAction describes a file operation to perform after a transaction commits.
type FileCleanupAction struct {
	// SourceFS is the filesystem containing the resource file
	SourceFS afero.Fs
	// SourcePath is the path to the original resource file
	SourcePath string
	// BackupPath is the path to write the backup copy (in /deleted/)
	BackupPath string
	// ShouldRemoveSource indicates if the source file should be deleted (no other references)
	ShouldRemoveSource bool
}

// deleteResourceDBOnly performs only the database operations of DeleteResource.
// Returns file cleanup actions to be performed after the transaction commits.
func (ctx *MahresourcesContext) deleteResourceDBOnly(resourceId uint) (*FileCleanupAction, error) {
	resource := models.Resource{ID: resourceId}
	if err := ctx.db.Model(&resource).First(&resource).Error; err != nil {
		return nil, err
	}

	fs, storageErr := ctx.GetFsForStorageLocation(resource.StorageLocation)
	if storageErr != nil {
		return nil, storageErr
	}

	subFolder := "deleted"
	if resource.StorageLocation != nil && *resource.StorageLocation != "" {
		subFolder = *resource.StorageLocation
	}
	folder := fmt.Sprintf("/deleted/%v/", subFolder)

	ownerIdStr := "nil"
	if resource.OwnerId != nil {
		ownerIdStr = fmt.Sprintf("%v", *resource.OwnerId)
	}
	backupPath := path.Join(folder, fmt.Sprintf("%v__%v__%v___%v", resource.Hash, resource.ID, ownerIdStr, strings.ReplaceAll(path.Clean(path.Base(resource.GetCleanLocation())), "\\", "_")))

	// Clear CurrentVersionID to break circular reference before deletion
	if resource.CurrentVersionID != nil {
		if err := ctx.db.Model(&resource).Update("current_version_id", nil).Error; err != nil {
			return nil, err
		}
	}

	if err := ctx.db.Select(clause.Associations).Delete(&resource).Error; err != nil {
		return nil, err
	}

	// Auto-delete empty series
	if resource.SeriesID != nil {
		seriesID := *resource.SeriesID
		result := ctx.db.Where("id = ? AND NOT EXISTS (SELECT 1 FROM resources WHERE series_id = ?)", seriesID, seriesID).Delete(&models.Series{})
		if result.Error != nil {
			ctx.Logger().Warning(models.LogActionDelete, "series", &seriesID, "Failed to auto-delete empty series", result.Error.Error(), nil)
		}
	}

	// Check hash references for file deletion decision
	refCount, countErr := ctx.CountHashReferences(resource.Hash)
	if countErr != nil {
		ctx.Logger().Warning(models.LogActionDelete, "resource", &resourceId, "Failed to count hash references", countErr.Error(), nil)
		refCount = 1 // Assume referenced to be safe
	}

	ctx.Logger().Info(models.LogActionDelete, "resource", &resourceId, resource.Name, "Deleted resource", nil)
	ctx.InvalidateSearchCacheByType(EntityTypeResource)

	return &FileCleanupAction{
		SourceFS:           fs,
		SourcePath:         resource.GetCleanLocation(),
		BackupPath:         backupPath,
		ShouldRemoveSource: refCount == 0,
	}, nil
}

func (ctx *MahresourcesContext) BulkDeleteResources(query *query_models.BulkQuery) error {
	var cleanupActions []*FileCleanupAction

	err := ctx.WithTransaction(func(altCtx *MahresourcesContext) error {
		for _, id := range query.ID {
			action, err := altCtx.deleteResourceDBOnly(id)
			if err != nil {
				return err
			}
			if action != nil {
				cleanupActions = append(cleanupActions, action)
			}
		}
		return nil
	})

	if err != nil {
		return err // Transaction rolled back, no file operations performed
	}

	// Phase 2: File operations after successful commit
	for _, action := range cleanupActions {
		// Create backup
		if err := ctx.fs.MkdirAll(path.Dir(action.BackupPath), 0777); err != nil {
			ctx.Logger().Warning(models.LogActionDelete, "resource", nil, "Failed to create backup dir", err.Error(), nil)
			continue
		}

		backupOK := false
		file, openErr := action.SourceFS.Open(action.SourcePath)
		if openErr == nil {
			backup, createErr := ctx.fs.Create(action.BackupPath)
			if createErr == nil {
				_, copyErr := io.Copy(backup, file)
				backup.Close()
				backupOK = copyErr == nil
			}
			file.Close()
		}

		// Only remove source file if backup succeeded (or source couldn't be opened, meaning it's already gone)
		if action.ShouldRemoveSource && (backupOK || openErr != nil) {
			_ = action.SourceFS.Remove(action.SourcePath)
		}
	}

	return nil
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
		tx := transactionCtx.db

		// Load losers WITHOUT associations — we only need their basic fields for backup
		var losers []*models.Resource
		if loadResourcesErr := tx.Find(&losers, &loserIds).Error; loadResourcesErr != nil {
			return loadResourcesErr
		}

		// Load winner WITHOUT associations
		var winner models.Resource
		if err := tx.First(&winner, winnerId).Error; err != nil {
			return err
		}

		// Transfer associations via direct SQL (no Go-side loading needed)
		if err := tx.Exec("INSERT INTO resource_tags (resource_id, tag_id) SELECT ?, tag_id FROM resource_tags WHERE resource_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO resource_notes (resource_id, note_id) SELECT ?, note_id FROM resource_notes WHERE resource_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO groups_related_resources (resource_id, group_id) SELECT ?, group_id FROM groups_related_resources WHERE resource_id IN ? ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
			return err
		}
		// Also add losers' owners as related groups
		if err := tx.Exec("INSERT INTO groups_related_resources (resource_id, group_id) SELECT ?, owner_id FROM resources WHERE id IN ? AND owner_id IS NOT NULL ON CONFLICT DO NOTHING", winnerId, loserIds).Error; err != nil {
			return err
		}

		deletedResBackups := make(map[string]types.JSON)

		for _, loser := range losers {
			backupData, err := json.Marshal(loser)
			if err != nil {
				return err
			}
			deletedResBackups[fmt.Sprintf("resource_%v", loser.ID)] = backupData

			// Merge meta
			switch transactionCtx.Config.DbType {
			case constants.DbTypePosgres:
				err = tx.Exec(`UPDATE resources SET meta = coalesce((SELECT meta FROM resources WHERE id = ?), '{}'::jsonb) || meta WHERE id = ?`, loser.ID, winnerId).Error
			case constants.DbTypeSqlite:
				err = tx.Exec(`UPDATE resources SET meta = json_patch(meta, coalesce((SELECT meta FROM resources WHERE id = ?), '{}')) WHERE id = ?`, loser.ID, winnerId).Error
			default:
				err = errors.New("db doesn't support merging meta")
			}
			if err != nil {
				return err
			}

			if err := transactionCtx.DeleteResource(loser.ID); err != nil {
				return err
			}
		}

		// Save backups to winner's meta
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
