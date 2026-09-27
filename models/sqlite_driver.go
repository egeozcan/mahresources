package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
)

// SQLiteDriverName is the database/sql driver every production SQLite handle
// opens with. CreateDatabaseConnection and CreateReadOnlyDatabaseConnection
// register it; a handle opened with the plain "sqlite3" driver gets neither the
// connection PRAGMAs nor the transaction semantics below.
const SQLiteDriverName = "sqlite3_pragmas"

// ReadOnlyTxOptions declares a transaction that never writes:
// db.Transaction(fn, models.ReadOnlyTxOptions(db)...). On SQLite it begins
// DEFERRED under query_only instead of taking the writer lock (see sqliteDriver);
// on any other database it changes nothing.
func ReadOnlyTxOptions(db *gorm.DB) []*sql.TxOptions {
	if db.Dialector.Name() != "sqlite" {
		return nil
	}
	return []*sql.TxOptions{{ReadOnly: true}}
}

// sqliteDriver wraps go-sqlite3 so that a transaction states its intent when it
// begins, instead of discovering it at its first write.
//
// SQLite's default BEGIN is DEFERRED: it takes no lock, the first read takes a
// WAL snapshot, and the first write tries to promote that snapshot to the writer
// lock. If another connection committed in between, the promotion fails with
// SQLITE_BUSY_SNAPSHOT, and SQLite does not call the busy handler for that code,
// so busy_timeout never applies: a transaction that reads before it writes fails
// with "database is locked" whenever any commit lands between its first read and
// its first write. BEGIN IMMEDIATE takes the writer lock at BEGIN, where the busy
// handler does apply, so it waits for the lock instead, and nothing can commit
// inside the transaction.
//
// Every transaction therefore begins IMMEDIATE unless it says it is read-only
// (sql.TxOptions{ReadOnly: true}). A read-only transaction begins DEFERRED, so it
// holds a snapshot without blocking writers, and runs with PRAGMA query_only, so
// a write inside it fails at once rather than reintroducing the promotion above.
// Postgres gives READ ONLY the same meaning.
type sqliteDriver struct {
	sqlite3.SQLiteDriver
}

func (d *sqliteDriver) Open(dsn string) (driver.Conn, error) {
	conn, err := d.SQLiteDriver.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &sqliteConn{SQLiteConn: conn.(*sqlite3.SQLiteConn)}, nil
}

// idleWriterLockWarnAfter is how long a write transaction may hold the writer
// lock without writing before its end is reported: every other writer in the
// process waited that long for nothing, and the fix is almost always to declare
// the transaction read-only.
var idleWriterLockWarnAfter = time.Second

// warnIdleWriterLock reports such a transaction.
var warnIdleWriterLock = log.Printf

// sqliteConn is a go-sqlite3 connection whose transactions begin as described on
// sqliteDriver. Every other method is go-sqlite3's own.
type sqliteConn struct {
	*sqlite3.SQLiteConn
	// broken marks a connection that may still be query_only because clearing it
	// failed, or still inside a transaction because rolling it back failed;
	// database/sql discards it instead of handing it to the next caller.
	broken bool
	// writeTx is the write transaction open on this connection, if any, watched
	// until its first write.
	writeTx *sqliteTx
}

func (c *sqliteConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *sqliteConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if !opts.ReadOnly {
		if err := execTransactionControl(ctx, c, "BEGIN IMMEDIATE"); err != nil {
			c.abandonFailedBegin()
			return nil, err
		}
		tx := &sqliteTx{conn: c, locked: time.Now()}
		c.writeTx = tx
		return tx, nil
	}
	if err := execTransactionControl(ctx, c, "BEGIN"); err != nil {
		c.abandonFailedBegin()
		return nil, err
	}
	tx := &sqliteTx{conn: c, readOnly: true}
	if _, err := c.SQLiteConn.ExecContext(ctx, "PRAGMA query_only = 1", nil); err != nil {
		return nil, errors.Join(err, tx.Rollback())
	}
	return tx, nil
}

func (c *sqliteConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	result, err := c.SQLiteConn.ExecContext(ctx, query, args)
	if err == nil {
		c.noteStatement(query)
	}
	return result, err
}

func (c *sqliteConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.SQLiteConn.QueryContext(ctx, query, args)
	if err == nil {
		c.noteStatement(query)
	}
	return rows, err
}

// noteStatement records that the open write transaction has written, once a
// statement that writes returned without an error (for a query, that is before its
// rows are read). It reads only the statement's leading keyword, so a write it
// misjudges, or one through a prepared statement, changes only whether a warning
// is logged.
func (c *sqliteConn) noteStatement(query string) {
	if c.writeTx == nil || c.writeTx.wrote {
		return
	}
	c.writeTx.wrote = statementWrites(query)
}

// statementWrites reports whether query changes the database, judged by its
// first keyword.
func statementWrites(query string) bool {
	query = strings.TrimLeft(query, " \t\r\n(")
	end := strings.IndexFunc(query, func(r rune) bool { return !unicode.IsLetter(r) })
	if end < 0 {
		end = len(query)
	}
	switch strings.ToUpper(query[:end]) {
	case "INSERT", "UPDATE", "DELETE", "REPLACE", "CREATE", "DROP", "ALTER", "REINDEX", "ANALYZE":
		return true
	case "WITH":
		upper := strings.ToUpper(query)
		return strings.Contains(upper, "INSERT ") || strings.Contains(upper, "UPDATE ") ||
			strings.Contains(upper, "DELETE ") || strings.Contains(upper, "REPLACE ")
	default:
		return false
	}
}

// IsValid implements driver.Validator.
func (c *sqliteConn) IsValid() bool {
	return !c.broken
}

type sqliteTx struct {
	conn     *sqliteConn
	readOnly bool
	// locked is when a write transaction took the writer lock, and wrote whether
	// it has written since.
	locked time.Time
	wrote  bool
}

// execTransactionControl runs BEGIN, COMMIT and ROLLBACK; a test replaces it to
// make one fail.
var execTransactionControl = func(ctx context.Context, c *sqliteConn, statement string) error {
	_, err := c.SQLiteConn.ExecContext(ctx, statement, nil)
	return err
}

// Commit follows go-sqlite3's own: a COMMIT that fails may leave the transaction
// open, and database/sql considers it finished either way, so it is rolled back.
func (tx *sqliteTx) Commit() error {
	err := execTransactionControl(context.Background(), tx.conn, "COMMIT")
	if err != nil {
		tx.conn.afterRollback(execTransactionControl(context.Background(), tx.conn, "ROLLBACK"))
	}
	return errors.Join(err, tx.finish())
}

func (tx *sqliteTx) Rollback() error {
	err := execTransactionControl(context.Background(), tx.conn, "ROLLBACK")
	tx.conn.afterRollback(err)
	return errors.Join(err, tx.finish())
}

// abandonFailedBegin ends a transaction a BEGIN reported as failed yet began: a
// BEGIN still waiting for the writer lock when its context ends can take the lock
// before the error reaches here. database/sql believes no transaction exists and
// pools the connection.
func (c *sqliteConn) abandonFailedBegin() {
	if !c.SQLiteConn.AutoCommit() {
		c.afterRollback(execTransactionControl(context.Background(), c, "ROLLBACK"))
	}
}

// afterRollback discards a connection a failed ROLLBACK left inside its
// transaction: database/sql considers the transaction over and would pool the
// connection, still holding whatever the transaction held (for a write
// transaction, the writer lock). Closing it ends the transaction. A ROLLBACK that
// failed because SQLite had already ended the transaction itself leaves nothing
// behind, and the connection stays.
func (c *sqliteConn) afterRollback(err error) {
	if err != nil && !c.SQLiteConn.AutoCommit() {
		c.broken = true
	}
}

// finish returns the connection to use outside a transaction: a read-only one's
// query_only is cleared, and a write transaction that held the lock long without
// writing is reported.
func (tx *sqliteTx) finish() error {
	if !tx.readOnly {
		tx.conn.writeTx = nil
		if held := time.Since(tx.locked); !tx.wrote && held >= idleWriterLockWarnAfter {
			warnIdleWriterLock("sqlite: a transaction held the writer lock for %s without writing; "+
				"if it only reads, pass models.ReadOnlyTxOptions to Transaction (from %s)",
				held.Round(time.Millisecond), transactionOpener())
		}
		return nil
	}
	if _, err := tx.conn.SQLiteConn.ExecContext(context.Background(), "PRAGMA query_only = 0", nil); err != nil {
		tx.conn.broken = true
		return err
	}
	return nil
}

// transactionOpener names the first caller outside database/sql, GORM and this
// file: the code whose transaction is ending.
func transactionOpener() string {
	pcs := make([]uintptr, 32)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(3, pcs)])
	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.File, "/database/sql/") && !strings.Contains(frame.File, "gorm.io/") &&
			!strings.HasSuffix(frame.File, "models/sqlite_driver.go") {
			return fmt.Sprintf("%s (%s:%d)", frame.Function, filepath.Base(frame.File), frame.Line)
		}
		if !more {
			return "unknown"
		}
	}
}
