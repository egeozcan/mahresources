package template_filters

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/util"
	"mahresources/shortcodes"
)

func setupExecutorTestContext(t *testing.T) (*application_context.MahresourcesContext, *gorm.DB) {
	t.Helper()
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Query{}, &models.Resource{}, &models.Note{}, &models.Tag{},
		&models.Group{}, &models.Category{}, &models.NoteType{}, &models.Preview{},
		&models.GroupRelation{}, &models.GroupRelationType{}, &models.ImageHash{},
		&models.ResourceSimilarity{}, &models.LogEntry{}, &models.NoteBlock{},
		&models.SavedMRQLQuery{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	util.AddInitialData(db)
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	ctx := application_context.NewMahresourcesContext(nil, db, readOnlyDB, &application_context.MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
	return ctx, db
}

// Regression: an inline query carrying a scope must splice SCOPE at the
// grammatically correct position (before LIMIT/ORDER BY/etc.), not append it at
// the end where MRQL would reject it. The result must round-trip through Parse.
func TestExecutorLinkScopeInsertedBeforeTrailingClauses(t *testing.T) {
	ctx, db := setupExecutorTestContext(t)
	scope := &models.Group{Name: "scope-grp"}
	if err := db.Create(scope).Error; err != nil {
		t.Fatalf("seed group: %v", err)
	}
	exec := BuildQueryExecutor(ctx)

	res, err := exec(context.Background(), `type = "resource" ORDER BY name LIMIT 5`, shortcodes.QueryOptions{
		ScopeGroupID: scope.ID,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	want := fmt.Sprintf(`type = "resource" SCOPE %d ORDER BY name LIMIT 5`, scope.ID)
	if res.EffectiveQuery != want {
		t.Fatalf("EffectiveQuery = %q, want %q", res.EffectiveQuery, want)
	}
}

// Regression: a scoped saved query must not link via ?saved=<id> (which opens
// globally) — the executor clears SavedID and bakes the scope into the query
// text so the view-all link reproduces the scoped result set.
func TestExecutorScopedSavedQueryDropsSavedID(t *testing.T) {
	ctx, db := setupExecutorTestContext(t)
	scope := &models.Group{Name: "saved-scope-grp"}
	if err := db.Create(scope).Error; err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := ctx.CreateSavedMRQLQuery("rep", `type = "resource" LIMIT 3`, ""); err != nil {
		t.Fatalf("create saved: %v", err)
	}
	exec := BuildQueryExecutor(ctx)

	res, err := exec(context.Background(), "", shortcodes.QueryOptions{
		SavedName:    "rep",
		ScopeGroupID: scope.ID,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if res.SavedID != 0 {
		t.Fatalf("scoped saved query must clear SavedID, got %d", res.SavedID)
	}
	want := fmt.Sprintf(`type = "resource" SCOPE %d LIMIT 3`, scope.ID)
	if res.EffectiveQuery != want {
		t.Fatalf("EffectiveQuery = %q, want %q", res.EffectiveQuery, want)
	}
}

// An unscoped saved query keeps its saved identity (?saved=<id>).
func TestExecutorUnscopedSavedQueryKeepsSavedID(t *testing.T) {
	ctx, _ := setupExecutorTestContext(t)
	saved, err := ctx.CreateSavedMRQLQuery("rep2", `type = "resource"`, "")
	if err != nil {
		t.Fatalf("create saved: %v", err)
	}
	exec := BuildQueryExecutor(ctx)

	res, err := exec(context.Background(), "", shortcodes.QueryOptions{SavedName: "rep2"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if res.SavedID != saved.ID {
		t.Fatalf("unscoped saved query should keep SavedID %d, got %d", saved.ID, res.SavedID)
	}
}
