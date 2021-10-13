package context

import (
	"mahresources/database_scopes"
	"mahresources/http_query"
	"mahresources/models"
)

func (ctx *MahresourcesContext) GetCategories(offset, maxResults int, query *http_query.CategoryQuery) (*[]models.Category, error) {
	var categories []models.Category

	ctx.db.Scopes(database_scopes.CategoryQuery(query)).Limit(maxResults).Offset(int(offset)).Preload("Categories").Find(&categories)

	return &categories, nil
}
