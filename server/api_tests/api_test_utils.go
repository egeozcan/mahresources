package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/jobs"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/jmoiron/sqlx"
	"mahresources/models"
	"mahresources/models/seed"
	"mahresources/server"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// openTestDatabase opens a fresh SQLite database for one test: a WAL file in the
// test's own temporary directory, which is the shape production's -memory-db mode
// uses too.
//
// Not a shared-cache in-memory database, which is what this was. Shared cache
// takes table-level locks, and a connection that meets one gets SQLITE_LOCKED
// ("database table is locked") immediately: busy_timeout never waits on it, and
// only the sqlite_unlock_notify build tag would. Background Job work (a claim
// scan, an event append, an import parse publishing its plan) runs on its own
// connection beside the request that started it, so tests of that work failed
// a few runs in fifteen on a lock production cannot raise. A WAL file has the
// production semantics: readers never block the writer, and writers wait out
// busy_timeout.
//
// It is also not a private-cache in-memory database, where every pooled
// connection is a separate empty database and any handler that fans out over
// goroutines queries an unmigrated schema. A file is one database whichever
// connection reaches it.
//
// synchronous=OFF because nothing here needs to survive a crash. The pool is
// closed at cleanup, before the directory is removed, so a package of a
// thousand tests does not hold a thousand databases open.
func openTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(path+"?_journal_mode=WAL&_busy_timeout=10000&_synchronous=OFF"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to reach the test database's pool: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// sharedCacheDBSeq keeps every shared-cache database name distinct. Under a
// shared cache the name is a lookup key, and t.Name() repeats on every iteration
// of -count=N, so without it iteration 2 would attach to iteration 1's rows.
var sharedCacheDBSeq atomic.Uint64

// openSharedCacheTestDatabase opens the database openTestDatabase deliberately
// avoids: shared-cache in-memory SQLite, whose table locks raise SQLITE_LOCKED
// without waiting. Only a test that pins code handling that lock uses it; other
// fixtures in the tree still run on shared cache, so that handling stays live.
func openSharedCacheTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", t.Name(), sharedCacheDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open shared-cache test database: %v", err)
	}
	return db
}

// TestContext holds the application context and the router for testing
type TestContext struct {
	AppCtx *application_context.MahresourcesContext
	Router http.Handler
	DB     *gorm.DB
	// Fs is the same in-memory filesystem the context was built with. A delete or a
	// merge decides whether to back a file up and whether to remove it, and the
	// filesystem is the only place those two decisions are observable.
	Fs afero.Fs
}

// SetupTestEnv creates a fresh in-memory database and application context for each test
func SetupTestEnv(t *testing.T) *TestContext {
	return setupTestEnvWithConfig(t, nil)
}

// setupTestEnvWithConfig is the configurable core of SetupTestEnv. The optional
// mutate callback adjusts the MahresourcesConfig before the context is built,
// letting tests enable auth or tweak other settings without duplicating setup.
func setupTestEnvWithConfig(t *testing.T, mutate func(*application_context.MahresourcesConfig)) *TestContext {
	return setupTestEnvOn(t, openTestDatabase(t), mutate)
}

// setupTestEnvOn migrates, seeds and serves the database it is given.
func setupTestEnvOn(t *testing.T, db *gorm.DB, mutate func(*application_context.MahresourcesConfig)) *TestContext {
	// AutoMigrate all models (same as main.go)
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
		&models.TemplatePartial{},
		&models.RuntimeSetting{},
		&models.User{},
		&models.SavedSearch{}, &models.UserSetting{},
		&models.Session{},
		&models.ApiToken{},
		&models.DownloadHistoryEntry{},
		&models.ScheduledDownload{},
		&models.ResourceReduction{},
		&models.PluginSchedule{},
		&models.PluginCommandRun{}, &models.PluginCommandRunOutput{},
		&models.PluginCommandImport{}, &models.PluginCommandImportMap{},
		// The durable job core. Deleting a user nulls a Job's owner and actor and
		// removes the viewer-keyed preferences beside it, so these tables exist
		// wherever the suite exercises user deletion.
		&models.Job{}, &models.JobResourceReceipt{}, &models.JobEvent{}, &models.JobEventSequence{}, &models.JobLink{},
		&models.JobPreference{}, &models.JobPinGuard{}, &models.JobLegacyHandle{},
		&models.JobSourceMapping{},
		&models.JobImportCommandFact{},
		&models.PluginCommandImportCommandFact{}, &models.PluginCommandImportCommandFactGroup{},
		&models.JobOutput{}, &models.JobReplayEnvelope{}, &models.JobClaim{},
		&models.JobCapacityLease{}, &models.JobCommandRequest{}, &models.JobWriterEpoch{}, &models.JobRuntimeFence{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}
	if err := models.EnsureJobWriterEpoch(db); err != nil {
		t.Fatalf("Failed to initialize Job writer epoch: %v", err)
	}
	if err := models.EnsureSupplementalIndexes(db); err != nil {
		t.Fatalf("Failed to create supplemental indexes: %v", err)
	}

	seed.AddInitialData(db)

	config := &application_context.MahresourcesConfig{
		DbType:                       constants.DbTypeSqlite,
		FfmpegPath:                   "ffmpeg",
		BindAddress:                  ":0",
		MaxUploadSize:                2 << 30,
		MaxImportSize:                10 << 30,
		MRQLDefaultLimit:             500,
		MRQLQueryTimeoutBoot:         10 * time.Second,
		ExportRetention:              24 * time.Hour,
		RemoteResourceConnectTimeout: 30 * time.Second,
		RemoteResourceIdleTimeout:    60 * time.Second,
		RemoteResourceOverallTimeout: 30 * time.Minute,
		HashSimilarityThreshold:      10,
		HashAHashThreshold:           5,
		// The download tests point the queue at an httptest server on loopback,
		// which the host fetch policy denies by default. Declaring it here is
		// what a deployment fetching from a LAN service does too.
		AllowPrivateFetch: []string{"127.0.0.1", "::1"},
	}

	// Mock filesystem
	fs := afero.NewMemMapFs()
	// CreateServer expects map[string]string for paths, but we want to control them.
	// However, NewMahresourcesContext creates proper FS objects from them.
	// For testing, we can just pass empty or temp paths if we don't strictly test AltFileSystems logic here.
	altFsPaths := make(map[string]string)

	// We need the sqlx DB for readOnlyDB param
	// For sqlite in memory, we can just pass the underlying sql.DB if compatible or nil if not strictly used in write ops tested here
	// context.go NewMahresourcesContext takes *sqlx.DB.
	// gorm DB.DB() returns *sql.DB.
	if mutate != nil {
		mutate(config)
	}

	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")

	appCtx := application_context.NewMahresourcesContext(fs, db, readOnlyDB, config)
	appCtx.SetJobService(jobs.NewService())
	replayKey, err := jobs.GenerateReplayKey()
	if err != nil {
		t.Fatalf("Generate test Job replay key: %v", err)
	}
	replayKeyring, err := jobs.NewKeyring(replayKey)
	if err != nil {
		t.Fatalf("Initialize test Job replay keyring: %v", err)
	}
	appCtx.SetJobReplayKeyring(replayKeyring)

	// Wire runtime settings (mirrors main.go boot sequence).
	settings := application_context.NewRuntimeSettings(
		db,
		application_context.NewStdlibSettingsLogger(),
		application_context.BuildSpecsExported(),
		application_context.BuildDefaultsFromConfig(config),
	)
	_ = settings.Load()
	appCtx.SetSettings(settings)

	// Ensure default resource category exists and set the resolved ID
	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)
	appCtx.DefaultResourceCategoryID = defaultRC.ID

	// Create request handler using the actual server setup
	// CreateServer takes altFs as map[string]string
	serverInstance := server.CreateServer(appCtx, fs, altFsPaths)

	return &TestContext{
		AppCtx: appCtx,
		Router: serverInstance.Handler,
		DB:     db,
		Fs:     fs,
	}
}

// MakeRequest sends a request to the test server and returns the response
func (tc *TestContext) MakeRequest(method, url string, body interface{}) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != nil {
		jsonBytes, _ := json.Marshal(body)
		bodyReader = bytes.NewBuffer(jsonBytes)
	}

	req, _ := http.NewRequest(method, url, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	return rr
}

// MakeFormRequest sends a form-encoded request to the test server
func (tc *TestContext) MakeFormRequest(method, reqUrl string, formData url.Values) *httptest.ResponseRecorder {
	bodyReader := strings.NewReader(formData.Encode())
	req, _ := http.NewRequest(method, reqUrl, bodyReader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	return rr
}

// Helper to create a dummy note for testing
func (tc *TestContext) CreateDummyNote(name string) *models.Note {
	note := &models.Note{Name: name, Description: "Test Description"}
	tc.DB.Create(note)
	return note
}

// Helper to create a dummy group for testing
func (tc *TestContext) CreateDummyGroup(name string) *models.Group {
	group := &models.Group{Name: name, Description: "Test Group Description"}
	tc.DB.Create(group)
	return group
}

func UintPtr(v uint) *uint {
	return &v
}

// Helper to create a dummy block for testing
func (tc *TestContext) CreateDummyBlock(noteID uint, blockType, content, position string) *models.NoteBlock {
	block := &models.NoteBlock{
		NoteID:   noteID,
		Type:     blockType,
		Position: position,
		Content:  []byte(content),
		State:    []byte("{}"),
	}
	tc.DB.Create(block)
	return block
}

// CreateResourceWithType inserts a Resource with the given name and content type
// directly into the test database. It is intentionally minimal — no file bytes
// are stored — because the content-type filter operates purely on the DB column.
func (tc *TestContext) CreateResourceWithType(t *testing.T, name, contentType string) *models.Resource {
	t.Helper()
	r := &models.Resource{Name: name, ContentType: contentType}
	if err := tc.DB.Create(r).Error; err != nil {
		t.Fatalf("CreateResourceWithType: %v", err)
	}
	return r
}

// CreateNoteType inserts a NoteType with the given name directly into the test
// database and returns the created record (with its auto-assigned ID).
func (tc *TestContext) CreateNoteType(t *testing.T, name string) *models.NoteType {
	t.Helper()
	nt := &models.NoteType{Name: name}
	if err := tc.DB.Create(nt).Error; err != nil {
		t.Fatalf("CreateNoteType: %v", err)
	}
	return nt
}

// CreateNoteWithType inserts a Note with the given name and the supplied
// NoteTypeId directly into the test database.
func (tc *TestContext) CreateNoteWithType(t *testing.T, name string, noteTypeId uint) *models.Note {
	t.Helper()
	n := &models.Note{Name: name, NoteTypeId: &noteTypeId}
	if err := tc.DB.Create(n).Error; err != nil {
		t.Fatalf("CreateNoteWithType: %v", err)
	}
	return n
}

// requireJsonPatch skips the test if SQLite json_patch is not available (needs json1 build tag).
func requireJsonPatch(t *testing.T, db *gorm.DB) {
	t.Helper()
	err := db.Exec(`SELECT json_patch('{}', '{}')`).Error
	if err != nil {
		t.Skip("json_patch not available (build with -tags json1)")
	}
}
