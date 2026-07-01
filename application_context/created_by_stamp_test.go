package application_context

import (
	"fmt"
	"reflect"
	"testing"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newStampTestContext opens a private in-memory SQLite DB migrated with the
// stamped content models plus the auth models, and returns a context in the
// requested auth mode. Mirrors newAuthTestContext but with the broader model set
// needed to exercise CreatedByUserId stamping.
func newStampTestContext(t *testing.T, authEnabled bool) *MahresourcesContext {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Query{},
		&models.Series{},
		&models.Tag{},
		&models.Category{},
		&models.ResourceCategory{},
		&models.NoteType{},
		&models.SavedMRQLQuery{},
		&models.Group{},
		&models.GroupRelationType{},
		&models.Resource{},
		&models.User{},
		&models.Note{},
		&models.ResourceVersion{},
		&models.NoteBlock{},
		&models.GroupRelation{},
		&models.Session{},
		&models.ApiToken{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	return NewMahresourcesContext(afero.NewMemMapFs(), db, readOnlyDB, &MahresourcesConfig{
		DbType:      constants.DbTypeSqlite,
		AuthEnabled: authEnabled,
	})
}

// makeAdmin creates an enabled admin (which warms the root-admin cache via the
// CreateUser re-warm) and returns it.
func makeAdmin(t *testing.T, ctx *MahresourcesContext, username string) *models.User {
	t.Helper()
	u, err := ctx.CreateUser(&UserInput{Username: username, Password: "password1", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create admin %q: %v", username, err)
	}
	return u
}

// --- Phase 1: migration + default NULL ---

func TestStamp_MigrationAddsColumnAndPlainCreateLeavesNull(t *testing.T) {
	ctx := newStampTestContext(t, true) // auth-on, no principal → no stamp
	if !ctx.db.Migrator().HasColumn(&models.Resource{}, "created_by_user_id") {
		t.Fatal("resources.created_by_user_id column missing after migrate")
	}
	res := &models.Resource{Name: "plain"}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	var got models.Resource
	if err := ctx.db.First(&got, res.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.CreatedByUserId != nil {
		t.Errorf("auth-on no-context create should leave CreatedByUserId NULL, got %v", *got.CreatedByUserId)
	}
}

// --- Phase 2: stamp under a request principal ---

func TestStamp_WithPrincipalStampsActingUser(t *testing.T) {
	ctx := newStampTestContext(t, true)
	admin := makeAdmin(t, ctx, "admin")
	editor, err := ctx.CreateUser(&UserInput{Username: "ed", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create editor: %v", err)
	}

	scoped := ctx.WithPrincipal(auth.FromUser(editor))
	tag := &models.Tag{Name: "t1"}
	if err := scoped.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if tag.CreatedByUserId == nil || *tag.CreatedByUserId != editor.ID {
		t.Fatalf("expected CreatedByUserId=%d, got %v", editor.ID, tag.CreatedByUserId)
	}
	_ = admin
}

func TestStamp_BatchStampsEveryRow(t *testing.T) {
	ctx := newStampTestContext(t, true)
	u, err := ctx.CreateUser(&UserInput{Username: "u", Password: "password1", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	scoped := ctx.WithPrincipal(auth.FromUser(u))
	tags := []*models.Tag{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	if err := scoped.db.Create(&tags).Error; err != nil {
		t.Fatalf("batch create: %v", err)
	}
	for i, tg := range tags {
		if tg.CreatedByUserId == nil || *tg.CreatedByUserId != u.ID {
			t.Errorf("row %d: expected CreatedByUserId=%d, got %v", i, u.ID, tg.CreatedByUserId)
		}
	}
}

func TestStamp_OverwriteNonSpoofable(t *testing.T) {
	ctx := newStampTestContext(t, true)
	u, err := ctx.CreateUser(&UserInput{Username: "u", Password: "password1", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	other := uint(99999)
	scoped := ctx.WithPrincipal(auth.FromUser(u))
	tag := &models.Tag{Name: "spoof", CreatedByUserId: &other}
	if err := scoped.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if tag.CreatedByUserId == nil || *tag.CreatedByUserId != u.ID {
		t.Fatalf("pre-set creator must be overwritten with acting user %d, got %v", u.ID, tag.CreatedByUserId)
	}
}

func TestStamp_AuthOnNoContextLeavesNull(t *testing.T) {
	ctx := newStampTestContext(t, true)
	makeAdmin(t, ctx, "admin") // warms cache, but auth-on → defaultActorID returns 0
	tag := &models.Tag{Name: "bg"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if tag.CreatedByUserId != nil {
		t.Errorf("auth-on background create should be NULL, got %v", *tag.CreatedByUserId)
	}
}

func TestStamp_NoAuthNoContextStampsRoot(t *testing.T) {
	ctx := newStampTestContext(t, false) // no-auth
	root := makeAdmin(t, ctx, "root")    // warms cache
	tag := &models.Tag{Name: "singleton"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if tag.CreatedByUserId == nil || *tag.CreatedByUserId != root.ID {
		t.Fatalf("no-auth singleton create should stamp root %d, got %v", root.ID, tag.CreatedByUserId)
	}
}

// --- Phase 2: no request DTO can supply CreatedByUserId ---

func TestStamp_CreateDTOsHaveNoCreatedByField(t *testing.T) {
	dtos := []any{
		query_models.ResourceCreator{},
		query_models.NoteCreator{},
		query_models.GroupCreator{},
		query_models.GroupEditor{},
		query_models.TagCreator{},
		query_models.CategoryCreator{},
		query_models.CategoryEditor{},
		query_models.ResourceCategoryCreator{},
		query_models.ResourceCategoryEditor{},
		query_models.NoteTypeEditor{},
		query_models.QueryCreator{},
		query_models.QueryEditor{},
		query_models.SeriesCreator{},
		query_models.SeriesEditor{},
		query_models.NoteEditor{},
	}
	for _, dto := range dtos {
		if hasFieldNamed(reflect.TypeOf(dto), "CreatedByUserId") {
			t.Errorf("%T must not expose CreatedByUserId (client-spoofable)", dto)
		}
	}
}

// hasFieldNamed reports whether t (a struct type) has a field named target,
// descending into embedded (anonymous) structs.
func hasFieldNamed(t reflect.Type, target string) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Name == target {
			return true
		}
		if f.Anonymous && hasFieldNamed(f.Type, target) {
			return true
		}
	}
	return false
}

// --- Phase 3: root-admin cache + queries ---

func TestRootAdmin_OrderingAndCount(t *testing.T) {
	ctx := newStampTestContext(t, false)
	a1 := makeAdmin(t, ctx, "admin1")
	_ = makeAdmin(t, ctx, "admin2")
	// A disabled admin and an editor must not count.
	if _, err := ctx.CreateUser(&UserInput{Username: "disabledadmin", Password: "password1", Role: models.RoleAdmin, Disabled: true}); err != nil {
		t.Fatalf("create disabled admin: %v", err)
	}
	if _, err := ctx.CreateUser(&UserInput{Username: "ed", Password: "password1", Role: models.RoleEditor}); err != nil {
		t.Fatalf("create editor: %v", err)
	}

	n, err := ctx.CountEnabledAdmins()
	if err != nil {
		t.Fatalf("CountEnabledAdmins: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 enabled admins, got %d", n)
	}

	root, err := ctx.RootAdmin()
	if err != nil {
		t.Fatalf("RootAdmin: %v", err)
	}
	if root.ID != a1.ID {
		t.Errorf("oldest enabled admin should be admin1 (id=%d), got id=%d", a1.ID, root.ID)
	}
}

func TestRootAdminPrincipal_ErrorsWhenNoAdmin(t *testing.T) {
	ctx := newStampTestContext(t, false)
	// Only a non-admin exists → no enabled admin.
	if _, err := ctx.CreateUser(&UserInput{Username: "ed", Password: "password1", Role: models.RoleEditor}); err != nil {
		t.Fatalf("create editor: %v", err)
	}
	if _, err := ctx.RootAdminPrincipal(); err == nil {
		t.Fatal("RootAdminPrincipal must return an error when no enabled admin exists")
	}
}

// TestRootAdmin_ColdCacheAfterShift proves refreshRootAdmin closes the
// invalidate→nil window: after the original root is removed (role demoted) and a
// second admin remains, a singleton create under no-auth stamps the current
// root's id — never NULL and never the removed id.
func TestRootAdmin_ColdCacheAfterShift(t *testing.T) {
	ctx := newStampTestContext(t, false)
	first := makeAdmin(t, ctx, "first")
	second := makeAdmin(t, ctx, "second")

	// Demote the original root to editor (root shifts to `second`).
	if _, err := ctx.UpdateUser(first.ID, &UserInput{Username: "first", Password: "", Role: models.RoleEditor}); err != nil {
		t.Fatalf("demote first: %v", err)
	}

	tag := &models.Tag{Name: "afterShift"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if tag.CreatedByUserId == nil {
		t.Fatal("singleton create after root shift must not be NULL")
	}
	if *tag.CreatedByUserId != second.ID {
		t.Errorf("expected current root %d, got %d", second.ID, *tag.CreatedByUserId)
	}
}
