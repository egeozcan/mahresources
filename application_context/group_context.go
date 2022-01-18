package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"net/url"
)

func (ctx *MahresourcesContext) CreateGroup(groupQuery *query_models.GroupCreator) (*models.Group, error) {
	if groupQuery.Name == "" {
		return nil, errors.New("group name needed")
	}

	if groupQuery.Meta == "" {
		groupQuery.Meta = "{}"
	}

	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	parsedURL, err := url.Parse(groupQuery.URL)

	if groupQuery.URL != "" && err != nil {
		return nil, err
	}

	groupUrl := (*types.URL)(parsedURL)

	group := models.Group{
		Name:        groupQuery.Name,
		Description: groupQuery.Description,
		CategoryId:  &groupQuery.CategoryId,
		Meta:        []byte(groupQuery.Meta),
		URL:         groupUrl,
	}

	if groupQuery.OwnerId != 0 {
		group.OwnerId = &groupQuery.OwnerId
	}

	if err := tx.Create(&group).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if len(groupQuery.Tags) > 0 {
		tags := make([]models.Tag, len(groupQuery.Tags))
		for i, v := range groupQuery.Tags {
			tags[i] = models.Tag{
				ID: v,
			}
		}

		if createTagsErr := tx.Model(&group).Association("Tags").Append(&tags); createTagsErr != nil {
			tx.Rollback()
			return nil, createTagsErr
		}
	}

	if len(groupQuery.Groups) > 0 {
		groups := make([]models.Group, len(groupQuery.Groups))
		for i, v := range groupQuery.Groups {
			groups[i] = models.Group{
				ID: v,
			}
		}

		if createGroupsErr := tx.Model(&group).Association("RelatedGroups").Append(&groups); createGroupsErr != nil {
			tx.Rollback()
			return nil, createGroupsErr
		}
	}

	return &group, tx.Commit().Error
}

func (ctx *MahresourcesContext) UpdateGroup(groupQuery *query_models.GroupEditor) (*models.Group, error) {
	if groupQuery.Name == "" {
		return nil, errors.New("group name needed")
	}

	tx := ctx.db.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	groups := make([]*models.Group, len(groupQuery.Groups))

	for i, group := range groupQuery.Groups {
		groups[i] = &models.Group{
			ID: group,
		}
	}

	tags := make([]*models.Tag, len(groupQuery.Tags))

	for i, tag := range groupQuery.Tags {
		tags[i] = &models.Tag{
			ID: tag,
		}
	}

	if groupQuery.Meta == "" {
		groupQuery.Meta = "{}"
	}

	group := &models.Group{
		ID:          groupQuery.ID,
		Name:        groupQuery.Name,
		Description: groupQuery.Description,
		Meta:        []byte(groupQuery.Meta),
	}

	if groupQuery.URL != "" {
		parsedURL, err := url.Parse(groupQuery.URL)

		if groupQuery.URL != "" && err != nil {
			tx.Rollback()
			return nil, err
		}

		groupUrl := (*types.URL)(parsedURL)
		group.URL = groupUrl
	} else {
		group.URL = nil
		if err := tx.Model(&group).Update("url", nil).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if groupQuery.OwnerId != 0 {
		group.OwnerId = &groupQuery.OwnerId
	} else if err := tx.Model(group).Association("Owner").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(group).Updates(group).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(group).Association("Tags").Replace(tags); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(group).Association("RelatedGroups").Replace(groups); err != nil {
		tx.Rollback()
		return nil, err
	}

	return group, tx.Commit().Error
}

func (ctx *MahresourcesContext) GetGroup(id uint) (*models.Group, error) {
	var group models.Group

	err := ctx.db.
		Preload("OwnGroups", pageLimit).
		Preload("OwnGroups.Category", pageLimit).
		Preload("OwnResources", pageLimitCustom(5)).
		Preload("OwnNotes", pageLimit).
		Preload("RelatedResources", pageLimitCustom(5)).
		Preload("RelatedNotes", pageLimit).
		Preload("RelatedGroups", pageLimit).
		Preload("Tags").
		Preload("Owner").
		Preload("Owner.Category").
		Preload("Category", pageLimit).
		Preload("Relationships").
		Preload("Relationships.ToGroup").
		Preload("Relationships.RelationType").
		Preload("BackRelations").
		Preload("BackRelations.FromGroup").
		Preload("BackRelations.RelationType").
		First(&group, id).Error

	return &group, err
}

func (ctx *MahresourcesContext) GetGroups(offset, maxResults int, query *query_models.GroupQuery) (*[]models.Group, error) {
	var groups []models.Group
	groupScope := database_scopes.GroupQuery(query, false, ctx.db)

	return &groups, ctx.db.Scopes(groupScope).Limit(maxResults).
		Offset(offset).Preload("Tags").Preload("Category").Find(&groups).Error
}

func (ctx *MahresourcesContext) GetGroupsWithIds(ids *[]uint) (*[]*models.Group, error) {
	var groups []*models.Group

	if len(*ids) == 0 {
		return &groups, nil
	}

	return &groups, ctx.db.Preload("Category").Find(&groups, ids).Error
}

func (ctx *MahresourcesContext) GetGroupsCount(query *query_models.GroupQuery) (int64, error) {
	var group models.Group
	var count int64

	return count, ctx.db.Scopes(database_scopes.GroupQuery(query, true, ctx.db)).Model(&group).Count(&count).Error
}

func (ctx *MahresourcesContext) DeleteGroup(groupId uint) error {
	group := models.Group{ID: groupId}

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		ctx.EnsureForeignKeysActive(tx)

		return tx.
			Select("OwnGroups").
			Select("OwnNotes").
			Select("RelatedResources").
			Select("RelatedNotes").
			Select("RelatedGroups").
			Select("Relationships").
			Select("BackRelations").
			Select("Tags").
			Delete(&group).Error
	})
}

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
		var losers []*models.Group

		if loadErr := altCtx.db.Preload(clause.Associations).Find(&losers, &loserIds).Error; loadErr != nil {
			return loadErr
		}

		var winner models.Group

		if err := altCtx.db.Preload(clause.Associations).First(&winner, winnerId).Error; err != nil {
			return err
		}

		backups := make(map[string]types.JSON)

		for _, loser := range losers {
			if winner.OwnerId != nil && loser.ID == *winner.OwnerId {
				if err := altCtx.db.Exec(`UPDATE groups set owner_id = NULL where id = ?`, winnerId).Error; err != nil {
					return err
				}
			}

			for _, tag := range loser.Tags {
				if err := altCtx.db.Exec(`INSERT INTO group_tags (group_id, tag_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, tag.ID).Error; err != nil {
					return err
				}
			}

			if err := altCtx.db.Exec(`UPDATE groups SET owner_id = ? WHERE owner_id = ?`, winnerId, loser.ID).Error; err != nil {
				return err
			}

			for _, group := range loser.RelatedGroups {
				if group.ID == winnerId {
					continue
				}
				if err := altCtx.db.Exec(`INSERT INTO group_related_groups (group_id, related_group_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, group.ID).Error; err != nil {
					return err
				}
			}

			if err := altCtx.db.Exec(`UPDATE notes SET owner_id = ? WHERE owner_id = ?`, winnerId, loser.ID).Error; err != nil {
				return err
			}

			for _, note := range loser.RelatedNotes {
				if err := altCtx.db.Exec(`INSERT INTO groups_related_notes (group_id, note_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, note.ID).Error; err != nil {
					return err
				}
			}

			if err := altCtx.db.Exec(`UPDATE resources SET owner_id = ? WHERE owner_id = ?`, winnerId, loser.ID).Error; err != nil {
				return err
			}

			for _, resource := range loser.RelatedResources {
				if err := altCtx.db.Exec(`INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, winnerId, resource.ID).Error; err != nil {
					return err
				}
			}

			if err := altCtx.db.Exec(`UPDATE group_relations SET from_group_id = ? WHERE from_group_id = ? AND to_group_id <> ? ON CONFLICT DO NOTHING`, winnerId, loser.ID, winnerId).Error; err != nil {
				return err
			}

			if err := altCtx.db.Exec(`UPDATE group_relations SET to_group_id = ? WHERE to_group_id = ? AND to_group_id <> ? ON CONFLICT DO NOTHING`, winnerId, loser.ID, winnerId).Error; err != nil {
				return err
			}

			backupData, err := json.Marshal(loser)

			if err != nil {
				return err
			}

			backups[fmt.Sprintf("resource_%v", loser.ID)] = backupData
			fmt.Printf("%#v\n", backups)

			switch altCtx.Config.DbType {
			case constants.DbTypePosgres:
				err = altCtx.db.Exec(`
				UPDATE groups
				SET meta = coalesce((SELECT meta FROM groups WHERE id = ?), '{}'::jsonb) || meta
				WHERE id = ?
			`, loser.ID, winnerId).Error
			case constants.DbTypeSqlite:
				err = altCtx.db.Exec(`
				UPDATE groups
				SET meta = json_patch(meta, coalesce((SELECT meta FROM groups WHERE id = ?), '{}'))
				WHERE id = ?
			`, loser.ID, winnerId).Error
			default:
				err = errors.New("db doesn't support merging meta")
			}

			if err != nil {
				return err
			}

			err = altCtx.DeleteGroup(loser.ID)

			if err != nil {
				return err
			}
		}

		fmt.Printf("%#v\n", backups)

		backupObj := make(map[string]interface{})
		backupObj["backups"] = backups

		backupsBytes, err := json.Marshal(&backupObj)

		if err != nil {
			return err
		}

		fmt.Println(string(backupsBytes))

		if ctx.Config.DbType == constants.DbTypePosgres {
			if err := altCtx.db.Exec("update resources set meta = meta || ? where id = ?", backupsBytes, winner.ID).Error; err != nil {
				return err
			}
		}

		if err := altCtx.db.Exec(`DELETE FROM group_relations WHERE to_group_id = from_group_id`).Error; err != nil {
			return err
		}

		return nil
	})
}

func (ctx *MahresourcesContext) GroupMetaKeys() (*[]fieldResult, error) {
	return metaKeys(ctx, "groups")
}

func (ctx *MahresourcesContext) BulkAddTagsToGroups(query *query_models.BulkEditQuery) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		for _, editedId := range query.EditedId {
			tag, err := ctx.GetTag(editedId)

			if err != nil {
				return err
			}

			for _, groupId := range query.ID {
				appendErr := tx.Model(&models.Group{ID: groupId}).Association("Tags").Append(tag)

				if appendErr != nil {
					return appendErr
				}
			}
		}

		return nil
	})
}

func (ctx *MahresourcesContext) BulkRemoveTagsFromGroups(query *query_models.BulkEditQuery) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		for _, editedId := range query.EditedId {
			tag, err := ctx.GetTag(editedId)

			if err != nil {
				return err
			}

			for _, groupId := range query.ID {
				appendErr := tx.Model(&models.Group{ID: groupId}).Association("Tags").Delete(tag)

				if appendErr != nil {
					return appendErr
				}
			}
		}

		return nil
	})
}

func (ctx *MahresourcesContext) BulkAddMetaToGroups(query *query_models.BulkEditMetaQuery) error {
	var group models.Group

	return ctx.db.
		Model(&group).
		Where("id in ?", query.ID).
		Update("Meta", gorm.Expr("Meta || ?", query.Meta)).Error
}

func (ctx *MahresourcesContext) BulkDeleteGroups(query *query_models.BulkQuery) error {
	for _, id := range query.ID {
		if err := ctx.DeleteGroup(id); err != nil {
			return err
		}
	}

	return nil
}
