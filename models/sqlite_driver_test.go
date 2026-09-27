package models

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/constants"
)

// openProductionSQLite opens a WAL database file the way the server does, and a
// second, plain connection to the same file that never waits for a lock: a write
// through it either commits at once or reports "database is locked", so a test
// can ask whether a transaction holds the writer lock without a race.
func openProductionSQLite(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "driver.db")
	db, _, err := CreateDatabaseConnection(constants.DbTypeSqlite, "file:"+path+"?_journal_mode=WAL", "", 0)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)").Error)

	other, err := sql.Open("sqlite3", "file:"+path+"?_busy_timeout=0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })
	return db, other
}

func countRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Table("t").Count(&n).Error)
	return n
}

func TestSQLiteWriteTransactionHoldsTheWriterLockFromBegin(t *testing.T) {
	db, other := openProductionSQLite(t)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := other.Exec("INSERT INTO t (v) VALUES ('before the first read')")
		require.ErrorContains(t, err, "database is locked", "a commit landed before the transaction's first statement")

		countRows(t, tx)
		_, err = other.Exec("INSERT INTO t (v) VALUES ('between the read and the write')")
		require.ErrorContains(t, err, "database is locked", "a commit landed between the transaction's read and its write")

		return tx.Exec("INSERT INTO t (v) VALUES ('mine')").Error
	})
	require.NoError(t, err, "a transaction that read before it wrote was refused")
	require.EqualValues(t, 1, countRows(t, db))
}

func TestSQLiteReadOnlyTransactionKeepsASnapshotWithoutBlockingWriters(t *testing.T) {
	db, other := openProductionSQLite(t)

	err := db.Transaction(func(tx *gorm.DB) error {
		before := countRows(t, tx)
		_, err := other.Exec("INSERT INTO t (v) VALUES ('committed meanwhile')")
		require.NoError(t, err, "a read-only transaction blocked a writer")
		require.Equal(t, before, countRows(t, tx), "a read-only transaction's second read saw a later commit")
		return nil
	}, &sql.TxOptions{ReadOnly: true})
	require.NoError(t, err)
	require.EqualValues(t, 1, countRows(t, db))
}

func TestSQLiteReadOnlyTransactionRefusesWritesAndLeavesItsConnectionWritable(t *testing.T) {
	db, _ := openProductionSQLite(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// One connection, so the statements after each transaction reuse the one it ran on.
	sqlDB.SetMaxOpenConns(1)

	errAbandon := errors.New("abandon")
	for _, outcome := range []error{nil, errAbandon} {
		err := db.Transaction(func(tx *gorm.DB) error {
			write := tx.Exec("INSERT INTO t (v) VALUES ('inside a read-only transaction')").Error
			require.ErrorContains(t, write, "readonly")
			return outcome
		}, &sql.TxOptions{ReadOnly: true})
		require.ErrorIs(t, err, outcome)

		require.NoError(t, db.Exec("INSERT INTO t (v) VALUES ('afterwards')").Error,
			"the connection stayed read-only after its read-only transaction ended")
	}
	require.EqualValues(t, 2, countRows(t, db))
}

// A BEGIN still waiting for the writer lock when its context ends may yet take the
// lock. Whatever BeginTx then reports has to match the connection: a transaction
// left open behind an error would be pooled holding the writer lock for good.
func TestSQLiteBeginCancelledWhileWaitingLeavesNoTransactionOpen(t *testing.T) {
	db, other := openProductionSQLite(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	holder, err := other.Conn(context.Background())
	require.NoError(t, err)
	_, err = holder.ExecContext(context.Background(), "BEGIN IMMEDIATE")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	began := make(chan error, 1)
	go func() {
		tx, err := sqlDB.BeginTx(ctx, nil)
		if err == nil {
			_ = tx.Rollback()
		}
		began <- err
	}()
	// The BEGIN holds the pool's only connection while it waits for the lock.
	require.Eventually(t, func() bool { return sqlDB.Stats().InUse == 1 }, 5*time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
	_, err = holder.ExecContext(context.Background(), "ROLLBACK")
	require.NoError(t, err)
	require.NoError(t, holder.Close())
	<-began

	_, err = other.Exec("INSERT INTO t (v) VALUES ('after the cancelled BEGIN')")
	require.NoError(t, err, "the cancelled BEGIN left a transaction holding the writer lock")
	require.NoError(t, db.Exec("INSERT INTO t (v) VALUES ('on the pooled connection')").Error)
}
