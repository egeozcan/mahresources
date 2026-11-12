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
//   - has an authorizer that allows only SELECT, reads, function calls and
//     recursive CTEs, and denies everything else (writes, DDL, ATTACH/DETACH,
//     PRAGMA, transactions), so query_only cannot be switched off again;
//   - may not read the tables IsSavedSQLDeniedTable names.
const ReadOnlySQLiteDriverName = "sqlite3_saved_sql"

var registerReadOnlyOnce sync.Once

// sqliteRecursive is SQLITE_RECURSIVE, which go-sqlite3 does not export.
const sqliteRecursive = 33

// savedSQLDeniedTables holds credentials, secrets, the audit log and other
// accounts' private data: what the role model keeps from editors and plain users.
// Saved SQL runs as any unscoped principal and is authored by editors, so it must
// not read them.
var savedSQLDeniedTables = map[string]bool{
	"users":                true, // password hashes, roles
	"sessions":             true, // session token hashes, CSRF tokens
	"api_tokens":           true, // API token hashes
	"plugin_states":        true, // plugin settings, including secrets in plaintext
	"plugin_kvs":           true, // plugin key-value store
	"runtime_settings":     true, // admin-only system settings
	"log_entries":          true, // admin-only audit log
	"user_settings":        true, // another account's preferences
	"saved_searches":       true, // another account's saved searches
	"job_replay_envelopes": true, // sealed job inputs
}

// IsSavedSQLDeniedTable reports whether saved SQL may not read table, and whether
// the schema listing offered to query authors leaves it out.
func IsSavedSQLDeniedTable(table string) bool {
	return savedSQLDeniedTables[strings.ToLower(table)]
}

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

// savedSQLAuthorizer is an allow-list; see ReadOnlySQLiteDriverName.
func savedSQLAuthorizer(action int, arg1, _, _ string) int {
	switch action {
	case sqlite3.SQLITE_SELECT, sqlite3.SQLITE_FUNCTION, sqliteRecursive:
		return sqlite3.SQLITE_OK
	case sqlite3.SQLITE_READ:
		if IsSavedSQLDeniedTable(arg1) {
			return sqlite3.SQLITE_DENY
		}
		return sqlite3.SQLITE_OK
	default:
		return sqlite3.SQLITE_DENY
	}
}
