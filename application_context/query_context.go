package application_context

import (
	"errors"
	"github.com/jmoiron/sqlx"
	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"strings"
)

func (ctx *MahresourcesContext) RunReadOnlyQuery(queryId uint, params map[string]any) (*sqlx.Rows, error) {
	var query models.Query

	if err := ctx.db.First(&query, queryId).Error; err != nil {
		return nil, err
	}

	return ctx.readOnlyDB.NamedQuery(strings.ReplaceAll(query.Text, "::", "::::"), params)
}

func (ctx *MahresourcesContext) RunReadOnlyQueryByName(queryName string, params map[string]any) (*sqlx.Rows, error) {
	var query models.Query

	if err := ctx.db.Where("name = ?", queryName).First(&query).Error; err != nil {
		return nil, err
	}

	return ctx.RunReadOnlyQuery(query.ID, params)
}

func (ctx *MahresourcesContext) GetQueries(offset, maxResults int, searchQuery *query_models.QueryQuery) ([]models.Query, error) {
	var res []models.Query

	if err := ctx.db.Scopes(database_scopes.QueryQuery(searchQuery)).Limit(maxResults).Offset(offset).Model(&res).Find(&res).Error; err != nil {
		return nil, err
	}

	return res, nil
}

func (ctx *MahresourcesContext) GetQueriesCount(queryQ *query_models.QueryQuery) (int64, error) {
	var query models.Query
	var count int64

	return count, ctx.db.Scopes(database_scopes.QueryQuery(queryQ)).Model(&query).Count(&count).Error
}

func (ctx *MahresourcesContext) GetQuery(id uint) (*models.Query, error) {
	var query models.Query

	err := ctx.db.
		First(&query, id).Error

	return &query, err
}

func (ctx *MahresourcesContext) CreateQuery(queryQuery *query_models.QueryCreator) (*models.Query, error) {
	if strings.TrimSpace(queryQuery.Name) == "" {
		return nil, errors.New("query name must be non-empty")
	}

	query := models.Query{
		Name:     queryQuery.Name,
		Text:     queryQuery.Text,
		Template: queryQuery.Template,
	}

	if err := ctx.db.Create(&query).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionCreate, "query", &query.ID, query.Name, "Created query", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeQuery)
	return &query, nil
}

func (ctx *MahresourcesContext) UpdateQuery(queryQuery *query_models.QueryEditor) (*models.Query, error) {
	if strings.TrimSpace(queryQuery.Name) == "" {
		return nil, errors.New("query name must be non-empty")
	}

	query := models.Query{
		ID:       queryQuery.ID,
		Name:     queryQuery.Name,
		Text:     queryQuery.Text,
		Template: queryQuery.Template,
	}

	if err := ctx.db.Save(&query).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionUpdate, "query", &query.ID, query.Name, "Updated query", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeQuery)
	return &query, nil
}

func (ctx *MahresourcesContext) DeleteQuery(queryId uint) error {
	query := models.Query{ID: queryId}

	err := ctx.db.Select(clause.Associations).Delete(&query).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "query", &queryId, "", "Deleted query", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeQuery)
	}
	return err
}
