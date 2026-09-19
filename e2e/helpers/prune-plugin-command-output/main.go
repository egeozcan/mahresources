package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: prune-plugin-command-output <sqlite-dsn> <run-id>")
		os.Exit(2)
	}
	db, err := sql.Open("sqlite3", os.Args[1]+"?_busy_timeout=10000")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()

	result, err := db.Exec("DELETE FROM plugin_command_run_outputs WHERE run_id = ?", os.Args[2])
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
