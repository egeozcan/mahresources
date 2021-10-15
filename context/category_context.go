package context

import (
	"mahresources/database_scopes"
	"mahresources/http_query"
	"mahresources/models"
)

func (ctx *MahresourcesContext) GetCategories(offset, maxResults int, query *http_query.CategoryQuery) (*[]models.Category, error) {
	var categories []models.Category

	ctx.db.Scopes(database_scopes.CategoryQuery(query)).Limit(maxResults).Offset(int(offset)).Find(&categories)

	return &categories, nil
}

func (ctx *MahresourcesContext) GetCategoriesWithIds(ids *[]uint, limit int) (*[]models.Category, error) {
	var categories []models.Category

	if len(*ids) == 0 {
		return &categories, nil
	}

	query := ctx.db

	if limit > 0 {
		query = query.Limit(limit)
	}

	query.Find(&categories, *ids)

	return &categories, nil
}
