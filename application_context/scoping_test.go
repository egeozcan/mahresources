package application_context

import (
	"errors"
	"fmt"
	"testing"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newScopingTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(
		&models.Category{}, &models.ResourceCategory{},
		&models.Group{}, &models.Resource{}, &models.Note{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	roDB := sqlx.NewDb(sqlDB, "sqlite3")
	return NewMahresourcesContext(afero.NewMemMapFs(), db, roDB, &MahresourcesConfig{DbType: constants.DbTypeSqlite})
}

// scopingFixture builds a group tree root(1) > child(2) > grandchild(3) plus an
// out-of-tree group(4), and one owned resource + note in each of group 2 and 4.
func scopingFixture(t *testing.T, ctx *MahresourcesContext) (root, child, grandchild, outside *models.Group) {
	t.Helper()
	root = &models.Group{Name: "root"}
	mustCreate(t, ctx, root)
	child = &models.Group{Name: "child", OwnerId: &root.ID}
	mustCreate(t, ctx, child)
	grandchild = &models.Group{Name: "grandchild", OwnerId: &child.ID}
	mustCreate(t, ctx, grandchild)
	outside = &models.Group{Name: "outside"}
	mustCreate(t, ctx, outside)

	mustCreate(t, ctx, &models.Resource{Name: "rIn", OwnerId: &child.ID})
	mustCreate(t, ctx, &models.Resource{Name: "rOut", OwnerId: &outside.ID})
	mustCreate(t, ctx, &models.Note{Name: "nIn", OwnerId: &grandchild.ID})
	mustCreate(t, ctx, &models.Note{Name: "nOut", OwnerId: &outside.ID})
	return
}

func mustCreate(t *testing.T, ctx *MahresourcesContext, v any) {
	t.Helper()
	if err := ctx.db.Create(v).Error; err != nil {
		t.Fatalf("create %T: %v", v, err)
	}
}

func scopedToRoot(ctx *MahresourcesContext, rootID uint) *MahresourcesContext {
	return ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, ScopeGroupID: &rootID})
}

func TestScoping_ResourcesAndNotesFilteredToSubtree(t *testing.T) {
	ctx := newScopingTestContext(t)
	root, _, _, _ := scopingFixture(t, ctx)

	scoped := scopedToRoot(ctx, root.ID)

	var resources []models.Resource
	if err := scoped.db.Find(&resources).Error; err != nil {
		t.Fatalf("find resources: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "rIn" {
		t.Fatalf("scoped user should see only in-subtree resource, got %+v", resources)
	}

	var notes []models.Note
	if err := scoped.db.Find(&notes).Error; err != nil {
		t.Fatalf("find notes: %v", err)
	}
	if len(notes) != 1 || notes[0].Name != "nIn" {
		t.Fatalf("scoped user should see only in-subtree note, got %+v", notes)
	}
}

func TestScoping_GroupsFilteredToSubtree(t *testing.T) {
	ctx := newScopingTestContext(t)
	root, _, _, outside := scopingFixture(t, ctx)

	scoped := scopedToRoot(ctx, root.ID)
	var groups []models.Group
	if err := scoped.db.Find(&groups).Error; err != nil {
		t.Fatalf("find groups: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("scoped user should see exactly the 3 subtree groups, got %d", len(groups))
	}
	for _, g := range groups {
		if g.ID == outside.ID {
			t.Fatalf("scoped user must not see the out-of-subtree group")
		}
	}
}

func TestScoping_SingleGetOutsideSubtreeNotFound(t *testing.T) {
	ctx := newScopingTestContext(t)
	root, _, _, outside := scopingFixture(t, ctx)
	scoped := scopedToRoot(ctx, root.ID)

	// Fetching the out-of-subtree group by id returns not found under scoping.
	var g models.Group
	err := scoped.db.First(&g, outside.ID).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("out-of-subtree group fetch should be not found, got %v", err)
	}
}

func TestScoping_AdminAndSystemUnrestricted(t *testing.T) {
	ctx := newScopingTestContext(t)
	scopingFixture(t, ctx)

	// System (singleton) context sees everything.
	var all []models.Resource
	if err := ctx.db.Find(&all).Error; err != nil {
		t.Fatalf("system find: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("system should see all resources, got %d", len(all))
	}

	// Admin principal is unrestricted.
	admin := ctx.WithPrincipal(&auth.Principal{Role: models.RoleAdmin})
	var adminRes []models.Resource
	if err := admin.db.Find(&adminRes).Error; err != nil {
		t.Fatalf("admin find: %v", err)
	}
	if len(adminRes) != 2 {
		t.Fatalf("admin should see all resources, got %d", len(adminRes))
	}
}

func TestScoping_UnscopedUserUnrestricted(t *testing.T) {
	ctx := newScopingTestContext(t)
	scopingFixture(t, ctx)

	// A user with no scope group is not data-restricted.
	user := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser})
	var res []models.Resource
	if err := user.db.Find(&res).Error; err != nil {
		t.Fatalf("unscoped user find: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("unscoped user should see all resources, got %d", len(res))
	}
}

func TestScoping_GuestWithoutSubtreeDeniesAll(t *testing.T) {
	ctx := newScopingTestContext(t)
	scopingFixture(t, ctx)

	// A guest whose scope group could not be resolved (nil) is fail-closed.
	guest := ctx.WithPrincipal(&auth.Principal{Role: models.RoleGuest})
	var res []models.Resource
	if err := guest.db.Find(&res).Error; err != nil {
		t.Fatalf("guest find: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("guest without a resolved subtree should see nothing, got %d", len(res))
	}
}

func TestScoping_CreateOutsideSubtreeRejected(t *testing.T) {
	ctx := newScopingTestContext(t)
	root, child, _, outside := scopingFixture(t, ctx)
	scoped := scopedToRoot(ctx, root.ID)

	// Creating a resource owned by an in-subtree group succeeds.
	if err := scoped.db.Create(&models.Resource{Name: "ok", OwnerId: &child.ID}).Error; err != nil {
		t.Fatalf("in-subtree create should succeed, got %v", err)
	}
	// Creating a resource owned by an out-of-subtree group is rejected.
	if err := scoped.db.Create(&models.Resource{Name: "bad", OwnerId: &outside.ID}).Error; err == nil {
		t.Fatalf("out-of-subtree create should be rejected")
	}
	// Creating an ownerless resource is rejected for a scoped principal.
	if err := scoped.db.Create(&models.Resource{Name: "ownerless"}).Error; err == nil {
		t.Fatalf("ownerless create should be rejected for a scoped principal")
	}
}

func TestScoping_FilePathInScope(t *testing.T) {
	ctx := newScopingTestContext(t)
	root, child, _, outside := scopingFixture(t, ctx)

	// Give the in/out resources distinct storage locations.
	ctx.db.Model(&models.Resource{}).Where("owner_id = ?", child.ID).Update("location", "in/file.bin")
	ctx.db.Model(&models.Resource{}).Where("owner_id = ?", outside.ID).Update("location", "out/file.bin")

	scoped := scopedToRoot(ctx, root.ID)
	if !scoped.FilePathInScope("in/file.bin") {
		t.Error("scoped user should be allowed the in-subtree file")
	}
	if scoped.FilePathInScope("out/file.bin") {
		t.Error("scoped user must not be allowed the out-of-subtree file")
	}
	if scoped.FilePathInScope("nonexistent.bin") {
		t.Error("nonexistent path must not be allowed")
	}

	// The system context is unrestricted but FilePathInScope still requires a
	// matching resource row.
	if !ctx.FilePathInScope("out/file.bin") {
		t.Error("system context should resolve any existing file path")
	}
}

func TestScoping_UpdateDeleteConfinedToSubtree(t *testing.T) {
	ctx := newScopingTestContext(t)
	root, _, _, outside := scopingFixture(t, ctx)
	scoped := scopedToRoot(ctx, root.ID)

	// An update targeting an out-of-subtree resource affects no rows.
	res := scoped.db.Model(&models.Resource{}).Where("owner_id = ?", outside.ID).Update("name", "hacked")
	if res.Error != nil {
		t.Fatalf("update: %v", res.Error)
	}
	if res.RowsAffected != 0 {
		t.Fatalf("scoped update should not touch out-of-subtree rows, affected %d", res.RowsAffected)
	}

	// Confirm via the system context that the row is unchanged.
	var rOut models.Resource
	ctx.db.Where("owner_id = ?", outside.ID).First(&rOut)
	if rOut.Name != "rOut" {
		t.Fatalf("out-of-subtree resource was modified: %q", rOut.Name)
	}
}
