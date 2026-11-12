package models

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// ErrSavedSQLMultipleStatements refuses saved SQL text with a second statement
// after the first. Only the first would run.
var ErrSavedSQLMultipleStatements = errors.New("not authorized: saved SQL must be a single statement")

// opflagP2IsReg is SQLite's OPFLAG_P2ISREG: the OpenRead's root page is in
// register P2 instead of being P2 itself, so the bytecode does not say which
// b-tree it opens. SQLite emits it only for schema work, never for a SELECT.
const opflagP2IsReg = 0x10

// savedSQLRefusedOpcodes write, change the schema or open a b-tree for
// writing. The authorizer and query_only already refuse every statement that
// would contain one; finding one in a saved query's bytecode is refused too, so
// the check does not depend on that.
var savedSQLRefusedOpcodes = map[string]bool{
	"OpenWrite": true, "SetCookie": true, "CreateBtree": true, "Destroy": true,
	"Clear": true, "ParseSchema": true, "SqlExec": true, "DropTable": true,
	"DropIndex": true, "DropTrigger": true, "Vacuum": true, "IncrVacuum": true,
	"IntegrityCk": true, "VCreate": true, "VDestroy": true, "VUpdate": true,
	"VRename": true, "JournalMode": true, "Checkpoint": true,
}

// checkPrepared refuses a statement go-sqlite3 has just compiled from query if
// text other than comments follows it, or if checkSavedSQLBytecode refuses it.
func (c *savedSQLConn) checkPrepared(ctx context.Context, query string, stmt driver.Stmt) error {
	tail, err := savedSQLStatementTail(stmt)
	if err != nil {
		return err
	}
	if !isOnlyCommentsAndSemicolons(tail) {
		return ErrSavedSQLMultipleStatements
	}
	return c.checkSavedSQLBytecode(ctx, query)
}

// savedSQLStatementTail returns the text go-sqlite3 left unparsed after the
// statement it prepared. go-sqlite3 keeps it in an unexported field and offers
// no accessor; if a release renames it, every saved query is refused (and the
// driver tests fail) rather than a second statement slipping by unnoticed.
func savedSQLStatementTail(stmt driver.Stmt) (string, error) {
	v := reflect.ValueOf(stmt)
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		if field := v.Elem().FieldByName("t"); field.IsValid() && field.Kind() == reflect.String {
			return field.String(), nil
		}
	}
	return "", errors.New("saved SQL driver: cannot read the statement tail from go-sqlite3")
}

// isOnlyCommentsAndSemicolons reports whether s holds no SQL: only whitespace,
// semicolons, and -- or /* */ comments.
func isOnlyCommentsAndSemicolons(s string) bool {
	for i := 0; i < len(s); {
		switch {
		case s[i] == ';' || s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '\f' || s[i] == '\v':
			i++
		case strings.HasPrefix(s[i:], "--"):
			end := strings.IndexByte(s[i:], '\n')
			if end < 0 {
				return true
			}
			i += end + 1
		case strings.HasPrefix(s[i:], "/*"):
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				// SQLite accepts an unterminated comment at the end of input.
				return true
			}
			i += 2 + end + 2
		default:
			return false
		}
	}
	return true
}

// checkSavedSQLBytecode compiles query as EXPLAIN on this connection and
// refuses it unless every b-tree its bytecode opens is a table, or an index of
// a table, that IsSavedSQLAllowedTable allows, in the main schema.
//
// A table or index read always takes an OpenRead (or ReopenIdx) of its root
// page, whatever the SQL looked like, so the check holds where the authorizer
// is not told about a read (see ReadOnlySQLiteDriverName). Root pages are mapped
// to tables through sqlite_master (savedSQLRootPages). The map, the
// schema_version it is cached under and the EXPLAIN are all read inside one
// read transaction, so they describe the same schema.
//
// Virtual tables (VOpen) are not mapped: their cursor names no table. The
// virtual tables a statement can reach are the FTS5 indexes the server creates,
// json_each and json_tree, and the pragma_* functions. An FTS5 index reads its
// shadow tables and its content table through statements of its own, which the
// authorizer checks column by column (an index over a denied table fails as
// soon as it reads its _config table); a pragma function runs a PRAGMA the
// authorizer refuses. The build compiles in no module that reads pages directly
// (dbstat, sqlite_dbpage, sqlite_stmt); TestSavedSQLHandleRefusesJoinColumnBypass
// checks that.
func (c *savedSQLConn) checkSavedSQLBytecode(ctx context.Context, query string) (err error) {
	if !c.conn.AutoCommit() {
		return errors.New("saved SQL driver: connection is unexpectedly inside a transaction")
	}
	if err := c.internalExec(ctx, "BEGIN"); err != nil {
		return err
	}
	defer func() {
		if rbErr := c.internalExec(context.Background(), "ROLLBACK"); rbErr != nil {
			// The connection is left inside a transaction; database/sql
			// discards a connection that returns ErrBadConn.
			err = driver.ErrBadConn
		}
	}()

	roots, err := c.savedSQLRootPages(ctx)
	if err != nil {
		return err
	}

	stmt, err := c.conn.PrepareContext(ctx, "EXPLAIN "+query)
	if err != nil {
		return err
	}
	defer stmt.Close()
	rows, err := stmt.(driver.StmtQueryContext).QueryContext(ctx, nil)
	if err != nil {
		return err
	}
	defer rows.Close()

	// addr, opcode, p1, p2, p3, p4, p5, comment
	row := make([]driver.Value, len(rows.Columns()))
	if len(row) < 7 {
		return fmt.Errorf("saved SQL driver: EXPLAIN returned %d columns", len(row))
	}
	for {
		if err := rows.Next(row); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		opcode := explainString(row[1])
		p2, p3, p5 := explainInt(row[3]), explainInt(row[4]), explainInt(row[6])
		switch {
		case opcode == "OpenRead" || opcode == "ReopenIdx":
			if p3 != 0 {
				return errors.New("not authorized: saved SQL may read only the main database")
			}
			if p5&opflagP2IsReg != 0 {
				return fmt.Errorf("not authorized: saved SQL opens a b-tree it does not name (%s)", opcode)
			}
			table, ok := roots[p2]
			if !ok {
				return fmt.Errorf("not authorized: saved SQL may not read root page %d (the schema catalogue or an unknown b-tree)", p2)
			}
			if !IsSavedSQLAllowedTable(table) {
				return fmt.Errorf("not authorized: saved SQL may not read %s", table)
			}
		case opcode == "Transaction" && (explainInt(row[2]) != 0 || p2 != 0):
			return errors.New("not authorized: saved SQL may only read the main database")
		case savedSQLRefusedOpcodes[opcode]:
			return fmt.Errorf("not authorized: saved SQL may not run %s", opcode)
		}
	}
}

// savedSQLRootPages maps every table and index b-tree in the main schema to the
// table it holds or indexes, by root page. It must run inside the read
// transaction the caller compiles the EXPLAIN in. The map is cached on the
// connection under PRAGMA schema_version, which every schema change (a
// migration, the FTS setup, a dropped and recreated table that takes over a
// freed page) increments.
func (c *savedSQLConn) savedSQLRootPages(ctx context.Context) (map[int64]string, error) {
	var version int64
	err := c.internalQuery(ctx, "PRAGMA schema_version", func(row []driver.Value) {
		version = explainInt(row[0])
	})
	if err != nil {
		return nil, err
	}
	if c.rootPages != nil && c.schemaVersion == version {
		return c.rootPages, nil
	}
	roots := map[int64]string{}
	err = c.internalQuery(ctx,
		"SELECT rootpage, tbl_name FROM main.sqlite_master WHERE type IN ('table', 'index') AND rootpage > 0",
		func(row []driver.Value) { roots[explainInt(row[0])] = explainString(row[1]) })
	if err != nil {
		return nil, err
	}
	c.rootPages, c.schemaVersion = roots, version
	return roots, nil
}

// internalExec and internalQuery run the check's own fixed statements with the
// authorizer's internal allowances on.
func (c *savedSQLConn) internalExec(ctx context.Context, statement string) error {
	c.internal.Store(true)
	defer c.internal.Store(false)
	_, err := c.conn.ExecContext(ctx, statement, nil)
	return err
}

func (c *savedSQLConn) internalQuery(ctx context.Context, query string, scan func([]driver.Value)) error {
	c.internal.Store(true)
	defer c.internal.Store(false)
	rows, err := c.conn.QueryContext(ctx, query, nil)
	if err != nil {
		return err
	}
	defer rows.Close()
	row := make([]driver.Value, len(rows.Columns()))
	for {
		if err := rows.Next(row); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		scan(row)
	}
}

func explainInt(v driver.Value) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return -1
}

func explainString(v driver.Value) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	}
	return ""
}
