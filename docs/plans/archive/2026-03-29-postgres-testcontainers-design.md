# Postgres Testcontainers Integration

**Date:** 2026-03-29
**Status:** Approved

## Problem

All Go tests and E2E tests run exclusively against in-memory SQLite. The app supports Postgres in production, and the MRQL translator has Postgres-specific branches (ILIKE vs LIKE, `json_extract` vs `->>`) that are never tested. Dialect-specific bugs can ship undetected.

## Solution

Add Postgres test coverage using [testcontainers-go](https://golang.testcontainers.org/) behind a `postgres` build tag. Three test layers run against Postgres:

1. **MRQL translator tests** — dialect-specific SQL generation
2. **API integration tests** — full CRUD surface (100+ tests)
3. **E2E tests** — browser + CLI against a real Postgres-backed server

## Design

### `cmd/testpg/main.go` — Testcontainer Launcher

A small Go binary that:
1. Starts a Postgres testcontainer (latest postgres image)
2. Creates a database
3. Prints the DSN to stdout (one line, parseable)
4. Waits for SIGINT/SIGTERM
5. Stops the container and exits

Used by both Go test helpers (via testcontainers Go API directly) and E2E scripts (via the binary). The binary exists so the Node E2E scripts can get a Postgres DSN without embedding testcontainers in the Node toolchain.

### Go Test Files (build tag `postgres`)

All Postgres test files use `//go:build postgres` so they're excluded from the default `go test` invocation.

**`mrql/pg_test_helper.go`** — Shared helper that starts a Postgres testcontainer using the testcontainers-go API, creates a per-package database, and returns a `*gorm.DB` configured with the postgres driver. The container is started once per package via `TestMain` and reused across all test functions. Within each test, `setupPostgresTestDB(t)` creates a fresh schema (AutoMigrate + seed) on a per-test database for isolation.

**`mrql/translator_pg_test.go`** — Runs the key translator tests against Postgres. Uses the same test data seeding as the SQLite tests (tags, groups, resources, notes with ownership hierarchy). Exercises the Postgres-specific code paths: ILIKE, `json_extract` vs `->>`, and cross-entity queries.

**`server/api_tests/pg_test_helper.go`** — Extends `SetupTestEnv` to accept a Postgres connection. Starts one container per package, creates a per-test database for isolation.

**`server/api_tests/api_pg_test.go`** — Runs the existing API tests against Postgres by using the Postgres `SetupTestEnv` variant.

### Container Lifecycle

- **One container per test package** — started in `TestMain`, stopped in cleanup
- **Fresh database per test function** — `CREATE DATABASE test_<random>` for each test, dropped in `t.Cleanup`. This gives full isolation without container restart overhead.
- The container uses the standard `postgres:16-alpine` image

### E2E Postgres Variant

**`e2e/scripts/run-tests-postgres.js`** — Node script that:
1. Builds the `testpg` binary if needed (`go build -o testpg ./cmd/testpg/`)
2. Spawns the `testpg` binary, captures the DSN from stdout
3. Starts the mahresources server with `-db-type=POSTGRES -db-dsn=<DSN>` (instead of `-ephemeral`)
4. Runs Playwright tests against that server
5. Sends SIGTERM to `testpg` on cleanup

New npm script: `test:with-server:postgres` wires this up.

The E2E test specs themselves don't change — only the backing database differs.

### Build Tag Gating

| Command | What runs |
|---------|-----------|
| `go test --tags 'json1 fts5' ./...` | SQLite only (current default, fast) |
| `go test --tags 'json1 fts5 postgres' ./...` | SQLite + Postgres (slower, needs Docker) |
| `cd e2e && npm run test:with-server` | E2E against SQLite ephemeral |
| `cd e2e && npm run test:with-server:postgres` | E2E against Postgres testcontainer |

### Operational Note

Claude should run Postgres tests (`go test --tags 'json1 fts5 postgres' ./...` and `npm run test:with-server:postgres`) when finishing features or bugfixes, alongside the regular SQLite tests.

## Changes Per File

| File | Purpose |
|------|---------|
| `cmd/testpg/main.go` | Testcontainer launcher binary |
| `mrql/pg_test_helper.go` | `//go:build postgres` — Postgres test setup for MRQL |
| `mrql/translator_pg_test.go` | `//go:build postgres` — MRQL tests on Postgres |
| `server/api_tests/pg_test_helper.go` | `//go:build postgres` — Postgres SetupTestEnv |
| `server/api_tests/api_pg_test.go` | `//go:build postgres` — API tests on Postgres |
| `e2e/scripts/run-tests-postgres.js` | Node script: testpg + server + Playwright |
| `e2e/package.json` | Add `test:with-server:postgres` script |
| `go.mod` / `go.sum` | Add `testcontainers-go` dependency |
| `CLAUDE.md` | Document postgres test commands |
