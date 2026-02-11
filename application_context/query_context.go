package application_context

import (
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"gorm.io/gorm/clause"
	"mahresources/constants"
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

func (ctx *MahresourcesContext) GetDatabaseSchema() (map[string][]string, error) {
	schema := make(map[string][]string)

	sqlDB, err := ctx.db.DB()
	if err != nil {
		return nil, fmt.Errorf("getting underlying DB connection: %w", err)
	}

	if ctx.Config.DbType == constants.DbTypePosgres {
		rows, err := sqlDB.Query(
			`SELECT table_name, column_name FROM information_schema.columns WHERE table_schema = 'public' ORDER BY table_name, ordinal_position`,
		)
		if err != nil {
			return nil, fmt.Errorf("querying postgres schema: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var table, column string
			if err := rows.Scan(&table, &column); err != nil {
				return nil, fmt.Errorf("scanning postgres schema row: %w", err)
			}
			schema[table] = append(schema[table], column)
		}
		return schema, rows.Err()
	}

	// SQLite path
	tableRows, err := sqlDB.Query(
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying sqlite tables: %w", err)
	}
	defer tableRows.Close()

	var tables []string
	for tableRows.Next() {
		var name string
		if err := tableRows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning sqlite table name: %w", err)
		}
		tables = append(tables, name)
	}
	if err := tableRows.Err(); err != nil {
		return nil, err
	}

	for _, table := range tables {
		colRows, err := sqlDB.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, table))
		if err != nil {
			return nil, fmt.Errorf("querying columns for table %s: %w", table, err)
		}

		var columns []string
		for colRows.Next() {
			var cid int
			var name, colType string
			var notNull, pk int
			var dfltValue *string
			if err := colRows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
				colRows.Close()
				return nil, fmt.Errorf("scanning column info for table %s: %w", table, err)
			}
			columns = append(columns, name)
		}
		colRows.Close()
		if err := colRows.Err(); err != nil {
			return nil, err
		}

		schema[table] = columns
	}

	return schema, nil
}

func (ctx *MahresourcesContext) DeleteQuery(queryId uint) error {
	// Load query name before deletion for audit log
	var query models.Query
	if err := ctx.db.First(&query, queryId).Error; err != nil {
		return err
	}
	queryName := query.Name

	err := ctx.db.Select(clause.Associations).Delete(&query).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "query", &queryId, queryName, "Deleted query", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeQuery)
	}
	return err
}
