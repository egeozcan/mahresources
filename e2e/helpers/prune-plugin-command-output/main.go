package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: prune-plugin-command-output <sqlite|postgres> <dsn> <run-id>")
		os.Exit(2)
	}
	driver, query, err := databaseConfig(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	dsn := os.Args[2]
	if driver == "sqlite3" {
		dsn += "?_busy_timeout=10000"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()

	result, err := db.Exec(query, os.Args[3])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		fmt.Fprintf(os.Stderr, "deleted %d output rows, want 1 (error: %v)\n", rows, err)
		os.Exit(1)
	}
}

func databaseConfig(database string) (driver, query string, err error) {
	switch strings.ToLower(database) {
	case "sqlite":
		return "sqlite3", "DELETE FROM plugin_command_run_outputs WHERE run_id = ?", nil
	case "postgres":
		return "postgres", "DELETE FROM plugin_command_run_outputs WHERE run_id = $1", nil
	default:
		return "", "", fmt.Errorf("unsupported database type %q", database)
	}
}
