package groupio

import (
	"testing"

	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/models"
)

// Test fixtures for the moved white-box tests.
//
// The fixture hands back an *opCtx rather than a (Service, Deps) pair, because
// that is what the moved tests were already written against: ctx.db, ctx.fs,
// ctx.buildExportPlan, ctx.ensureGUID, ctx.StreamExport and friends all resolve
// exactly as before, so the port needed no edits to test bodies.
//
// Building an opCtx directly is safe here and only here. Production must go
// through Service + Deps so the handle is bound per call; a test that owns its
// DB for the whole test has no derived handle to miss.

// testDefaultResourceCategoryID mirrors the DefaultResourceCategoryID: 1 that
// NewMahresourcesContext hardcodes, which the moved tests relied on.
const testDefaultResourceCategoryID uint = 1

// testScope is an unrestricted ScopeResolver. None of the moved tests exercises
// a scoped principal — the scope and confinement tests live in
// application_context, where WithPrincipal and the GORM callbacks are
// reachable — so allow-all matches the unscoped context these tests always had.
type testScope struct{ db *gorm.DB }

func (s testScope) SubtreeGroupIDs(rootID uint) ([]uint, error) {
	var ids []uint
	err := s.db.Raw(`
		WITH RECURSIVE tree AS (
			SELECT id FROM groups WHERE id = ?
			UNION
			SELECT g.id FROM groups g JOIN tree ON g.owner_id = tree.id
		)
		SELECT id FROM tree
		LIMIT 1000000
	`, rootID).Scan(&ids).Error
	return ids, err
}

func (s testScope) VisibleGroupIDs(ids []uint) map[uint]bool {
	visible := make(map[uint]bool, len(ids))
	for _, id := range ids {
		visible[id] = true
	}
	return visible
}

// migrateTestSchema covers every model the export and import paths touch.
// Hand-maintained, as its application_context ancestor was: add to it when a
// new model enters the import path.
func migrateTestSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.ResourceVersion{},
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
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// newTestCtx wires a DB and filesystem into an opCtx and seeds the default
// resource category.
func newTestCtx(t *testing.T, db *gorm.DB, fs afero.Fs, alt map[string]afero.Fs) *opCtx {
	t.Helper()
	if alt == nil {
		alt = map[string]afero.Fs{}
	}
	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = testDefaultResourceCategoryID
	db.FirstOrCreate(defaultRC, testDefaultResourceCategoryID)

	return NewService(fs, alt).op(Deps{DB: db, Scope: testScope{db: db}})
}

// createTestContext is the port of application_context's helper of the same
// name, which the moved tests call in 50 places.
//
// The DSN keeps cache=shared deliberately: every context created this way
// shares one SQLite database, and several moved tests depend on that — the
// full-round-trip tests export from one context and import into another,
// resolving GUID and hash collisions across them.
func createTestContext(t *testing.T) *opCtx {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	migrateTestSchema(t, db)
	return newTestCtx(t, db, afero.NewMemMapFs(), nil)
}
