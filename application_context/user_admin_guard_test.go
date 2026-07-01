package application_context

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newSharedFileContext opens a temp-file SQLite DB (WAL + busy_timeout) shared
// across pool connections, so concurrent goroutines see the same data. Used for
// the last-admin concurrency test (the in-memory cache=private DB gives each
// connection its own database and cannot be shared).
func newSharedFileContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Group{}, &models.Resource{}, &models.Note{}, &models.Tag{},
		&models.Category{}, &models.ResourceCategory{}, &models.NoteType{},
		&models.Series{}, &models.Query{}, &models.SavedMRQLQuery{},
		&models.NoteBlock{}, &models.GroupRelation{}, &models.GroupRelationType{},
		&models.ResourceVersion{}, &models.User{}, &models.Session{}, &models.ApiToken{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(4)
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	return NewMahresourcesContext(afero.NewMemMapFs(), db, readOnlyDB, &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
}

func TestLastAdmin_DeleteSoleAdminBlocked(t *testing.T) {
	ctx := newStampTestContext(t, true)
	admin := makeAdmin(t, ctx, "solo")
	if err := ctx.DeleteUser(admin.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("deleting sole admin: want ErrLastAdmin, got %v", err)
	}
	// The admin must still exist.
	if _, err := ctx.GetUser(admin.ID); err != nil {
		t.Fatalf("sole admin should still exist: %v", err)
	}
}

func TestLastAdmin_DemoteSoleAdminBlocked(t *testing.T) {
	ctx := newStampTestContext(t, true)
	admin := makeAdmin(t, ctx, "solo")
	_, err := ctx.UpdateUser(admin.ID, &UserInput{Username: "solo", Role: models.RoleEditor})
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demoting sole admin: want ErrLastAdmin, got %v", err)
	}
	got, _ := ctx.GetUser(admin.ID)
	if got.Role != models.RoleAdmin || got.Disabled {
		t.Fatalf("sole admin must remain an enabled admin, got role=%q disabled=%v", got.Role, got.Disabled)
	}
}

func TestLastAdmin_DisableSoleAdminBlocked(t *testing.T) {
	ctx := newStampTestContext(t, true)
	admin := makeAdmin(t, ctx, "solo")
	_, err := ctx.UpdateUser(admin.ID, &UserInput{Username: "solo", Role: models.RoleAdmin, Disabled: true})
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("disabling sole admin: want ErrLastAdmin, got %v", err)
	}
	got, _ := ctx.GetUser(admin.ID)
	if got.Disabled {
		t.Fatalf("sole admin must remain enabled")
	}
}

func TestLastAdmin_WithTwoAdminsEachOperationSucceeds(t *testing.T) {
	t.Run("delete", func(t *testing.T) {
		ctx := newStampTestContext(t, true)
		a1 := makeAdmin(t, ctx, "a1")
		makeAdmin(t, ctx, "a2")
		if err := ctx.DeleteUser(a1.ID); err != nil {
			t.Fatalf("delete one of two admins should succeed, got %v", err)
		}
	})
	t.Run("demote", func(t *testing.T) {
		ctx := newStampTestContext(t, true)
		a1 := makeAdmin(t, ctx, "a1")
		makeAdmin(t, ctx, "a2")
		if _, err := ctx.UpdateUser(a1.ID, &UserInput{Username: "a1", Role: models.RoleEditor}); err != nil {
			t.Fatalf("demote one of two admins should succeed, got %v", err)
		}
	})
	t.Run("disable", func(t *testing.T) {
		ctx := newStampTestContext(t, true)
		a1 := makeAdmin(t, ctx, "a1")
		makeAdmin(t, ctx, "a2")
		if _, err := ctx.UpdateUser(a1.ID, &UserInput{Username: "a1", Role: models.RoleAdmin, Disabled: true}); err != nil {
			t.Fatalf("disable one of two admins should succeed, got %v", err)
		}
	})
}

// TestLastAdmin_ConcurrentDeleteDifferentAdmins: two goroutines each delete a
// different one of two admins. Exactly one succeeds, the other gets ErrLastAdmin,
// and ≥1 enabled admin remains. SQLite serializes writers; Postgres coverage of
// the same invariant lives in the API test suite.
func TestLastAdmin_ConcurrentDeleteDifferentAdmins(t *testing.T) {
	ctx := newSharedFileContext(t)
	a1 := makeAdmin(t, ctx, "a1")
	a2 := makeAdmin(t, ctx, "a2")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	targets := []uint{a1.ID, a2.ID}
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(idx uint) {
			defer wg.Done()
			errs[idx] = ctx.DeleteUser(targets[idx])
		}(uint(i))
	}
	wg.Wait()

	successes, lastAdmin := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			successes++
		case errors.Is(e, ErrLastAdmin):
			lastAdmin++
		default:
			t.Fatalf("unexpected error: %v", e)
		}
	}
	if successes != 1 || lastAdmin != 1 {
		t.Fatalf("want exactly one success and one ErrLastAdmin, got successes=%d lastAdmin=%d", successes, lastAdmin)
	}
	n, err := ctx.CountEnabledAdmins()
	if err != nil {
		t.Fatalf("CountEnabledAdmins: %v", err)
	}
	if n < 1 {
		t.Fatalf("at least one enabled admin must remain, got %d", n)
	}
}

// TestRootAdminCache_ConcurrentMutationsConverge stresses the refreshRootAdmin
// resolve+store serialization: after several concurrent admin deletions settle,
// the no-auth default actor must equal the current oldest enabled admin — never a
// deleted one. Without the mutex, a stale read could win a later store and pin
// the cache to a removed admin (a lost update). Looped over fresh DBs to surface
// the race.
func TestRootAdminCache_ConcurrentMutationsConverge(t *testing.T) {
	for iter := 0; iter < 15; iter++ {
		ctx := newSharedFileContext(t)
		// a1 (oldest) .. a4. a4 is never deleted, so deleting a1..a3 always leaves
		// a4 as the sole remaining admin the cache must converge to.
		admins := make([]*models.User, 4)
		for i := 0; i < 4; i++ {
			u, err := ctx.CreateUser(&UserInput{Username: fmt.Sprintf("a%d", i), Password: "password1", Role: models.RoleAdmin})
			if err != nil {
				t.Fatalf("iter %d create a%d: %v", iter, i, err)
			}
			admins[i] = u
		}
		keep := admins[3]

		var wg sync.WaitGroup
		errs := make([]error, 3)
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				errs[idx] = ctx.DeleteUser(admins[idx].ID)
			}(i)
		}
		wg.Wait()
		for i, e := range errs {
			if e != nil {
				t.Fatalf("iter %d: delete a%d should succeed (a4 remains), got %v", iter, i, e)
			}
		}

		// The cache must have converged to the oldest remaining admin (a4).
		if got := ctx.defaultActorID(); got != keep.ID {
			t.Fatalf("iter %d: defaultActorID=%d, want surviving admin a4=%d (stale/lost-update cache)", iter, got, keep.ID)
		}
	}
}

// Phase 5: content stamped by a deleted user survives with a NULL creator.
func TestDeleteUser_NullsCreatorReferences(t *testing.T) {
	ctx := newStampTestContext(t, true)
	makeAdmin(t, ctx, "keeper") // keeps an admin so deleting U is allowed
	u, err := ctx.CreateUser(&UserInput{Username: "u", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	scoped := ctx.WithPrincipal(auth.FromUser(u))
	res := &models.Resource{Name: "owned"}
	if err := scoped.db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	if res.CreatedByUserId == nil || *res.CreatedByUserId != u.ID {
		t.Fatalf("resource should be stamped by U, got %v", res.CreatedByUserId)
	}

	if err := ctx.DeleteUser(u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	var got models.Resource
	if err := ctx.db.First(&got, res.ID).Error; err != nil {
		t.Fatalf("resource should survive user deletion: %v", err)
	}
	if got.CreatedByUserId != nil {
		t.Fatalf("creator reference must be NULL after user deletion, got %v", *got.CreatedByUserId)
	}
}
