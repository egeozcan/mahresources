package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

// captureIdleWriterLockWarnings shortens the threshold for the warning about a
// transaction that holds the writer lock without writing, and collects what it
// reports instead of logging it.
func captureIdleWriterLockWarnings(t *testing.T) *[]string {
	t.Helper()
	var warnings []string
	previousAfter, previousWarn := idleWriterLockWarnAfter, warnIdleWriterLock
	idleWriterLockWarnAfter = 50 * time.Millisecond
	warnIdleWriterLock = func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { idleWriterLockWarnAfter, warnIdleWriterLock = previousAfter, previousWarn })
	return &warnings
}

func TestSQLiteWarnsAboutAWriteTransactionThatHeldTheLockWithoutWriting(t *testing.T) {
	db, _ := openProductionSQLite(t)
	warnings := captureIdleWriterLockWarnings(t)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		countRows(t, tx)
		time.Sleep(80 * time.Millisecond)
		return nil
	}))
	require.Len(t, *warnings, 1)
	require.Contains(t, (*warnings)[0], "without writing")
	require.Contains(t, (*warnings)[0], "TestSQLiteWarnsAboutAWriteTransactionThatHeldTheLockWithoutWriting",
		"the warning does not name the code that opened the transaction")
}

func TestSQLiteDoesNotWarnAboutTransactionsThatWroteOrWereReadOnly(t *testing.T) {
	db, _ := openProductionSQLite(t)
	warnings := captureIdleWriterLockWarnings(t)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		countRows(t, tx)
		time.Sleep(80 * time.Millisecond)
		return tx.Exec("INSERT INTO t (v) VALUES ('wrote')").Error
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		countRows(t, tx)
		time.Sleep(80 * time.Millisecond)
		return nil
	}, &sql.TxOptions{ReadOnly: true}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		countRows(t, tx)
		return nil
	}))
	require.Empty(t, *warnings)
}

// failTransactionControl makes the named transaction statements (COMMIT, ROLLBACK)
// fail without running, as an I/O error would, until the test ends.
func failTransactionControl(t *testing.T, statements ...string) {
	t.Helper()
	previous := execTransactionControl
	execTransactionControl = func(ctx context.Context, c *sqliteConn, statement string) error {
		for _, failing := range statements {
			if statement == failing {
				return errors.New("injected failure of " + statement)
			}
		}
		return previous(ctx, c, statement)
	}
	t.Cleanup(func() { execTransactionControl = previous })
}

// A transaction whose ROLLBACK failed may still be open, holding the writer lock;
// its connection must be discarded rather than pooled, which closes it and ends the
// transaction.
func TestSQLiteConnectionLeftInATransactionIsNotPooled(t *testing.T) {
	errAbandon := errors.New("abandon")
	for name, tc := range map[string]struct {
		failing []string
		outcome error
	}{
		"a rollback that failed":                     {[]string{"ROLLBACK"}, errAbandon},
		"a commit and then its rollback that failed": {[]string{"COMMIT", "ROLLBACK"}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			db, other := openProductionSQLite(t)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			failTransactionControl(t, tc.failing...)

			err = db.Transaction(func(tx *gorm.DB) error {
				require.NoError(t, tx.Exec("INSERT INTO t (v) VALUES ('never committed')").Error)
				return tc.outcome
			})
			require.Error(t, err)

			_, err = other.Exec("INSERT INTO t (v) VALUES ('another connection')")
			require.NoError(t, err, "a connection still in its transaction went back to the pool holding the writer lock")
			require.NoError(t, db.Exec("INSERT INTO t (v) VALUES ('the pool')").Error)
			require.EqualValues(t, 2, countRows(t, db))
		})
	}
}

// A BEGIN reported as failed after it took effect (a context cancelled as it
// returned) must not leave its transaction on a pooled connection either.
func TestSQLiteBeginReportedFailedAfterTakingEffectLeavesNothingOpen(t *testing.T) {
	db, other := openProductionSQLite(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previous := execTransactionControl
	execTransactionControl = func(ctx context.Context, c *sqliteConn, statement string) error {
		if err := previous(ctx, c, statement); err != nil || statement != "BEGIN IMMEDIATE" {
			return err
		}
		return errors.New("injected failure after BEGIN IMMEDIATE ran")
	}
	t.Cleanup(func() { execTransactionControl = previous })

	require.Error(t, db.Transaction(func(tx *gorm.DB) error { return nil }))
	execTransactionControl = previous

	_, err = other.Exec("INSERT INTO t (v) VALUES ('another connection')")
	require.NoError(t, err, "the BEGIN reported as failed left its transaction holding the writer lock")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec("INSERT INTO t (v) VALUES ('the pool')").Error
	}))
	require.EqualValues(t, 2, countRows(t, db))
}
