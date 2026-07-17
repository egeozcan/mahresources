package api_handlers

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/shortcodes"
)

func TestBuildMRQLAPIRenderContextAddsBudgetAndDeadline(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	appCtx := application_context.NewMahresourcesContext(nil, db, sqlx.NewDb(sqlDB, "sqlite3"), &application_context.MahresourcesConfig{
		DbType:              constants.DbTypeSqlite,
		MRQLPageQueryBudget: 7,
	})
	started := time.Now()
	reqCtx, cancel := buildMRQLAPIRenderContext(context.Background(), appCtx, false)
	defer cancel()
	budget := shortcodes.QueryBudgetFrom(reqCtx)
	if budget == nil || budget.Limit() != 7 {
		t.Fatalf("query budget = %#v, want limit 7", budget)
	}
	deadline, ok := reqCtx.Deadline()
	if !ok {
		t.Fatal("render context has no deadline")
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > appCtx.MRQLQueryTimeout() || deadline.Before(started) {
		t.Fatalf("unexpected render deadline: %v (remaining %v)", deadline, remaining)
	}
}

func TestRenderMRQLCustomTemplatesStopsOnCanceledContext(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	appCtx := application_context.NewMahresourcesContext(nil, db, sqlx.NewDb(sqlDB, "sqlite3"), &application_context.MahresourcesConfig{DbType: constants.DbTypeSqlite})
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	result := &application_context.MRQLResult{Resources: []models.Resource{{
		ResourceCategory: &models.ResourceCategory{CustomMRQLResult: "card"},
	}}}
	if err := renderMRQLCustomTemplates(appCtx, result, parent); !errors.Is(err, context.Canceled) {
		t.Fatalf("render error = %v, want context canceled", err)
	}
}

func TestRenderMRQLCustomTemplatesBatchesCarrierQueries(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ResourceCategory{}, &models.Resource{}); err != nil {
		t.Fatal(err)
	}
	category := models.ResourceCategory{Name: "batch", CustomMRQLResult: "card"}
	if err := db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	appCtx := application_context.NewMahresourcesContext(nil, db, sqlx.NewDb(sqlDB, "sqlite3"), &application_context.MahresourcesConfig{DbType: constants.DbTypeSqlite})
	result := &application_context.MRQLResult{Resources: make([]models.Resource, 100)}
	for i := range result.Resources {
		result.Resources[i].ResourceCategoryId = category.ID
	}
	var queries atomic.Int64
	callback := "test:count_api_mrql_render_queries"
	if err := db.Callback().Query().Before("gorm:query").Register(callback, func(*gorm.DB) { queries.Add(1) }); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(callback)

	if err := renderMRQLCustomTemplates(appCtx, result, context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := queries.Load(); got != 1 {
		t.Fatalf("render queries = %d, want one batched carrier query", got)
	}
}
