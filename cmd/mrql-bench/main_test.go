package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func executeCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := newRootCommand(&stdout, &stderr)
	command.SetArgs(args)
	err := command.ExecuteContext(context.Background())
	return stdout.String() + stderr.String(), err
}

func TestListNeedsNoDatabaseAndIsDeterministic(t *testing.T) {
	first, err := executeCommand(t, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	second, err := executeCommand(t, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("list output is not deterministic")
	}
	for _, value := range []string{"100k", "3m", "scalar-selective", "nested-mrql"} {
		if !strings.Contains(first, value) {
			t.Errorf("list output missing %q", value)
		}
	}
}

func TestPrepareRejectsUnsafePostgresBeforeConnecting(t *testing.T) {
	output, err := executeCommand(t, "prepare", "--backend", "postgres", "--dsn", "postgres://user:secret@localhost/production", "--profile", "tiny")
	if err == nil {
		t.Fatal("expected unsafe Postgres preparation to fail")
	}
	if strings.Contains(output+err.Error(), "secret") {
		t.Fatal("error output leaked DSN credentials")
	}
}

func TestPrepareRequiresCredentialFreePostgresManifestPath(t *testing.T) {
	output, err := executeCommand(t, "prepare", "--backend", "postgres", "--dsn", "postgres://user:secret@localhost/mrql_benchmark", "--profile", "tiny", "--allow-destructive")
	if err == nil {
		t.Fatal("expected explicit PostgreSQL manifest validation")
	}
	if strings.Contains(output+err.Error(), "secret") {
		t.Fatal("validation leaked DSN credentials")
	}
}

func TestPostgresDSNSafetyUsesEffectiveDatabaseAndRejectsSearchPath(t *testing.T) {
	for _, dsn := range []string{
		"postgres://user:secret@localhost/mrql_benchmark?dbname=production",
		"postgres://user:secret@localhost/mrql_benchmark?database=production",
		"postgres://user:secret@localhost/mrql_benchmark?search_path=production",
	} {
		if err := requireBenchmarkPostgresDSN(dsn); err == nil {
			t.Errorf("unsafe DSN was accepted: %s", dsn)
		}
	}
	if err := requireBenchmarkPostgresDSN("postgres://user:secret@localhost/mrql_benchmark?sslmode=disable"); err != nil {
		t.Fatalf("safe benchmark DSN rejected: %v", err)
	}
}

func TestRunRequiresOutput(t *testing.T) {
	_, err := executeCommand(t, "run", "--dsn", "fixture.db", "--manifest", "fixture.json")
	if err == nil {
		t.Fatal("expected required output validation")
	}
}
