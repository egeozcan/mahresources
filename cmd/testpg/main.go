//go:build postgres

package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"mahresources/internal/testpgutil"
)

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "createdb" {
		// createdb <admin-dsn> — creates a random database, prints its DSN
		createDB(os.Args[2])
		return
	}

	// Default: start container, print DSN, wait for signal
	ctx := context.Background()

	container, err := testpgutil.StartContainer(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(container.DSN())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Fprintf(os.Stderr, "Shutting down postgres container...\n")
	if err := container.Stop(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping container: %v\n", err)
		os.Exit(1)
	}
}

func createDB(adminDSN string) {
	dbName := fmt.Sprintf("e2e_%d_%d", time.Now().UnixMilli(), rand.Intn(10000))

	db, err := sql.Open("postgres", adminDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE DATABASE " + dbName); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating database: %v\n", err)
		os.Exit(1)
	}

	// Replace database name in DSN
	u, err := url.Parse(adminDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing DSN: %v\n", err)
		os.Exit(1)
	}
	u.Path = "/" + dbName
	fmt.Println(u.String())
}
