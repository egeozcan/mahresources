package application_context

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
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
