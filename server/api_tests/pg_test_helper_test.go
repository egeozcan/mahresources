//go:build postgres

package api_tests

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"

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
