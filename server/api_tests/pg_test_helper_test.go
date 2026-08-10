//go:build postgres

package api_tests

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/spf13/afero"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/internal/testpgutil"
	"mahresources/models"
	"mahresources/models/seed"
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
	db, testDSN := pgContainer.CreateTestDBWithDSN(t)

	err := db.AutoMigrate(
		&models.Query{},
		&models.Series{},
		&models.Tag{},
		&models.Category{},
		&models.ResourceCategory{},
		&models.NoteType{},
		&models.LogEntry{},
		&models.PluginState{},
		&models.PluginKV{},
		&models.SavedMRQLQuery{},
		&models.TemplatePartial{},
		&models.Group{},
		&models.GroupRelationType{},
		&models.Resource{},
		&models.Note{},
		&models.ResourceVersion{},
		&models.NoteBlock{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
		&models.User{},
		&models.UserSetting{},
		&models.Session{},
		&models.ApiToken{},
		&models.DownloadHistoryEntry{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	if err := models.EnsureSupplementalIndexes(db); err != nil {
		t.Fatalf("Failed to create supplemental indexes: %v", err)
	}
	seed.AddInitialData(db)

	config := &application_context.MahresourcesConfig{
		DbType:      constants.DbTypePosgres,
		BindAddress: ":0",
	}

	fs := afero.NewMemMapFs()
	altFsPaths := make(map[string]string)

	// Open the read-only handle the way production does — through
	// models.CreateReadOnlyDatabaseConnection, i.e. lib/pq — rather than wrapping
	// GORM's pgx handle. Saved queries are the only thing that runs on this
	// connection, and the two drivers return different Go types for the same
	// column: with the pgx handle a numeric, uuid or array column arrives as a
	// string and with lib/pq it arrives as []byte. Reusing the GORM handle made
	// every value-encoding defect in POST /v1/query/run invisible to this suite.
	readOnlyDB, err := models.CreateReadOnlyDatabaseConnection("postgres", testDSN)
	if err != nil {
		t.Fatalf("Failed to open the read-only connection: %v", err)
	}
	t.Cleanup(func() { readOnlyDB.Close() })

	appCtx := application_context.NewMahresourcesContext(fs, db, readOnlyDB, config)

	// Ensure default resource category exists
	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)
	appCtx.DefaultResourceCategoryID = defaultRC.ID

	serverInstance := server.CreateServer(appCtx, fs, altFsPaths)

	return &TestContext{
		AppCtx: appCtx,
		Router: serverInstance.Handler,
		DB:     db,
	}
}
