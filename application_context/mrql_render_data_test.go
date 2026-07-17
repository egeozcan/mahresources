package application_context

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/jmoiron/sqlx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
)

func setupMRQLRenderDataTest(t *testing.T) (*MahresourcesContext, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Category{}, &models.ResourceCategory{}, &models.NoteType{}, &models.Group{}, &models.Resource{}, &models.Note{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	ctx := NewMahresourcesContext(nil, db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{DbType: constants.DbTypeSqlite})
	return ctx, db
}

func TestLoadMRQLRenderDataBatchesScopesAndCaches(t *testing.T) {
	ctx, db := setupMRQLRenderDataTest(t)
	root := &models.Group{Name: "root"}
	if err := db.Create(root).Error; err != nil {
		t.Fatal(err)
	}
	rootID := root.ID
	child := &models.Group{Name: "child", OwnerId: &rootID}
	if err := db.Create(child).Error; err != nil {
		t.Fatal(err)
	}
	noteType := &models.NoteType{Name: "typed", CustomMRQLResult: "card"}
	if err := db.Create(noteType).Error; err != nil {
		t.Fatal(err)
	}

	var queries atomic.Int64
	callbackName := "test:count_mrql_render_queries"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) { queries.Add(1) }); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(callbackName)

	reqCtx := WithMRQLRenderDataCache(context.Background())
	data, err := ctx.LoadMRQLRenderData(reqCtx, nil, []uint{noteType.ID, noteType.ID}, nil, []uint{child.ID, child.ID})
	if err != nil {
		t.Fatal(err)
	}
	if data.NoteTypes[noteType.ID] == nil || data.NoteTypes[noteType.ID].CustomMRQLResult != "card" {
		t.Fatalf("note type not loaded: %#v", data.NoteTypes)
	}
	if len(data.NoteTypes[noteType.ID].Notes) != 0 {
		t.Fatal("batch carrier unexpectedly preloaded associations")
	}
	scope := data.Scopes[child.ID]
	if scope.ParentGroupID != root.ID || scope.RootGroupID != root.ID {
		t.Fatalf("scope = %#v, want parent/root %d", scope, root.ID)
	}
	firstCount := queries.Load()
	if firstCount > 2 {
		t.Fatalf("first load used %d queries, want at most note type + ancestry", firstCount)
	}
	if _, err := ctx.LoadMRQLRenderData(reqCtx, nil, []uint{noteType.ID}, nil, []uint{child.ID}); err != nil {
		t.Fatal(err)
	}
	if queries.Load() != firstCount {
		t.Fatalf("cache hit issued queries: before=%d after=%d", firstCount, queries.Load())
	}
}
