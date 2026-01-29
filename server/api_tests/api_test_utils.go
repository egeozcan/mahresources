package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mahresources/application_context"
	"mahresources/constants"

	"github.com/jmoiron/sqlx"
	"mahresources/models"
	"mahresources/models/util"
	"mahresources/server"
	"net/http"
	"net/http/httptest"
	"testing"

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
		&models.LogEntry{},
		&models.NoteBlock{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	util.AddInitialData(db)

	config := &application_context.MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
		BindAddress: ":0",
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

// Add more helpers as needed...
