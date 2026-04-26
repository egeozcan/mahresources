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

	"github.com/jmoiron/sqlx"
	"mahresources/models"
	"mahresources/models/util"
	"mahresources/server"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestContext holds the application context and the router for testing
type TestContext struct {
	AppCtx *application_context.MahresourcesContext
	Router http.Handler
	DB     *gorm.DB
}

// SetupTestEnv creates a fresh in-memory database and application context for each test
func SetupTestEnv(t *testing.T) *TestContext {
	// Use unique in-memory SQLite database per test to avoid interference
	// The test name is sanitized to create a unique database name
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
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
		&models.RuntimeSetting{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	util.AddInitialData(db)

	config := &application_context.MahresourcesConfig{
		DbType:                       constants.DbTypeSqlite,
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
