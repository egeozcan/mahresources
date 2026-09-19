package main

import "testing"

func TestDatabaseConfig(t *testing.T) {
	tests := []struct {
		name       string
		database   string
		wantDriver string
		wantQuery  string
		wantErr    bool
	}{
		{name: "sqlite", database: "sqlite", wantDriver: "sqlite3", wantQuery: "DELETE FROM plugin_command_run_outputs WHERE run_id = ?"},
		{name: "postgres", database: "postgres", wantDriver: "postgres", wantQuery: "DELETE FROM plugin_command_run_outputs WHERE run_id = $1"},
		{name: "case insensitive", database: "POSTGRES", wantDriver: "postgres", wantQuery: "DELETE FROM plugin_command_run_outputs WHERE run_id = $1"},
		{name: "unknown", database: "mysql", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver, query, err := databaseConfig(tt.database)
			if tt.wantErr {
				if err == nil {
					t.Fatal("databaseConfig error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if driver != tt.wantDriver || query != tt.wantQuery {
				t.Fatalf("databaseConfig(%q) = (%q, %q), want (%q, %q)", tt.database, driver, query, tt.wantDriver, tt.wantQuery)
			}
		})
	}
}
