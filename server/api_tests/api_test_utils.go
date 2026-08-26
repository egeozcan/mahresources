package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mahresources/application_context"
	"mahresources/constants"
	"net/url"
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

// testDBSeq makes every database name in this package distinct, whatever the test
// name is. See the DSN comment in setupTestEnvWithConfig for why that matters under
// a shared cache; it also covers the five tests that call the setup helper twice.
var testDBSeq atomic.Uint64

func nextTestDBSeq() uint64 { return testDBSeq.Add(1) }

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
	// One in-memory SQLite database per test, keyed on the test name so tests never
	// see each other's rows.
	//
	// `cache=shared` and not `cache=private`, which is what this was and which was
	// quietly wrong. Under a private cache the name is decoration: *every pooled
	// connection gets its own brand-new empty database*. A handler that runs on one
	// connection sees the migrated, seeded DB; anything that fans out over
	// goroutines has the rest of them opening fresh connections into empty
	// databases and logging `no such table: …`.
	//
	// Three fan-out sites are affected. The dashboard provider runs five goroutines
	// (dashboard_template_context.go), GetDataStats fifteen, and global search one
	// per entity type. Measured effect before the change:
	// TestDashboardTimeAttributeIsARealInstant took the "rendered no <time datetime>
	// elements" branch and t.Skip'd in **12 of 20** separate runs — and a skip
	// scores green, so a guard in the one suite CI runs was silently not running,
	// most of the time.
	//
	// Nothing holds a connection open deliberately, and nothing needs to: Go keeps
	// up to MaxIdleConns (default 2) connections alive with no idle timeout, and
	// gorm.Open pings, so a connection exists from open until the pool is closed —
	// which nothing in this package does. Deliberately NOT an explicit
	// sqlDB.Conn() keepalive: seventeen tests pin the pool with
	// SetMaxOpenConns(1), and checking out the single permitted connection would
	// deadlock every one of them on a context.Background() wait.
	//
	// The sequence number is what `cache=private` used to give for free. Under a
	// shared cache the name is a real lookup key, and `t.Name()` is identical on
	// every iteration of `go test -count=N` — so iterations 2..N would attach to
	// iteration 1's database, still holding its rows. That is not hypothetical: with
	// the name alone, `-count=3` over four of this package's tests failed, and the
	// same four passed under `cache=private`. Measured before shipping the change.
	dbName := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", t.Name(), nextTestDBSeq())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// AutoMigrate all models (same as main.go)
	err = db.AutoMigrate(
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
		&models.UserSetting{},
		&models.Session{},
		&models.ApiToken{},
		&models.DownloadHistoryEntry{},
		&models.ResourceReduction{},
		&models.PluginSchedule{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
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
