package models

import (
	"database/sql"
	"strings"
	"sync"

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
//   - may read only the tables IsSavedSQLAllowedTable allows, and the
//     json_each/json_tree table-valued functions.
const ReadOnlySQLiteDriverName = "sqlite3_saved_sql"

var registerReadOnlyOnce sync.Once

// sqliteRecursive is SQLITE_RECURSIVE, which go-sqlite3 does not export.
const sqliteRecursive = 33

func registerReadOnlySQLiteDriver() {
	registerReadOnlyOnce.Do(func() {
		sql.Register(ReadOnlySQLiteDriverName, &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if _, err := conn.Exec("PRAGMA busy_timeout = 10000", nil); err != nil {
					return err
				}
				if _, err := conn.Exec("PRAGMA query_only = ON", nil); err != nil {
					return err
				}
				conn.SetLimit(sqlite3.SQLITE_LIMIT_ATTACHED, 0)
				// Last: from here on the authorizer refuses PRAGMA, including the
				// two above.
				conn.RegisterAuthorizer(savedSQLAuthorizer)
				return nil
			},
		})
	})
}

// savedSQLTableFunctions are the eponymous virtual tables saved SQL may read
// from: they read their arguments, not the database.
var savedSQLTableFunctions = map[string]bool{"json_each": true, "json_tree": true}

// savedSQLAuthorizer is an allow-list; see ReadOnlySQLiteDriverName.
func savedSQLAuthorizer(action int, arg1, arg2, _ string) int {
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
		// recursive CTE counted this way arrives here as an unknown table. Such a
		// reference reads no data, so only names the table list denies and the
		// SQLite catalogue are refused; a table the server does not create at
		// all would give away its row count and nothing else.
		if arg2 == "" && !IsSavedSQLClassifiedTable(arg1) && !strings.HasPrefix(strings.ToLower(arg1), "sqlite_") {
			return sqlite3.SQLITE_OK
		}
		return sqlite3.SQLITE_DENY
	default:
		return sqlite3.SQLITE_DENY
	}
}
