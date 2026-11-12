package main

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/fts"
	"mahresources/models"
)

// migrateEverything builds the schema a server start builds: the model
// migration, the durable job core and, on SQLite, the full-text indexes.
func migrateEverything(t *testing.T, db *gorm.DB, dsn string) {
	t.Helper()
	if err := migrateModels(db); err != nil {
		t.Fatalf("migrateModels: %v", err)
	}
	if err := migrateJobCore(db, dsn); err != nil {
		t.Fatalf("migrateJobCore: %v", err)
	}
	if db.Dialector.Name() == "sqlite" {
		if err := fts.NewSQLiteFTS().Setup(db); err != nil {
			t.Fatalf("FTS setup: %v", err)
		}
	}
}

// assertEveryTableClassified fails for a migrated table the saved-SQL table
// list does not decide, and for an entry in that list naming no table.
func assertEveryTableClassified(t *testing.T, tables []string) {
	t.Helper()
	if len(tables) < 20 {
		t.Fatalf("only %d tables found (%v); the schema was not migrated", len(tables), tables)
	}
	present := map[string]bool{}
	for _, table := range tables {
		present[table] = true
		if !models.IsSavedSQLClassifiedTable(table) {
			t.Errorf("table %s is neither allowed nor denied to saved SQL; add it to savedSQLTables in models/saved_sql_tables.go", table)
		}
	}
	for table := range models.SavedSQLTableClassifications() {
		if !present[table] {
			t.Errorf("savedSQLTables names %s, which the migration does not create", table)
		}
	}
}

// GitHub #87: saved SQL reads only tables on an allow-list, and every table
// the server creates is on it as allowed or denied, so a new table cannot
// become readable by default or be forgotten.
func TestSavedSQLTablesClassifyEveryMigratedTable(t *testing.T) {
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, filepath.Join(t.TempDir(), "schema.db"), "", 0)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrateEverything(t, db, "")

	var tables []string
	if err := db.Raw(`SELECT name FROM sqlite_master WHERE type IN ('table', 'view')
		AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`).Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	assertEveryTableClassified(t, tables)
}
