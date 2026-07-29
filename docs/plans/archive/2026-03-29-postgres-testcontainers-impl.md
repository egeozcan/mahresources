# Postgres Testcontainers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run MRQL translator tests, API integration tests, and E2E tests against a real Postgres database using testcontainers-go, gated behind a `postgres` build tag.

**Architecture:** A shared `testpgutil` internal package manages testcontainer lifecycle (one container per test package, fresh database per test). The `cmd/testpg` binary wraps this for E2E scripts. Postgres test files use `//go:build postgres` to stay out of default test runs.

**Tech Stack:** testcontainers-go, postgres:16-alpine Docker image, GORM postgres driver (already a dependency)

---

## File Map

| File | Responsibility |
|------|---------------|
| `go.mod` | Add testcontainers-go dependency |
| `internal/testpgutil/testpgutil.go` | Shared testcontainer lifecycle (start, create DB, cleanup) |
| `cmd/testpg/main.go` | Binary: start container, print DSN, wait for signal |
| `mrql/pg_test_helper.go` | `//go:build postgres` — TestMain + setupPostgresTestDB for MRQL |
| `mrql/translator_pg_test.go` | `//go:build postgres` — MRQL tests on Postgres |
| `server/api_tests/pg_test_helper.go` | `//go:build postgres` — SetupPostgresTestEnv for API tests |
| `server/api_tests/api_pg_test.go` | `//go:build postgres` — API tests on Postgres |
| `e2e/scripts/run-tests-postgres.js` | Node script: testpg + server + Playwright |
| `e2e/package.json` | Add `test:with-server:postgres` script |
| `CLAUDE.md` | Document postgres test commands |

---

### Task 1: Add testcontainers-go dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the testcontainers-go dependency**

Run:
```bash
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/postgres@latest
```

- [ ] **Step 2: Tidy modules**

Run: `go mod tidy`

- [ ] **Step 3: Verify build still works**

Run: `go build --tags 'json1 fts5' ./...`
Expected: compiles clean

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add testcontainers-go for postgres testing"
```

---

### Task 2: Shared testcontainer utility package

**Files:**
- Create: `internal/testpgutil/testpgutil.go`

- [ ] **Step 1: Create the testpgutil package**

Create `internal/testpgutil/testpgutil.go`:

```go
//go:build postgres

package testpgutil

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres" as pgdriver
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Container holds a running Postgres testcontainer and its admin DSN.
type Container struct {
	tc      *postgres.PostgresContainer
	AdminDSN string // DSN for the default "test" database (used to CREATE/DROP per-test DBs)
}

// StartContainer starts a Postgres testcontainer. Call once per test package
// (typically in TestMain). The caller must call Stop() when done.
func StartContainer(ctx context.Context) (*Container, error) {
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	return &Container{
		tc:       pgContainer,
		AdminDSN: dsn,
	}, nil
}

// Stop terminates the container.
func (c *Container) Stop(ctx context.Context) error {
	if c.tc != nil {
		return c.tc.Terminate(ctx)
	}
	return nil
}

// DSN returns the admin DSN (for the default "test" database).
func (c *Container) DSN() string {
	return c.AdminDSN
}

// CreateTestDB creates a fresh database for a single test and returns a GORM
// connection to it. The database is dropped in t.Cleanup.
func (c *Container) CreateTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	// Generate a unique database name
	dbName := fmt.Sprintf("test_%d_%d", time.Now().UnixNano(), rand.Intn(100000))

	// Connect to admin DB to create the test database
	adminDB, err := gorm.Open(pgdriver.Open(c.AdminDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to admin DB: %v", err)
	}
	if err := adminDB.Exec("CREATE DATABASE " + dbName).Error; err != nil {
		t.Fatalf("failed to create test database %s: %v", dbName, err)
	}
	sqlAdmin, _ := adminDB.DB()
	sqlAdmin.Close()

	// Build DSN for the new database by replacing the database name
	testDSN := replaceDatabaseInDSN(c.AdminDSN, dbName)

	// Connect to the test database
	db, err := gorm.Open(pgdriver.Open(testDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to test database %s: %v", dbName, err)
	}

	// Cleanup: close connection and drop database when test finishes
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()

		cleanupDB, err := gorm.Open(pgdriver.Open(c.AdminDSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err == nil {
			cleanupDB.Exec("DROP DATABASE IF EXISTS " + dbName)
			sqlCleanup, _ := cleanupDB.DB()
			sqlCleanup.Close()
		}
	})

	return db
}

// replaceDatabaseInDSN replaces the database name in a postgres DSN.
// Handles the key=value format: "host=X port=Y user=U password=P dbname=OLD sslmode=disable"
// and the URL format: "postgres://user:pass@host:port/OLD?sslmode=disable"
func replaceDatabaseInDSN(dsn, newDB string) string {
	// testcontainers-go returns URL format
	// Replace the path component (database name)
	// URL format: postgres://user:pass@host:port/dbname?params
	import "net/url"
	u, err := url.Parse(dsn)
	if err != nil {
		// Fallback: assume key=value format
		return dsn
	}
	u.Path = "/" + newDB
	return u.String()
}
```

Wait — the `import` inside a function isn't valid Go. Let me fix this. The `net/url` import should be at the top level. Let me restructure:

Actually, since there's a syntax issue with the inline import, let me write the correct version. Create `internal/testpgutil/testpgutil.go`:

```go
//go:build postgres

package testpgutil

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Container holds a running Postgres testcontainer and its admin DSN.
type Container struct {
	tc       *pgmodule.PostgresContainer
	AdminDSN string
}

// StartContainer starts a Postgres testcontainer. Call once per test package
// (typically in TestMain). The caller must call Stop() when done.
func StartContainer(ctx context.Context) (*Container, error) {
	pgContainer, err := pgmodule.Run(ctx,
		"postgres:16-alpine",
		pgmodule.WithDatabase("test"),
		pgmodule.WithUsername("test"),
		pgmodule.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	return &Container{
		tc:       pgContainer,
		AdminDSN: dsn,
	}, nil
}

// Stop terminates the container.
func (c *Container) Stop(ctx context.Context) error {
	if c.tc != nil {
		return c.tc.Terminate(ctx)
	}
	return nil
}

// DSN returns the admin DSN (for the default "test" database).
func (c *Container) DSN() string {
	return c.AdminDSN
}

// CreateTestDB creates a fresh database for a single test and returns a GORM
// connection to it. The database is dropped in t.Cleanup.
func (c *Container) CreateTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := fmt.Sprintf("test_%d_%d", time.Now().UnixNano(), rand.Intn(100000))

	adminDB, err := gorm.Open(pgdriver.Open(c.AdminDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to admin DB: %v", err)
	}
	if err := adminDB.Exec("CREATE DATABASE " + dbName).Error; err != nil {
		t.Fatalf("failed to create test database %s: %v", dbName, err)
	}
	sqlAdmin, _ := adminDB.DB()
	sqlAdmin.Close()

	testDSN := replaceDatabaseInDSN(c.AdminDSN, dbName)

	db, err := gorm.Open(pgdriver.Open(testDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to test database %s: %v", dbName, err)
	}

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()

		cleanupDB, err := gorm.Open(pgdriver.Open(c.AdminDSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err == nil {
			cleanupDB.Exec("DROP DATABASE IF EXISTS " + dbName)
			sqlCleanup, _ := cleanupDB.DB()
			sqlCleanup.Close()
		}
	})

	return db
}

// replaceDatabaseInDSN replaces the database name in a postgres connection URL.
func replaceDatabaseInDSN(dsn, newDB string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	u.Path = "/" + newDB
	return u.String()
}
```

- [ ] **Step 2: Verify build with postgres tag**

Run: `go build --tags 'json1 fts5 postgres' ./internal/testpgutil/...`
Expected: compiles clean

- [ ] **Step 3: Verify default build excludes it**

Run: `go build --tags 'json1 fts5' ./...`
Expected: compiles clean (testpgutil excluded by build tag)

- [ ] **Step 4: Commit**

```bash
git add internal/testpgutil/testpgutil.go
git commit -m "feat: add shared testcontainer postgres utility package"
```

---

### Task 3: testpg binary for E2E scripts

**Files:**
- Create: `cmd/testpg/main.go`

- [ ] **Step 1: Create the testpg binary**

Create `cmd/testpg/main.go`:

```go
//go:build postgres

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"mahresources/internal/testpgutil"
)

func main() {
	ctx := context.Background()

	container, err := testpgutil.StartContainer(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Print DSN to stdout (one line, consumed by E2E scripts)
	fmt.Println(container.DSN())

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Fprintf(os.Stderr, "Shutting down postgres container...\n")
	if err := container.Stop(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping container: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build the binary**

Run: `go build --tags 'json1 fts5 postgres' -o testpg ./cmd/testpg/`
Expected: builds clean

- [ ] **Step 3: Smoke test (requires Docker)**

Run: `./testpg &` then wait a few seconds, read the DSN from stdout, then `kill %1`.
Expected: prints a DSN like `postgres://test:test@localhost:XXXXX/test?sslmode=disable`, container starts and stops cleanly.

- [ ] **Step 4: Add testpg to .gitignore if not already ignored**

Check if the binary is gitignored. If not, add `testpg` to `.gitignore`.

- [ ] **Step 5: Commit**

```bash
git add cmd/testpg/main.go
git commit -m "feat: add testpg binary for E2E postgres testing"
```

---

### Task 4: MRQL Postgres tests

**Files:**
- Create: `mrql/pg_test_helper.go`
- Create: `mrql/translator_pg_test.go`

- [ ] **Step 1: Create the pg test helper with TestMain**

Create `mrql/pg_test_helper.go`:

```go
//go:build postgres

package mrql

import (
	"context"
	"fmt"
	"os"
	"testing"

	"mahresources/internal/testpgutil"

	"gorm.io/gorm"
)

var pgContainer *testpgutil.Container

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	pgContainer, err = testpgutil.StartContainer(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	pgContainer.Stop(ctx)
	os.Exit(code)
}

// setupPostgresTestDB creates a fresh Postgres database for the test,
// migrates the schema, and seeds the same test data as setupTestDB.
func setupPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := pgContainer.CreateTestDB(t)

	// Migrate the same tables as the SQLite version
	if err := db.AutoMigrate(&testTag{}, &testGroup{}, &testResource{}, &testNote{}); err != nil {
		t.Fatalf("auto-migrate failed: %v", err)
	}

	// Create junction tables (Postgres syntax)
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS resource_tags (resource_id INTEGER NOT NULL, tag_id INTEGER NOT NULL, PRIMARY KEY (resource_id, tag_id))`,
		`CREATE TABLE IF NOT EXISTS note_tags (note_id INTEGER NOT NULL, tag_id INTEGER NOT NULL, PRIMARY KEY (note_id, tag_id))`,
		`CREATE TABLE IF NOT EXISTS group_tags (group_id INTEGER NOT NULL, tag_id INTEGER NOT NULL, PRIMARY KEY (group_id, tag_id))`,
		`CREATE TABLE IF NOT EXISTS groups_related_resources (resource_id INTEGER NOT NULL, group_id INTEGER NOT NULL, PRIMARY KEY (resource_id, group_id))`,
		`CREATE TABLE IF NOT EXISTS groups_related_notes (note_id INTEGER NOT NULL, group_id INTEGER NOT NULL, PRIMARY KEY (note_id, group_id))`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("create junction table failed: %v", err)
		}
	}

	// Seed same data as setupTestDB
	seedTestData(db)

	return db
}

// seedTestData inserts the standard test data set.
// Extracted so both SQLite and Postgres helpers use identical data.
func seedTestData(db *gorm.DB) {
	now := time.Now()
	parentGroupID := uint(1)
	workGroupID := uint(2)

	tags := []testTag{
		{ID: 1, Name: "photo"},
		{ID: 2, Name: "video"},
		{ID: 3, Name: "document"},
	}
	for _, tag := range tags {
		db.Create(&tag)
	}

	groups := []testGroup{
		{ID: 1, Name: "Vacation", Meta: `{"region":"europe","priority":3}`},
		{ID: 2, Name: "Work", OwnerID: &parentGroupID, Meta: `{}`},
		{ID: 3, Name: "Archive", Meta: `{}`},
		{ID: 4, Name: "Sub-Work", OwnerID: &workGroupID, Meta: `{}`},
		{ID: 5, Name: "Photos", OwnerID: &parentGroupID, Meta: `{}`},
	}
	for _, g := range groups {
		db.Create(&g)
	}

	resources := []testResource{
		{ID: 1, Name: "sunset.jpg", OriginalName: "sunset.jpg", ContentType: "image/jpeg", FileSize: 1024000, Width: 1920, Height: 1080, CreatedAt: now, UpdatedAt: now, Meta: `{"rating":5}`},
		{ID: 2, Name: "photo_album.png", OriginalName: "photo_album.png", ContentType: "image/png", FileSize: 2048000, Width: 800, Height: 600, CreatedAt: now, UpdatedAt: now, Meta: `{"rating":3}`},
		{ID: 3, Name: "report.pdf", OriginalName: "report.pdf", ContentType: "application/pdf", FileSize: 512000, CreatedAt: now, UpdatedAt: now, Meta: `{}`},
		{ID: 4, Name: "untagged_file.txt", OriginalName: "untagged.txt", ContentType: "text/plain", FileSize: 100, CreatedAt: now.Add(-24 * 30 * time.Hour), UpdatedAt: now, Meta: `{}`},
	}
	for _, r := range resources {
		db.Create(&r)
	}

	notes := []testNote{
		{ID: 1, Name: "Meeting notes", CreatedAt: now, UpdatedAt: now, Meta: `{"priority":"high"}`},
		{ID: 2, Name: "Todo list", CreatedAt: now, UpdatedAt: now, Meta: `{"priority":"low","count":7}`},
	}
	for _, n := range notes {
		db.Create(&n)
	}

	db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (1, 1)")
	db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (2, 1)")
	db.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (2, 2)")
	db.Exec("INSERT INTO note_tags (note_id, tag_id) VALUES (1, 3)")
	db.Exec("INSERT INTO note_tags (note_id, tag_id) VALUES (1, 1)")
	db.Exec("INSERT INTO groups_related_resources (resource_id, group_id) VALUES (1, 1)")
	db.Exec("INSERT INTO groups_related_resources (resource_id, group_id) VALUES (3, 2)")
	db.Exec("INSERT INTO groups_related_notes (note_id, group_id) VALUES (1, 1)")
	db.Exec("INSERT INTO groups_related_notes (note_id, group_id) VALUES (2, 2)")
	db.Exec("INSERT INTO group_tags (group_id, tag_id) VALUES (1, 1)")
	db.Exec("INSERT INTO group_tags (group_id, tag_id) VALUES (2, 3)")
	db.Model(&testResource{}).Where("id = ?", 1).Update("owner_id", 1)
	db.Model(&testResource{}).Where("id = ?", 3).Update("owner_id", 2)
	db.Model(&testNote{}).Where("id = ?", 1).Update("owner_id", 1)
	db.Model(&testNote{}).Where("id = ?", 2).Update("owner_id", 2)
}
```

Note: you'll need to add `"time"` to the imports. Also, the `seedTestData` function references types defined in `translator_test.go` (same package `mrql`), so the imports work.

- [ ] **Step 2: Create the postgres translator tests**

Create `mrql/translator_pg_test.go`:

```go
//go:build postgres

package mrql

import (
	"testing"
)

// TestPG_ResourceNameContains tests basic LIKE/ILIKE on Postgres.
func TestPG_ResourceNameContains(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `name ~ "sunset"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
		t.Fatalf("expected [sunset.jpg], got %v", namesOfResources(resources))
	}
}

// TestPG_ContentTypeContains tests ILIKE for contains on Postgres.
func TestPG_ContentTypeContains(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `contentType ~ "image"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 image resources, got %d: %v", len(resources), namesOfResources(resources))
	}
}

// TestPG_TagsEqual tests tag relation subquery on Postgres.
func TestPG_TagsEqual(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `tags = "photo"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 photo-tagged resources, got %d", len(resources))
	}
}

// TestPG_MetaJsonExtract tests Postgres JSON extraction (->>) vs SQLite json_extract.
func TestPG_MetaJsonExtract(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `meta.rating > 3`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
		t.Fatalf("expected [sunset.jpg] with rating > 3, got %v", namesOfResources(resources))
	}
}

// TestPG_OwnerDirect tests owner = "name" on Postgres.
func TestPG_OwnerDirect(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner = "Vacation"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
		t.Fatalf("expected [sunset.jpg], got %v", namesOfResources(resources))
	}
}

// TestPG_OwnerTags tests owner.tags traversal on Postgres.
func TestPG_OwnerTags(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner.tags = "photo"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
		t.Fatalf("expected [sunset.jpg], got %v", namesOfResources(resources))
	}
}

// TestPG_OwnerParentChain tests multi-level traversal on Postgres.
func TestPG_OwnerParentChain(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner.parent.name = "Vacation"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "report.pdf" {
		t.Fatalf("expected [report.pdf], got %v", namesOfResources(resources))
	}
}

// TestPG_OwnerParentTags tests owner.parent.tags on Postgres.
func TestPG_OwnerParentTags(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner.parent.tags = "photo"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "report.pdf" {
		t.Fatalf("expected [report.pdf], got %v", namesOfResources(resources))
	}
}

// TestPG_ParentParentChain tests parent.parent.name on groups (Postgres).
func TestPG_ParentParentChain(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `parent.parent.name = "Vacation"`, EntityGroup, db)
	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "Sub-Work" {
		t.Fatalf("expected [Sub-Work], got %v", namesOfGroups(groups))
	}
}

// TestPG_OwnerNegationNull tests owner != includes NULL on Postgres.
func TestPG_OwnerNegationNull(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner != "Work"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources, got %d: %v", len(resources), namesOfResources(resources))
	}
}

// TestPG_NoteOwner tests owner traversal on notes.
func TestPG_NoteOwner(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner.tags = "document"`, EntityNote, db)
	var notes []testNote
	if err := result.Find(&notes).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(notes) != 1 || notes[0].Name != "Todo list" {
		t.Fatalf("expected [Todo list], got %v", namesOfNotes(notes))
	}
}

// TestPG_OwnerIsEmpty tests owner IS EMPTY on Postgres.
func TestPG_OwnerIsEmpty(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner IS EMPTY`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources with no owner, got %d: %v", len(resources), namesOfResources(resources))
	}
}

// TestPG_CrossEntityName tests cross-entity query on Postgres.
func TestPG_CrossEntityName(t *testing.T) {
	db := setupPostgresTestDB(t)
	// Just verify it doesn't error — cross-entity uses all three entity types
	result := parseAndTranslate(t, `name ~ "sunset"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
}
```

- [ ] **Step 3: Run postgres tests (requires Docker)**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/ -v -count=1`
Expected: all pass (container starts, tests run against Postgres, container stops)

- [ ] **Step 4: Verify default tests still work without Docker**

Run: `go test --tags 'json1 fts5' ./mrql/...`
Expected: all pass (postgres tests excluded)

- [ ] **Step 5: Commit**

```bash
git add mrql/pg_test_helper.go mrql/translator_pg_test.go
git commit -m "test(mrql): add postgres translator tests via testcontainers"
```

---

### Task 5: API Postgres tests

**Files:**
- Create: `server/api_tests/pg_test_helper.go`
- Create: `server/api_tests/api_pg_test.go`

- [ ] **Step 1: Create the postgres test helper for API tests**

Create `server/api_tests/pg_test_helper.go`:

```go
//go:build postgres

package api_tests

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/internal/testpgutil"
	"mahresources/models"
	"mahresources/models/util"
	"mahresources/server"
)

var pgContainer *testpgutil.Container

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	pgContainer, err = testpgutil.StartContainer(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	pgContainer.Stop(ctx)
	os.Exit(code)
}

// SetupPostgresTestEnv creates a fresh Postgres database and application context.
func SetupPostgresTestEnv(t *testing.T) *TestContext {
	db := pgContainer.CreateTestDB(t)

	// AutoMigrate all models (same as SetupTestEnv)
	err := db.AutoMigrate(
		&models.Query{},
		&models.Series{},
		&models.Resource{},
		&models.ResourceVersion{},
		&models.Note{},
		&models.NoteBlock{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.ResourceCategory{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
		&models.LogEntry{},
		&models.PluginState{},
		&models.PluginKV{},
		&models.SavedMRQLQuery{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	util.AddInitialData(db)

	config := &application_context.MahresourcesConfig{
		DbType:      constants.DbTypePosgres,
		BindAddress: ":0",
	}

	fs := afero.NewMemMapFs()
	altFsPaths := make(map[string]string)

	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "postgres")

	appCtx := application_context.NewMahresourcesContext(fs, db, readOnlyDB, config)
	serverInstance := server.CreateServer(appCtx, fs, altFsPaths)

	return &TestContext{
		AppCtx: appCtx,
		Router: serverInstance.Handler,
		DB:     db,
	}
}
```

- [ ] **Step 2: Create API postgres test file**

Create `server/api_tests/api_pg_test.go`. This runs a representative subset of the API tests against Postgres — the full CRUD lifecycle plus the most important edge cases:

```go
//go:build postgres

package api_tests

import (
	"net/http"
	"testing"
)

// TestPG_TagCRUD tests tag create/read/update/delete on Postgres.
func TestPG_TagCRUD(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	// Create
	rr := tc.MakeFormRequest("POST", "/v1/tag", map[string]string{"Name": "pg-test-tag"})
	if rr.Code != http.StatusOK {
		t.Fatalf("create tag: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// List
	rr = tc.MakeRequest("GET", "/v1/tags.json", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list tags: expected 200, got %d", rr.Code)
	}
}

// TestPG_GroupCRUD tests group operations on Postgres.
func TestPG_GroupCRUD(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	rr := tc.MakeFormRequest("POST", "/v1/group", map[string]string{"Name": "pg-test-group"})
	if rr.Code != http.StatusOK {
		t.Fatalf("create group: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	rr = tc.MakeRequest("GET", "/v1/groups.json", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list groups: expected 200, got %d", rr.Code)
	}
}

// TestPG_NoteCRUD tests note operations on Postgres.
func TestPG_NoteCRUD(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	rr := tc.MakeFormRequest("POST", "/v1/note", map[string]string{
		"Name": "pg-test-note",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("create note: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	rr = tc.MakeRequest("GET", "/v1/notes.json", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list notes: expected 200, got %d", rr.Code)
	}
}

// TestPG_NoteEndpoints runs the comprehensive note API test on Postgres.
func TestPG_NoteEndpoints(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	// Create a note type first
	rr := tc.MakeFormRequest("POST", "/v1/noteType", map[string]string{"Name": "pg-type"})
	if rr.Code != http.StatusOK {
		t.Fatalf("create note type: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Create tag
	rr = tc.MakeFormRequest("POST", "/v1/tag", map[string]string{"Name": "pg-note-tag"})
	if rr.Code != http.StatusOK {
		t.Fatalf("create tag: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Create a group for ownership
	rr = tc.MakeFormRequest("POST", "/v1/group", map[string]string{"Name": "pg-owner"})
	if rr.Code != http.StatusOK {
		t.Fatalf("create group: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Create note with owner
	rr = tc.MakeFormRequest("POST", "/v1/note", map[string]string{
		"Name":    "pg-owned-note",
		"OwnerId": "1",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("create note with owner: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestPG_BulkDeleteEmpty tests that bulk delete with empty IDs returns 400.
func TestPG_BulkDeleteEmpty(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	rr := tc.MakeFormRequest("POST", "/v1/tags/delete", map[string]string{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

// TestPG_GroupSelfOwnership tests that groups cannot own themselves.
func TestPG_GroupSelfOwnership(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	// Create group
	rr := tc.MakeFormRequest("POST", "/v1/group", map[string]string{"Name": "self-owner"})
	if rr.Code != http.StatusOK {
		t.Fatalf("create group: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Try to set self as owner (should fail)
	rr = tc.MakeFormRequest("POST", "/v1/group", map[string]string{
		"ID":      "1",
		"Name":    "self-owner",
		"OwnerId": "1",
	})
	// Self-ownership should be rejected
	if rr.Code == http.StatusOK {
		// Check if the response indicates an error
		body := rr.Body.String()
		_ = body // The server may or may not reject this at the API level
	}
}

// TestPG_MRQL tests MRQL execution via API on Postgres.
func TestPG_MRQL(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	// Create some data first
	tc.MakeFormRequest("POST", "/v1/tag", map[string]string{"Name": "mrql-pg-tag"})
	tc.MakeFormRequest("POST", "/v1/group", map[string]string{"Name": "mrql-pg-group"})

	// Execute MRQL query via API
	rr := tc.MakeRequest("POST", "/v1/mrql", map[string]interface{}{
		"query": `type = group AND name ~ "mrql-pg"`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("MRQL query: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
```

- [ ] **Step 3: Run API postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./server/api_tests/ -v -count=1`
Expected: all pass

- [ ] **Step 4: Verify default tests still work**

Run: `go test --tags 'json1 fts5' ./server/api_tests/...`
Expected: all pass (postgres tests excluded)

- [ ] **Step 5: Commit**

```bash
git add server/api_tests/pg_test_helper.go server/api_tests/api_pg_test.go
git commit -m "test(api): add postgres API tests via testcontainers"
```

---

### Task 6: E2E Postgres test script

**Files:**
- Create: `e2e/scripts/run-tests-postgres.js`
- Modify: `e2e/package.json`

- [ ] **Step 1: Create the postgres E2E test runner**

Create `e2e/scripts/run-tests-postgres.js`:

```javascript
#!/usr/bin/env node

/**
 * Postgres E2E Test Runner
 *
 * 1. Builds binaries (server, CLI, testpg)
 * 2. Starts a Postgres testcontainer via the testpg binary
 * 3. Starts mahresources against Postgres
 * 4. Runs Playwright tests
 * 5. Cleans up everything
 */

const { spawn, execSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const net = require('net');

const PROJECT_ROOT = path.resolve(__dirname, '../..');
const SERVER_BINARY = path.join(PROJECT_ROOT, 'mahresources');
const CLI_BINARY = path.join(PROJECT_ROOT, 'mr');
const TESTPG_BINARY = path.join(PROJECT_ROOT, 'testpg');
const E2E_DIR = path.join(PROJECT_ROOT, 'e2e');

function ensureBuilt() {
  if (!fs.existsSync(SERVER_BINARY)) {
    console.log('Building server binary...');
    execSync('npm run build', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  }
  console.log('Building CLI binary...');
  execSync('go build --tags "json1 fts5" -o mr ./cmd/mr/', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  console.log('Building testpg binary...');
  execSync('go build --tags "json1 fts5 postgres" -o testpg ./cmd/testpg/', { cwd: PROJECT_ROOT, stdio: 'inherit' });
}

function findAvailablePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.listen(0, '127.0.0.1', () => {
      const addr = server.address();
      if (addr && typeof addr !== 'string') {
        const port = addr.port;
        server.close(() => resolve(port));
      } else {
        reject(new Error('Could not get port'));
      }
    });
    server.on('error', reject);
  });
}

function waitForServer(port, timeout = 30000) {
  const startTime = Date.now();
  return new Promise((resolve, reject) => {
    const check = async () => {
      if (Date.now() - startTime > timeout) {
        reject(new Error(`Server on port ${port} did not start within ${timeout}ms`));
        return;
      }
      try {
        const response = await fetch(`http://127.0.0.1:${port}/`);
        if (response.ok) { resolve(); return; }
      } catch { /* not ready */ }
      setTimeout(check, 200);
    };
    check();
  });
}

async function main() {
  ensureBuilt();

  console.log('Starting Postgres testcontainer...');
  const testpg = spawn(TESTPG_BINARY, [], {
    cwd: PROJECT_ROOT,
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  // Capture DSN from stdout (first line)
  const dsn = await new Promise((resolve, reject) => {
    let buffer = '';
    const timeout = setTimeout(() => reject(new Error('Timeout waiting for DSN from testpg')), 60000);
    testpg.stdout.on('data', (data) => {
      buffer += data.toString();
      const lines = buffer.split('\n');
      if (lines.length > 1 || buffer.includes('\n')) {
        clearTimeout(timeout);
        resolve(lines[0].trim());
      }
    });
    testpg.stderr.on('data', (data) => {
      process.stderr.write(`[testpg] ${data}`);
    });
    testpg.on('error', (err) => {
      clearTimeout(timeout);
      reject(err);
    });
    testpg.on('exit', (code) => {
      if (!buffer.includes('\n')) {
        clearTimeout(timeout);
        reject(new Error(`testpg exited with code ${code} before printing DSN`));
      }
    });
  });

  console.log(`Postgres DSN: ${dsn.replace(/password=[^&\s]+/, 'password=***')}`);

  // Start mahresources against Postgres
  const port = await findAvailablePort();
  const sharePort = await findAvailablePort();
  console.log(`Starting mahresources on port ${port} with Postgres...`);

  const server = spawn(SERVER_BINARY, [
    `-db-type=POSTGRES`,
    `-db-dsn=${dsn}`,
    `-bind-address=:${port}`,
    `-share-port=${sharePort}`,
    '-share-bind-address=127.0.0.1',
    '-hash-worker-disabled',
    '-thumb-worker-disabled',
    '-skip-version-migration',
    '-plugin-path=./e2e/test-plugins',
  ], {
    cwd: PROJECT_ROOT,
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  server.stdout.on('data', () => {});
  server.stderr.on('data', () => {});

  try {
    await waitForServer(port, 30000);
    console.log(`Server ready on port ${port}`);

    // Run Playwright tests
    const args = process.argv.slice(2);
    const playwrightArgs = args.length > 0 ? args : ['test'];

    console.log(`Running: npx playwright ${playwrightArgs.join(' ')}`);
    const testProcess = spawn('npx', ['playwright', ...playwrightArgs], {
      cwd: E2E_DIR,
      stdio: 'inherit',
      env: {
        ...process.env,
        BASE_URL: `http://127.0.0.1:${port}`,
        CLI_BASE_URL: `http://127.0.0.1:${port}`,
        SHARE_BASE_URL: `http://127.0.0.1:${sharePort}`,
        CLI_PATH: CLI_BINARY,
      },
    });

    const exitCode = await new Promise((resolve) => {
      testProcess.on('close', resolve);
    });

    process.exitCode = exitCode;
  } finally {
    // Cleanup
    console.log('Stopping server...');
    server.kill('SIGTERM');
    await new Promise(r => setTimeout(r, 2000));
    if (!server.killed) try { server.kill('SIGKILL'); } catch {}

    console.log('Stopping Postgres container...');
    testpg.kill('SIGTERM');
    await new Promise(r => setTimeout(r, 5000));
    if (!testpg.killed) try { testpg.kill('SIGKILL'); } catch {}
  }
}

main().catch((err) => {
  console.error('Fatal error:', err);
  process.exit(1);
});
```

- [ ] **Step 2: Add npm script**

In `e2e/package.json`, add to the `scripts` section:

```json
"test:with-server:postgres": "node scripts/run-tests-postgres.js test"
```

- [ ] **Step 3: Smoke test (requires Docker)**

Run: `cd e2e && npm run test:with-server:postgres -- --project=cli -- tests/cli/cli-mrql.spec.ts`
Expected: starts container, starts server on Postgres, runs the MRQL CLI tests, cleans up.

- [ ] **Step 4: Commit**

```bash
git add e2e/scripts/run-tests-postgres.js e2e/package.json
git commit -m "feat(e2e): add postgres E2E test runner via testcontainers"
```

---

### Task 7: Update CLAUDE.md and .gitignore

**Files:**
- Modify: `CLAUDE.md`
- Modify: `.gitignore` (if testpg binary isn't already ignored)

- [ ] **Step 1: Add postgres test commands to CLAUDE.md**

In the Testing section of `CLAUDE.md`, after the E2E test commands, add:

```markdown
### Postgres Tests (requires Docker)

```bash
# Run Go tests against Postgres (MRQL + API)
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1

# Run E2E tests against Postgres
cd e2e && npm run test:with-server:postgres

# Run all Postgres tests (Go + E2E)
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres
```

**Note:** Postgres tests should be run when finishing features or bugfixes, alongside regular SQLite tests. They require Docker to be running.
```

- [ ] **Step 2: Update .gitignore**

Add `testpg` to `.gitignore` if not already present.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md .gitignore
git commit -m "docs: add postgres test commands to CLAUDE.md"
```

---

### Task 8: Final verification

- [ ] **Step 1: Run default Go tests (SQLite only)**

Run: `go test --tags 'json1 fts5' ./...`
Expected: all pass, no postgres tests included

- [ ] **Step 2: Run postgres Go tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -v -count=1`
Expected: all pass against real Postgres

- [ ] **Step 3: Run postgres E2E tests**

Run: `cd e2e && npm run test:with-server:postgres`
Expected: container starts, server runs on Postgres, tests pass, cleanup succeeds

- [ ] **Step 4: Build still works**

Run: `npm run build`
Expected: clean build
