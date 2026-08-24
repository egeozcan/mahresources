package application_context

import (
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

// newWALTestContext builds a context backed by a real on-disk SQLite database
// opened through CreateDatabaseConnection, so the "sqlite3_pragmas" driver
// applies journal_mode=WAL, synchronous=NORMAL and busy_timeout=10000 to every
// pooled connection — exactly the production configuration.
//
// This matters: TestAddResource_ConcurrentSameHash opens a shared-cache memory
// DSN with the plain sqlite driver, so it exercises neither WAL nor the busy
// timeout, and every goroutine there uploads identical bytes, which the
// per-hash idlock serializes end to end. It cannot see the defect below.
//
// The filesystem is a real OsFs over t.TempDir() rather than a MemMapFs,
// because the window this test aims at is the wall-clock duration of
// AddResource's io.Copy into the target filesystem.
func newWALTestContext(t *testing.T, maxConns int) *MahresourcesContext {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "concurrency.db")
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, dsn, "", 0)
	if err != nil {
		t.Fatalf("open WAL test database: %v", err)
	}

	if err := db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
		&models.LogEntry{},
		&models.ResourceCategory{},
		&models.Series{},
		&models.NoteBlock{},
		&models.PluginKV{},
		&models.ResourceVersion{},
	); err != nil {
		t.Fatalf("migrate WAL test database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sql.DB: %v", err)
	}
	if maxConns > 0 {
		// The e2e harness runs the server with -max-db-connections=2 to reduce
		// SQLite contention, and production defaults to unlimited. Both shapes
		// are worth covering: a constrained pool makes Begin() queue, which
		// changes which transactions overlap.
		sqlDB.SetMaxOpenConns(maxConns)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	fs := afero.NewBasePathFs(afero.NewOsFs(), t.TempDir())
	ctx := NewMahresourcesContext(fs, db, sqlx.NewDb(sqlDB, "sqlite3_pragmas"), &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})

	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)

	return ctx
}

// TestAddResource_ConcurrentDistinctHashes is the guard for the client-side
// bulk upload widget, which posts one file per request and therefore lands
// several AddResource calls on the server at once. The pre-existing shape of
// AddResource could not survive that on SQLite:
//
//	tx := ctx.db.Begin()                    // deferred — takes no lock
//	tx.Where("hash = ?", ...).First(...)    // first statement is a READ → WAL snapshot
//	io.Copy(savedFile, tempFile)            // the whole file, transaction still open
//	tx.Save(res)                            // first WRITE → the snapshot must promote
//
// In WAL, promoting a read snapshot to a write after another connection has
// committed returns SQLITE_BUSY_SNAPSHOT, and for that extended result code
// SQLite does not invoke the busy handler — so busy_timeout does not apply and
// nothing in the upload path retries it. The caller saw "database is locked",
// which the upload handler classifies as neither a conflict nor a bad request
// and reports as HTTP 500, on a file whose bytes were already written to disk.
//
// Distinct contents are essential: identical bytes would be serialized by the
// per-hash idlock and this would never overlap.
func TestAddResource_ConcurrentDistinctHashes(t *testing.T) {
	testConcurrentDistinctUploads(t, 0)
}

// TestAddResource_ConcurrentDistinctHashesConstrainedPool is the same guard
// under the connection limit the e2e harness runs with, where Begin() queues
// behind the two available connections instead of always getting one.
func TestAddResource_ConcurrentDistinctHashesConstrainedPool(t *testing.T) {
	testConcurrentDistinctUploads(t, 2)
}

func testConcurrentDistinctUploads(t *testing.T, maxConns int) {
	t.Helper()
	ctx := newWALTestContext(t, maxConns)

	ownerGroup := &models.Group{Name: "concurrent-owner"}
	if err := ctx.db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("create owner group: %v", err)
	}

	// Big enough that the io.Copy inside the transaction spans the other
	// goroutines' commits, which is what opens the window.
	const concurrency = 8
	const fileSize = 2 << 20 // 2 MiB

	payloads := make([][]byte, concurrency)
	for i := range payloads {
		buf := make([]byte, fileSize)
		if _, err := rand.Read(buf); err != nil {
			t.Fatalf("generate payload %d: %v", i, err)
		}
		payloads[i] = buf
	}

	var wg sync.WaitGroup
	errs := make([]error, concurrency)

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = ctx.AddResource(
				newBytesFile(payloads[idx]),
				fmt.Sprintf("concurrent-%d.bin", idx),
				&query_models.ResourceCreator{
					ResourceQueryBase: query_models.ResourceQueryBase{
						Name:    fmt.Sprintf("concurrent-%d", idx),
						OwnerId: ownerGroup.ID,
					},
				},
			)
		}(i)
	}
	wg.Wait()

	var failed int
	for i, err := range errs {
		if err != nil {
			failed++
			t.Errorf("upload %d failed: %v", i, err)
		}
	}
	if failed > 0 {
		t.Fatalf("%d of %d concurrent uploads of distinct files failed; every one of them should succeed", failed, concurrency)
	}

	var stored int64
	if err := ctx.db.Model(&models.Resource{}).Count(&stored).Error; err != nil {
		t.Fatalf("count resources: %v", err)
	}
	if stored != concurrency {
		t.Fatalf("expected %d resources, found %d", concurrency, stored)
	}
}

// TestUploadRetryRebuildsTheResource pins the one thing a retry of the write
// transaction must get right: it may not reuse a struct the previous attempt
// mutated.
//
// The transaction hands `res` to GORM (which stamps the id and the GUID) and to
// AssignResourceToSeries, which writes SeriesID and OwnMeta
// (series_context.go:227,243,247). A rollback removes the series row but not the
// pointer to it, so an attempt that reused the struct carried a SeriesID
// referencing nothing — and SQLite runs with PRAGMA foreign_keys = ON, so the
// retry died with "FOREIGN KEY constraint failed" rather than succeeding.
//
// The contention is injected rather than raced for: a lock this test waited to
// lose would make it a coin flip, and the interleave that matters here is
// "something after the series assignment failed", which is precisely specifiable.
func TestUploadRetryRebuildsTheResource(t *testing.T) {
	ctx := newWALTestContext(t, 0)

	ownerGroup := &models.Group{Name: "retry-owner"}
	if err := ctx.db.Create(ownerGroup).Error; err != nil {
		t.Fatalf("create owner group: %v", err)
	}

	// Fail the first resource_versions INSERT only. That statement runs after
	// GetOrCreateSeriesForResource and AssignResourceToSeries, so the attempt it
	// kills is one that has already written SeriesID onto res.
	var failures atomic.Int32
	err := ctx.db.Callback().Create().Before("gorm:create").Register(
		"test:fail_first_version_insert",
		func(db *gorm.DB) {
			if db.Statement == nil || db.Statement.Table != "resource_versions" {
				return
			}
			if failures.Add(1) == 1 {
				_ = db.AddError(errors.New("database is locked"))
			}
		},
	)
	if err != nil {
		t.Fatalf("register injection callback: %v", err)
	}
	t.Cleanup(func() {
		_ = ctx.db.Callback().Create().Remove("test:fail_first_version_insert")
	})

	payload := make([]byte, 4096)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("generate payload: %v", err)
	}

	res, err := ctx.AddResource(newBytesFile(payload), "retried.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:       "retried",
			OwnerId:    ownerGroup.ID,
			SeriesSlug: "retry-series",
		},
	})
	if err != nil {
		t.Fatalf("upload should have survived one injected lock failure, got: %v", err)
	}
	if res == nil || res.ID == 0 {
		t.Fatal("expected a persisted resource")
	}
	if failures.Load() < 2 {
		// A control: if the injection never fired twice the retry never happened
		// and this test proves nothing about it.
		t.Fatalf("expected the version insert to be attempted at least twice, saw %d", failures.Load())
	}

	// The surviving row must point at a series that actually exists.
	var stored models.Resource
	if err := ctx.db.First(&stored, res.ID).Error; err != nil {
		t.Fatalf("re-read resource: %v", err)
	}
	if stored.SeriesID == nil {
		t.Fatal("expected the retried upload to still land in its series")
	}
	var series models.Series
	if err := ctx.db.First(&series, *stored.SeriesID).Error; err != nil {
		t.Fatalf("the resource points at a series that does not exist: %v", err)
	}

	// Exactly one resource and one version, not one per attempt.
	var resources, versions int64
	ctx.db.Model(&models.Resource{}).Count(&resources)
	ctx.db.Model(&models.ResourceVersion{}).Count(&versions)
	if resources != 1 || versions != 1 {
		t.Fatalf("retry duplicated rows: %d resources, %d versions", resources, versions)
	}
}

// TestUploadPanicIsNotReportedAsSuccess pins the named return on
// insertUploadedResource's recover.
//
// The recover exists to guarantee the rollback. In a function with unnamed
// results a bare recover also returns the zero error, so a panicking callback
// rolled the row back and then reported success: AddResource logged the create,
// fired after_resource_create, and handed back a resource whose row was gone.
func TestUploadPanicIsNotReportedAsSuccess(t *testing.T) {
	ctx := newWALTestContext(t, 0)

	err := ctx.db.Callback().Create().Before("gorm:create").Register(
		"test:panic_on_version_insert",
		func(db *gorm.DB) {
			if db.Statement != nil && db.Statement.Table == "resource_versions" {
				panic("injected callback panic")
			}
		},
	)
	if err != nil {
		t.Fatalf("register injection callback: %v", err)
	}
	t.Cleanup(func() {
		_ = ctx.db.Callback().Create().Remove("test:panic_on_version_insert")
	})

	payload := make([]byte, 2048)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("generate payload: %v", err)
	}

	res, err := ctx.AddResource(newBytesFile(payload), "panics.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "panics"},
	})
	if err == nil {
		t.Fatalf("a panic mid-transaction must not be reported as success; got resource %+v", res)
	}
	if res != nil {
		t.Fatalf("expected no resource alongside the error, got %+v", res)
	}

	// And the rollback actually happened.
	var stored int64
	if err := ctx.db.Model(&models.Resource{}).Count(&stored).Error; err != nil {
		t.Fatalf("count resources: %v", err)
	}
	if stored != 0 {
		t.Fatalf("expected the rolled-back row to be gone, found %d resources", stored)
	}
}

// TestHashLookupFailureIsNotTreatedAsNewContent pins the dedup lookup's error
// handling.
//
// The read that asks "does a resource with this hash already exist" used to
// treat every error as "no". Resource.Hash carries a plain index, not a unique
// one, so falling through on a transient failure persists a second row for
// content that already exists — silently breaking the invariant the per-hash
// idlock exists to protect. Sequential uploads made such a failure vanishingly
// unlikely; concurrent ones do not.
func TestHashLookupFailureIsNotTreatedAsNewContent(t *testing.T) {
	ctx := newWALTestContext(t, 0)

	payload := make([]byte, 2048)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("generate payload: %v", err)
	}

	// One good upload, so there is a row the lookup would have found.
	if _, err := ctx.AddResource(newBytesFile(payload), "original.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "original"},
	}); err != nil {
		t.Fatalf("seed upload: %v", err)
	}

	// Now break only that lookup.
	err := ctx.db.Callback().Query().Before("gorm:query").Register(
		"test:fail_hash_lookup",
		func(db *gorm.DB) {
			if db.Statement != nil && db.Statement.Table == "resources" {
				_ = db.AddError(errors.New("database is locked"))
			}
		},
	)
	if err != nil {
		t.Fatalf("register injection callback: %v", err)
	}
	t.Cleanup(func() {
		_ = ctx.db.Callback().Query().Remove("test:fail_hash_lookup")
	})

	_, err = ctx.AddResource(newBytesFile(payload), "duplicate.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "duplicate"},
	})
	if err == nil {
		t.Fatal("a failed dedup lookup must surface as an error, not as new content")
	}

	_ = ctx.db.Callback().Query().Remove("test:fail_hash_lookup")

	var stored int64
	if countErr := ctx.db.Model(&models.Resource{}).Where("hash IS NOT NULL AND hash != ''").Count(&stored).Error; countErr != nil {
		t.Fatalf("count resources: %v", countErr)
	}
	if stored != 1 {
		t.Fatalf("expected the content to still exist exactly once, found %d rows", stored)
	}
}
