//go:build postgres

package main

import (
	"context"
	"testing"

	"mahresources/internal/testpgutil"
	"mahresources/models"
)

// The PostgreSQL half of TestSavedSQLTablesClassifyEveryMigratedTable: every
// table, view, materialized view and foreign table the migration leaves in the
// saved-SQL schema is classified.
func TestSavedSQLTablesClassifyEveryMigratedTablePostgres(t *testing.T) {
	ctx := context.Background()
	container, err := testpgutil.StartContainer(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Stop(ctx) })
	db, dsn := container.CreateTestDBWithDSN(t)
	migrateEverything(t, db, dsn)

	var tables []string
	if err := db.Raw(`SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = ? AND c.relkind IN ('r', 'p', 'v', 'm', 'f') ORDER BY c.relname`,
		models.SavedSQLSchema).Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	assertEveryTableClassified(t, tables)
}
