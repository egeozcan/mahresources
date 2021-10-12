package context

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/api_model"
	"mahresources/database_scopes"
	"mahresources/http_query"
	"mahresources/models"
)

func (ctx *MahresourcesContext) CreateGroup(groupQuery *http_query.GroupCreator) (*models.Group, error) {
	if groupQuery.Name == "" {
		return nil, errors.New("group name needed")
	}

	group := models.Group{
		Name:        groupQuery.Name,
		Description: groupQuery.Description,
	}
	ctx.db.Create(&group)

	if len(groupQuery.Tags) > 0 {
		tags := make([]models.Tag, len(groupQuery.Tags))
		for i, v := range groupQuery.Tags {
			tags[i] = models.Tag{
				Model: gorm.Model{ID: v},
			}
		}
		createTagsErr := ctx.db.Model(&group).Association("Tags").Append(&tags)

		if createTagsErr != nil {
			return nil, createTagsErr
		}
	}

	return &group, nil
}
func (ctx *MahresourcesContext) UpdateGroup(groupQuery *http_query.GroupEditor) (*models.Group, error) {
	if groupQuery.Name == "" {
		return nil, errors.New("group name needed")
	}

	tags := make([]*models.Tag, len(groupQuery.Tags))

	for i, tag := range groupQuery.Tags {
		tags[i] = &models.Tag{
			Model: gorm.Model{
				ID: tag,
			},
		}
	}

	group := models.Group{
		Model: gorm.Model{
			ID: groupQuery.ID,
		},
		Name:        groupQuery.Name,
		Description: groupQuery.Description,
	}

	err := ctx.db.Model(&group).Association("Tags").Clear()

	if err != nil {
		return nil, err
	}

	group.Tags = tags

	ctx.db.Save(&group)

	return &group, nil
}

func (ctx *MahresourcesContext) GetGroup(id uint) (*models.Group, error) {
	var group models.Group
	ctx.db.Preload(clause.Associations).First(&group, id)

	if group.ID == 0 {
		return nil, errors.New("could not load group")
	}

	return &group, nil
}

func (ctx *MahresourcesContext) GetGroups(offset, maxResults int, query *http_query.GroupQuery) (*[]models.Group, error) {
	var groups []models.Group

	ctx.db.Scopes(database_scopes.GroupQuery(query)).Limit(maxResults).Offset(int(offset)).Preload("Tags").Find(&groups)

	return &groups, nil
}

func (ctx *MahresourcesContext) GetGroupsAutoComplete(name string, maxResults int) (*[]api_model.AutoCompleteResult, error) {
	var groups []models.Group

	ctx.db.Where("name LIKE ?", "%"+name+"%").Limit(maxResults).Find(&groups)

	results := make([]api_model.AutoCompleteResult, len(groups))

	for i, v := range groups {
		results[i] = api_model.AutoCompleteResult{
			Name: v.Name,
			ID:   v.ID,
		}
	}

	return &results, nil
}

func (ctx *MahresourcesContext) GetGroupsWithIds(ids []uint) (*[]*models.Group, error) {
	var groups []*models.Group

	ctx.db.Find(&groups, ids)

	return &groups, nil
}

func (ctx *MahresourcesContext) GetGroupsCount(query *http_query.GroupQuery) (int64, error) {
	var group models.Group
	var count int64
	ctx.db.Scopes(database_scopes.GroupQuery(query)).Model(&group).Count(&count)

	return count, nil
}

func (ctx *MahresourcesContext) GetTagsForGroups() (*[]models.Tag, error) {
	var tags []models.Tag
	ctx.db.Raw(`SELECT
					  Count(*)
					  , id
					  , name
					from tags t
					join group_tags pt on t.id = pt.tag_id
					group by t.name, t.id
					order by count(*) desc
	`).Scan(&tags)

	return &tags, nil
}