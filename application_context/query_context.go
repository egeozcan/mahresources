package application_context

import (
	"context"
	"database/sql"
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

// ErrSavedSQLScoped refuses saved SQL, and the schema listing that supports
// writing it, to a group-limited principal.
var ErrSavedSQLScoped = errors.New("saved SQL queries are not available for group-limited accounts")

// RunReadOnlyQuery runs a saved query on the read-only handle and hands its rows
// to read, closing them afterwards. An error read returns is returned as is.
//
// The handle, not this function, is what keeps the query from writing or reading
// tables outside the allow-list on SQLite (models.ReadOnlySQLiteDriverName). On
// Postgres the query runs as one prepared statement inside a READ ONLY
// transaction that has already taken its snapshot, so it can neither switch the
// transaction to read-write nor end it and write after; which tables it can read
// there is up to the role behind DB_READONLY_DSN, so under -auth it runs only
// while that role passes models.VerifyPostgresReadOnlyRole (savedSQLRefusal).
func (ctx *MahresourcesContext) RunReadOnlyQuery(queryId uint, params map[string]any, read func(*sqlx.Rows) error) error {
	// Saved queries are arbitrary SQL executed on the unscoped read-only DB, which
	// the group-subtree scope callbacks cannot constrain. There is no safe way to
	// confine arbitrary SQL to a subtree, so deny it outright to group-limited
	// principals (fail-closed). Admins, the system context, and unscoped users run
	// it as before. The MRQL saved-query path remains available to them (it is
	// force-scoped at the executor).
	if ctx.isScopedPrincipal() {
		return ErrSavedSQLScoped
	}
	if err := ctx.savedSQLRefusal(); err != nil {
		return err
	}

	var query models.Query

	if err := ctx.db.First(&query, queryId).Error; err != nil {
		return err
	}

	if strings.TrimSpace(query.Text) == "" {
		return errors.New("query text is empty")
	}

	text := strings.ReplaceAll(query.Text, "::", "::::")
	if ctx.readOnlyDB.DriverName() == "postgres" {
		return runPostgresReadOnly(ctx.readOnlyDB, text, params, read)
	}

	rows, err := ctx.readOnlyDB.NamedQuery(text, params)
	if err != nil {
		return err
	}
	defer rows.Close()
	return read(rows)
}

func runPostgresReadOnly(db *sqlx.DB, text string, params map[string]any, read func(*sqlx.Rows) error) error {
	tx, err := db.BeginTxx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	// Nothing in a read-only transaction is kept, so it always ends by rollback.
	defer func() { _ = tx.Rollback() }()

	// Postgres accepts SET TRANSACTION READ WRITE until the transaction's first
	// snapshot; take it before the saved text runs.
	if _, err := tx.Exec("SELECT 1"); err != nil {
		return err
	}
	// Prepared, so the extended protocol refuses more than one statement: no
	// "COMMIT; UPDATE ..." that writes after the transaction ends.
	stmt, err := tx.PrepareNamed(text)
	if err != nil {
		return err
	}
	defer stmt.Close()

	rows, err := stmt.Queryx(params)
	if err != nil {
		return err
	}
	defer rows.Close()
	return read(rows)
}

func (ctx *MahresourcesContext) RunReadOnlyQueryByName(queryName string, params map[string]any, read func(*sqlx.Rows) error) error {
	if ctx.isScopedPrincipal() {
		return ErrSavedSQLScoped
	}
	if err := ctx.savedSQLRefusal(); err != nil {
		return err
	}
	var query models.Query

	if err := ctx.db.Where("name = ?", queryName).First(&query).Error; err != nil {
		return err
	}

	return ctx.RunReadOnlyQuery(query.ID, params, read)
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

// GetDatabaseSchema lists tables and columns for the query editor. It lists
// only the tables saved SQL may read (models.IsSavedSQLAllowedTable), and is
// refused to group-limited principals, who cannot run saved SQL, and whenever
// running saved SQL is refused (savedSQLRefusal).
func (ctx *MahresourcesContext) GetDatabaseSchema() (map[string][]string, error) {
	if ctx.isScopedPrincipal() {
		return nil, ErrSavedSQLScoped
	}
	if err := ctx.savedSQLRefusal(); err != nil {
		return nil, err
	}
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
			if models.IsSavedSQLAllowedTable(table) {
				schema[table] = append(schema[table], column)
			}
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
		if models.IsSavedSQLAllowedTable(table) {
			schema[table] = append(schema[table], column)
		}
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
