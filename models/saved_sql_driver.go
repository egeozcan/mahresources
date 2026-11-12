package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// ReadOnlySQLiteDriverName is the database/sql driver CreateReadOnlyDatabaseConnection
// opens SQLite with: the handle saved SQL queries run on. It is a separate
// registration from SQLiteDriverName because what it installs on each connection
// would break every ordinary handle.
//
// mode=ro in the DSN is not enough on its own. It protects only the connection's
// main schema (a database ATTACHed on the same connection opens read-write and may
// be created), and go-sqlite3 drops it silently from a DSN without the file:
// prefix. So every connection on this driver, whatever its DSN:
//
//   - may attach no database (SQLITE_LIMIT_ATTACHED = 0);
//   - runs with PRAGMA query_only = ON;
//   - has an authorizer that allows only SELECT, reads, function calls,
//     recursive CTEs and the one PRAGMA FTS5 issues internally (reading
//     data_version), and denies everything else (writes, DDL, ATTACH/DETACH,
//     every other PRAGMA, transactions), so query_only cannot be switched off
//     again;
//   - runs only the first statement of the text it is given, and refuses text
//     with a second one;
//   - compiles every statement twice, once as EXPLAIN, and refuses it unless
//     every table or index b-tree its bytecode opens belongs to a table
//     IsSavedSQLAllowedTable allows (checkSavedSQLBytecode).
//
// The authorizer cannot be the only gate on which tables are read: SQLite does
// not report every column read to it. The columns a USING or NATURAL join merges
// are read from the right-hand table without an SQLITE_READ callback, and once
// any column of a table is read SQLite skips the empty-column callback it
// otherwise sends for a table in the FROM clause, so
//
//	SELECT password_hash FROM (SELECT 1 AS password_hash WHERE 0)
//	FULL JOIN users USING (password_hash)
//
// dumps users.password_hash with the authorizer never hearing of users. The
// bytecode does not have that blind spot: whatever SQLite resolved the query to,
// reading a table or index means an OpenRead of its root page.
const ReadOnlySQLiteDriverName = "sqlite3_saved_sql"

var registerReadOnlyOnce sync.Once

// sqliteRecursive is SQLITE_RECURSIVE, which go-sqlite3 does not export.
const sqliteRecursive = 33

func registerReadOnlySQLiteDriver() {
	registerReadOnlyOnce.Do(func() {
		sql.Register(ReadOnlySQLiteDriverName, &savedSQLDriver{inner: &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if _, err := conn.Exec("PRAGMA busy_timeout = 10000", nil); err != nil {
					return err
				}
				if _, err := conn.Exec("PRAGMA query_only = ON", nil); err != nil {
					return err
				}
				conn.SetLimit(sqlite3.SQLITE_LIMIT_ATTACHED, 0)
				return nil
			},
		}})
	})
}

// savedSQLDriver opens go-sqlite3 connections and wraps each in a savedSQLConn,
// whose authorizer it installs before anyone else can use the connection.
type savedSQLDriver struct {
	inner *sqlite3.SQLiteDriver
}

func (d *savedSQLDriver) Open(dsn string) (driver.Conn, error) {
	conn, err := d.inner.Open(dsn)
	if err != nil {
		return nil, err
	}
	sqliteConn, ok := conn.(*sqlite3.SQLiteConn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("saved SQL driver: unexpected connection type")
	}
	c := &savedSQLConn{conn: sqliteConn}
	// From here on the authorizer refuses PRAGMA, including the ConnectHook's.
	sqliteConn.RegisterAuthorizer(c.authorize)
	return c, nil
}

// savedSQLConn is one connection of the saved SQL handle.
//
// It deliberately implements neither driver.QueryerContext nor
// driver.ExecerContext, so database/sql prepares every statement through
// PrepareContext, where the bytecode check runs, and executes the prepared
// statement. go-sqlite3's own Query and Exec would instead run every statement
// in the text one after another.
type savedSQLConn struct {
	conn *sqlite3.SQLiteConn

	// internal is set while the connection runs the statements the bytecode
	// check needs for itself (a read transaction, PRAGMA schema_version and a
	// read of sqlite_master), which the authorizer refuses to saved SQL. Never
	// while a statement of the caller's is compiled or run.
	internal atomic.Bool

	// The root page map of the main schema (see savedSQLRootPages), and the
	// schema_version it was read at. Only the goroutine holding the connection
	// touches these.
	schemaVersion int64
	rootPages     map[int64]string
}

var (
	_ driver.Conn               = (*savedSQLConn)(nil)
	_ driver.ConnPrepareContext = (*savedSQLConn)(nil)
	_ driver.ConnBeginTx        = (*savedSQLConn)(nil)
	_ driver.Pinger             = (*savedSQLConn)(nil)
)

func (c *savedSQLConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

// PrepareContext compiles query's first statement, with the authorizer as the
// first gate, then refuses it if text follows it or if its bytecode opens a
// table outside the allow-list.
func (c *savedSQLConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	stmt, err := c.conn.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	if err := c.checkPrepared(ctx, query, stmt); err != nil {
		_ = stmt.Close()
		return nil, err
	}
	return stmt, nil
}

func (c *savedSQLConn) Close() error { return c.conn.Close() }

func (c *savedSQLConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

// BeginTx is go-sqlite3's, whose BEGIN the authorizer refuses.
func (c *savedSQLConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.conn.BeginTx(ctx, opts)
}

func (c *savedSQLConn) Ping(ctx context.Context) error { return c.conn.Ping(ctx) }

// savedSQLTableFunctions are the eponymous virtual tables saved SQL may read
// from: they read their arguments, not the database.
var savedSQLTableFunctions = map[string]bool{"json_each": true, "json_tree": true}

// authorize is the connection's SQLite authorizer: an allow-list, see
// ReadOnlySQLiteDriverName. It is the first of two gates on what saved SQL
// reads; checkSavedSQLBytecode is the second, and the one that does not depend
// on SQLite reporting every column.
func (c *savedSQLConn) authorize(action int, arg1, arg2, dbName string) int {
	if c.internal.Load() {
		switch action {
		case sqlite3.SQLITE_TRANSACTION:
			return sqlite3.SQLITE_OK
		case sqlite3.SQLITE_PRAGMA:
			if strings.EqualFold(arg1, "schema_version") && arg2 == "" {
				return sqlite3.SQLITE_OK
			}
		case sqlite3.SQLITE_READ:
			if strings.EqualFold(arg1, "sqlite_master") && dbName == "main" {
				return sqlite3.SQLITE_OK
			}
		}
	}
	return savedSQLAuthorizer(action, arg1, arg2)
}

func savedSQLAuthorizer(action int, arg1, arg2 string) int {
	switch action {
	case sqlite3.SQLITE_PRAGMA:
		// FTS5 reads PRAGMA main.data_version from inside every MATCH query to
		// tell whether its cached index structure is stale. Reading it is
		// harmless; setting a value, or any other pragma, is not allowed.
		if strings.EqualFold(arg1, "data_version") && arg2 == "" {
			return sqlite3.SQLITE_OK
		}
		return sqlite3.SQLITE_DENY
	case sqlite3.SQLITE_SELECT, sqlite3.SQLITE_FUNCTION, sqliteRecursive:
		return sqlite3.SQLITE_OK
	case sqlite3.SQLITE_READ:
		if IsSavedSQLAllowedTable(arg1) || savedSQLTableFunctions[strings.ToLower(arg1)] {
			return sqlite3.SQLITE_OK
		}
		// An empty column is SQLite reporting a FROM item none of whose columns
		// the statement reads (SELECT count(*) FROM x). Its name is whatever the
		// FROM clause says, which for a CTE is the CTE's own name, so a
		// recursive CTE counted this way arrives here as an unknown table. Only
		// names the table list denies and the SQLite catalogue are refused here;
		// a real table the list does not mention is refused by the bytecode
		// check, which sees the b-tree a count or a scan of it opens.
		if arg2 == "" && !IsSavedSQLClassifiedTable(arg1) && !strings.HasPrefix(strings.ToLower(arg1), "sqlite_") {
			return sqlite3.SQLITE_OK
		}
		return sqlite3.SQLITE_DENY
	default:
		return sqlite3.SQLITE_DENY
	}
}
