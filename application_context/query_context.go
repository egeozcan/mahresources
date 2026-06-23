package application_context

import (
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"strings"
)

func (ctx *MahresourcesContext) RunReadOnlyQuery(queryId uint, params map[string]any) (*sqlx.Rows, error) {
	// Saved queries are arbitrary SQL executed on the unscoped read-only DB, which
	// the group-subtree scope callbacks cannot constrain. There is no safe way to
	// confine arbitrary SQL to a subtree, so deny it outright to group-limited
	// principals (fail-closed). Admins, the system context, and unscoped users run
	// it as before. The MRQL saved-query path remains available to them (it is
	// force-scoped at the executor).
	if ctx.isScopedPrincipal() {
		return nil, errors.New("saved SQL queries are not available for group-limited accounts")
	}

	var query models.Query

	if err := ctx.db.First(&query, queryId).Error; err != nil {
		return nil, err
	}

	if strings.TrimSpace(query.Text) == "" {
		return nil, errors.New("query text is empty")
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

	if err := ctx.db.Scopes(database_scopes.QueryQuery(searchQuery, false)).Limit(maxResults).Offset(offset).Model(&res).Find(&res).Error; err != nil {
		return nil, err
	}

	return res, nil
}

func (ctx *MahresourcesContext) GetQueriesCount(queryQ *query_models.QueryQuery) (int64, error) {
	var query models.Query
	var count int64

	return count, ctx.db.Scopes(database_scopes.QueryQuery(queryQ, true)).Model(&query).Count(&count).Error
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

	if err := ValidateEntityName(queryQuery.Name, "query"); err != nil {
		return nil, err
	}

	if strings.TrimSpace(queryQuery.Text) == "" {
		return nil, errors.New("query text must be non-empty")
	}

	query := models.Query{
		Name:        queryQuery.Name,
		Text:        queryQuery.Text,
		Template:    queryQuery.Template,
		Description: queryQuery.Description,
	}

	if err := ctx.db.Create(&query).Error; err != nil {
		return nil, friendlyUniqueError("query", err)
	}

	ctx.Logger().Info(models.LogActionCreate, "query", &query.ID, query.Name, "Created query", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeQuery)
	return &query, nil
}

func (ctx *MahresourcesContext) UpdateQuery(queryQuery *query_models.QueryEditor) (*models.Query, error) {
	var query models.Query
	if err := ctx.db.First(&query, queryQuery.ID).Error; err != nil {
		return nil, err
	}

	query.Name = queryQuery.Name
	query.Text = queryQuery.Text
	query.Template = queryQuery.Template
	query.Description = queryQuery.Description

	if strings.TrimSpace(query.Text) == "" {
		return nil, errors.New("query text must be non-empty")
	}

	if err := ctx.db.Save(&query).Error; err != nil {
		return nil, friendlyUniqueError("query", err)
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
	rows, err := sqlDB.Query(
		`SELECT m.name as table_name, p.name as column_name
		 FROM sqlite_master m
		 JOIN pragma_table_info(m.name) p
		 WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%'
		 ORDER BY m.name, p.cid`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying sqlite schema: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return nil, fmt.Errorf("scanning sqlite schema row: %w", err)
		}
		schema[table] = append(schema[table], column)
	}

	return schema, rows.Err()
}

func (ctx *MahresourcesContext) DeleteQuery(queryId uint) error {
	// Load query name before deletion for audit log
	var query models.Query
	if err := ctx.db.First(&query, queryId).Error; err != nil {
		return err
	}
	queryName := query.Name

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Select(clause.Associations).Delete(&query).Error; err != nil {
			return err
		}
		// BH-020: scrub dangling queryId references from table note_blocks
		return ScrubQueryFromBlocks(tx, queryId)
	})
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "query", &queryId, queryName, "Deleted query", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeQuery)
	}
	return err
}
