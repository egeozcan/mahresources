package application_context

import (
	"mahresources/constants"
	"testing"
)

func TestGetDatabaseSchema_SQLite_Correctness(t *testing.T) {
	cfg := &MahresourcesInputConfig{
		MemoryDB: true,
		MemoryFS: true,
	}
	ctx, gormDB, _ := CreateContextWithConfig(cfg)
	ctx.Config.DbType = constants.DbTypeSqlite

	// Create tables with specific column orders
	err := gormDB.Exec("CREATE TABLE test_table_1 (id INTEGER, name TEXT)").Error
	if err != nil {
		t.Fatalf("failed to create table 1: %v", err)
	}
	err = gormDB.Exec("CREATE TABLE test_table_2 (id INTEGER, description TEXT, created_at DATETIME)").Error
	if err != nil {
		t.Fatalf("failed to create table 2: %v", err)
	}

	schema, err := ctx.GetDatabaseSchema()
	if err != nil {
		t.Fatal(err)
	}

	// Verify table existence
	if _, ok := schema["test_table_1"]; !ok {
		t.Error("test_table_1 not found in schema")
	}
	if _, ok := schema["test_table_2"]; !ok {
		t.Error("test_table_2 not found in schema")
	}

	// Verify column count and order for table 1
	cols1 := schema["test_table_1"]
	if len(cols1) != 2 {
		t.Errorf("expected 2 columns for test_table_1, got %d: %v", len(cols1), cols1)
	} else {
		if cols1[0] != "id" || cols1[1] != "name" {
			t.Errorf("incorrect column order for test_table_1, got %v, expected [id name]", cols1)
		}
	}

	// Verify column count and order for table 2
	cols2 := schema["test_table_2"]
	if len(cols2) != 3 {
		t.Errorf("expected 3 columns for test_table_2, got %d: %v", len(cols2), cols2)
	} else {
		if cols2[0] != "id" || cols2[1] != "description" || cols2[2] != "created_at" {
			t.Errorf("incorrect column order for test_table_2, got %v, expected [id description created_at]", cols2)
		}
	}
}
