package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"

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

// sqliteConn is a go-sqlite3 connection whose transactions begin as described on
// sqliteDriver. Every other method is go-sqlite3's own.
type sqliteConn struct {
	*sqlite3.SQLiteConn
	// broken marks a connection that may still be query_only because clearing it
	// failed; database/sql discards it instead of handing it to the next caller.
	broken bool
}

func (c *sqliteConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *sqliteConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if !opts.ReadOnly {
		if _, err := c.ExecContext(ctx, "BEGIN IMMEDIATE", nil); err != nil {
			return nil, err
		}
		return &sqliteTx{conn: c}, nil
	}
	if _, err := c.ExecContext(ctx, "BEGIN", nil); err != nil {
		return nil, err
	}
	tx := &sqliteTx{conn: c, readOnly: true}
	if _, err := c.ExecContext(ctx, "PRAGMA query_only = 1", nil); err != nil {
		return nil, errors.Join(err, tx.Rollback())
	}
	return tx, nil
}

// IsValid implements driver.Validator.
func (c *sqliteConn) IsValid() bool {
	return !c.broken
}

type sqliteTx struct {
	conn     *sqliteConn
	readOnly bool
}

// Commit follows go-sqlite3's own: a COMMIT that fails may leave the transaction
// open, and database/sql considers it finished either way, so it is rolled back.
func (tx *sqliteTx) Commit() error {
	_, err := tx.conn.ExecContext(context.Background(), "COMMIT", nil)
	if err != nil {
		_, _ = tx.conn.ExecContext(context.Background(), "ROLLBACK", nil)
	}
	return errors.Join(err, tx.finish())
}

func (tx *sqliteTx) Rollback() error {
	_, err := tx.conn.ExecContext(context.Background(), "ROLLBACK", nil)
	return errors.Join(err, tx.finish())
}

// finish returns a read-only transaction's connection to read-write use.
func (tx *sqliteTx) finish() error {
	if !tx.readOnly {
		return nil
	}
	if _, err := tx.conn.ExecContext(context.Background(), "PRAGMA query_only = 0", nil); err != nil {
		tx.conn.broken = true
		return err
	}
	return nil
}
