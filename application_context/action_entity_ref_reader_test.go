//go:build json1 && fts5

package application_context

import (
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
)

// createIsolatedTestContext opens a private in-memory SQLite database (not shared
// with other tests) and returns a fully migrated MahresourcesContext. Use this
// when a test creates a large number of rows that would pollute the shared
// file::memory:?cache=shared instance used by createTestContext.
func createIsolatedTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("createIsolatedTestContext: open db: %v", err)
	}
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
		&models.ResourceSimilarity{},
		&models.LogEntry{},
		&models.ResourceCategory{},
		&models.Series{},
		&models.NoteBlock{},
		&models.PluginKV{},
	)
	if err != nil {
		t.Fatalf("createIsolatedTestContext: migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnlyDB, &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)
	return ctx
}

// createResourceWithType inserts a Resource with the given name and content type
// directly into the test DB and returns the created model.
func createResourceWithType(t *testing.T, ctx *MahresourcesContext, name, contentType string) *models.Resource {
	t.Helper()
	r := &models.Resource{
		Name:        name,
		ContentType: contentType,
	}
	if err := ctx.db.Create(r).Error; err != nil {
		t.Fatalf("createResourceWithType(%q, %q): %v", name, contentType, err)
	}
	return r
}

// createCategory inserts a Category with the given name and returns it.
func createCategory(t *testing.T, ctx *MahresourcesContext, name string) *models.Category {
	t.Helper()
	cat, err := ctx.CreateCategory(&query_models.CategoryCreator{Name: name})
	if err != nil {
		t.Fatalf("createCategory(%q): %v", name, err)
	}
	return cat
}

// createGroupWithCategory inserts a Group optionally linked to a category.
// Pass categoryID=0 for no category.
func createGroupWithCategory(t *testing.T, ctx *MahresourcesContext, name string, categoryID uint) *models.Group {
	t.Helper()
	g, err := ctx.CreateGroup(&query_models.GroupCreator{
		Name:       name,
		CategoryId: categoryID,
	})
	if err != nil {
		t.Fatalf("createGroupWithCategory(%q, cat=%d): %v", name, categoryID, err)
	}
	return g
}

func TestActionEntityRefReader_ResourcesMatching_FiltersByContentType(t *testing.T) {
	ctx := createIsolatedTestContext(t)
	r1 := createResourceWithType(t, ctx, "a.png", "image/png")
	r2 := createResourceWithType(t, ctx, "b.jpg", "image/jpeg")
	r3 := createResourceWithType(t, ctx, "c.pdf", "application/pdf")

	reader := NewActionEntityRefReader(ctx)
	matched, err := reader.ResourcesMatching(
		[]uint{r1.ID, r2.ID, r3.ID},
		plugin_system.ActionFilter{ContentTypes: []string{"image/png", "image/jpeg"}},
	)
	if err != nil {
		t.Fatalf("ResourcesMatching: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %d (%v)", len(matched), matched)
	}
}

func TestActionEntityRefReader_GroupsMatching_FiltersByCategory(t *testing.T) {
	ctx := createIsolatedTestContext(t)
	cat := createCategory(t, ctx, "Cat A")
	g1 := createGroupWithCategory(t, ctx, "G1", cat.ID)
	g2 := createGroupWithCategory(t, ctx, "G2", 0) // no category
	g3 := createGroupWithCategory(t, ctx, "G3", cat.ID)

	reader := NewActionEntityRefReader(ctx)
	matched, err := reader.GroupsMatching(
		[]uint{g1.ID, g2.ID, g3.ID},
		plugin_system.ActionFilter{CategoryIDs: []uint{cat.ID}},
	)
	if err != nil {
		t.Fatalf("GroupsMatching: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %d (%v)", len(matched), matched)
	}
}

func TestActionEntityRefReader_Chunking(t *testing.T) {
	// Use an isolated DB so 600 resources don't pollute the shared in-memory DB.
	ctx := createIsolatedTestContext(t)
	var ids []uint
	for i := 0; i < 600; i++ {
		r := createResourceWithType(t, ctx, fmt.Sprintf("r%d.png", i), "image/png")
		ids = append(ids, r.ID)
	}
	reader := NewActionEntityRefReader(ctx)
	matched, err := reader.ResourcesMatching(ids, plugin_system.ActionFilter{})
	if err != nil {
		t.Fatalf("chunked query failed: %v", err)
	}
	if len(matched) != 600 {
		t.Fatalf("expected 600 matches across chunks, got %d", len(matched))
	}
}
