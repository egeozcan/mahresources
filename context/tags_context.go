package context

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/database_scopes"
	"mahresources/http_query"
	"mahresources/models"
)

func (ctx *MahresourcesContext) GetTags(offset, maxResults int, query *http_query.TagQuery) (*[]models.Tag, error) {
	var tags []models.Tag

	ctx.db.Scopes(database_scopes.TagQuery(query)).Limit(maxResults).Offset(offset).Find(&tags)

	return &tags, nil
}

func (ctx *MahresourcesContext) GetTagsCount(query *http_query.TagQuery) (int64, error) {
	var tag models.Tag
	var count int64
	ctx.db.Scopes(database_scopes.TagQuery(query)).Model(&tag).Count(&count)

	return count, nil
}

func (ctx *MahresourcesContext) GetTagsByName(name string, limit int) (*[]*models.Tag, error) {
	var tags []*models.Tag

	var query *gorm.DB

	if name != "" {
		query = ctx.db.Where("name like ?", "%"+name+"%").Order("name")
	} else {
		query = ctx.db.Order("name")
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	query.Find(&tags)

	return &tags, nil
}

func (ctx *MahresourcesContext) GetTag(id uint) (*models.Tag, error) {
	var tag models.Tag
	ctx.db.Preload(clause.Associations).First(&tag, id)

	if tag.ID == 0 {
		return nil, errors.New("could not load tag")
	}

	return &tag, nil
}

func (ctx *MahresourcesContext) GetTagsWithIds(ids *[]uint, limit int) (*[]models.Tag, error) {
	var tags []models.Tag

	query := ctx.db

	if limit > 0 {
		query = query.Limit(limit)
	}

	query.Find(&tags, *ids)

	fmt.Println("tags", tags)

	return &tags, nil
}

func (ctx *MahresourcesContext) CreateTag(tagQuery *http_query.TagCreator) (*models.Tag, error) {
	tag := models.Tag{
		Name: tagQuery.Name,
	}
	ctx.db.Create(&tag)

	return &tag, nil
}

func (ctx *MahresourcesContext) UpdateTag(tagQuery *http_query.TagCreator) (*models.Tag, error) {

	tag := models.Tag{
		Model: gorm.Model{
			ID: tagQuery.ID,
		},
		Name: tagQuery.Name,
	}
	ctx.db.Save(&tag)

	return &tag, nil
}
