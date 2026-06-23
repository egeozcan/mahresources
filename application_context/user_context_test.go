package application_context

import (
	"errors"
	"fmt"
	"testing"

	"mahresources/constants"
	"mahresources/models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newAuthTestContext opens a private in-memory SQLite database migrated with the
// models needed by the auth/account services and returns a context. Kept
// self-contained (no build tags) so the auth unit tests run with or without the
// json1/fts5 build tags.
func newAuthTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Category{},
		&models.Group{},
		&models.User{},
		&models.Session{},
		&models.ApiToken{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	return NewMahresourcesContext(afero.NewMemMapFs(), db, readOnlyDB, &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
}

func makeTestGroup(t *testing.T, ctx *MahresourcesContext, name string) *models.Group {
	t.Helper()
	g := &models.Group{Name: name}
	if err := ctx.db.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	return g
}

func TestCreateUser_Success(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, err := ctx.CreateUser(&UserInput{Username: "  editor1 ", DisplayName: "Ed", Password: "pw-secret", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Username != "editor1" {
		t.Errorf("username should be trimmed, got %q", u.Username)
	}
	if u.PasswordHash == "" || u.PasswordHash == "pw-secret" {
		t.Error("password must be hashed")
	}
	if u.GUID == nil || *u.GUID == "" {
		t.Error("GUID should be auto-assigned")
	}
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	ctx := newAuthTestContext(t)
	if _, err := ctx.CreateUser(&UserInput{Username: "dup", Password: "pw", Role: models.RoleEditor}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := ctx.CreateUser(&UserInput{Username: "dup", Password: "pw", Role: models.RoleUser})
	if !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("expected ErrUsernameTaken, got %v", err)
	}
}

func TestCreateUser_Validation(t *testing.T) {
	ctx := newAuthTestContext(t)

	if _, err := ctx.CreateUser(&UserInput{Username: "x", Password: "pw", Role: "wizard"}); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("invalid role: got %v", err)
	}
	if _, err := ctx.CreateUser(&UserInput{Username: "", Password: "pw", Role: models.RoleAdmin}); !errors.Is(err, ErrUsernameRequired) {
		t.Errorf("empty username: got %v", err)
	}
	if _, err := ctx.CreateUser(&UserInput{Username: "noPw", Role: models.RoleAdmin}); !errors.Is(err, ErrPasswordRequired) {
		t.Errorf("missing password: got %v", err)
	}
	// Guest with no scope group must be rejected.
	if _, err := ctx.CreateUser(&UserInput{Username: "g", Password: "pw", Role: models.RoleGuest}); !errors.Is(err, ErrScopeGroupRequired) {
		t.Errorf("guest without scope: got %v", err)
	}
	// Scope group must exist.
	bad := uint(9999)
	if _, err := ctx.CreateUser(&UserInput{Username: "u", Password: "pw", Role: models.RoleUser, ScopeGroupId: &bad}); !errors.Is(err, ErrScopeGroupMissing) {
		t.Errorf("nonexistent scope group: got %v", err)
	}
}

func TestCreateUser_ScopeNormalization(t *testing.T) {
	ctx := newAuthTestContext(t)
	g := makeTestGroup(t, ctx, "scope")

	// Admin with a scope group: scope is forced nil.
	admin, err := ctx.CreateUser(&UserInput{Username: "a", Password: "pw", Role: models.RoleAdmin, ScopeGroupId: &g.ID})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if admin.ScopeGroupId != nil {
		t.Errorf("admin scope should be nil, got %v", admin.ScopeGroupId)
	}

	// Guest with a valid scope group: scope is retained.
	guest, err := ctx.CreateUser(&UserInput{Username: "g", Password: "pw", Role: models.RoleGuest, ScopeGroupId: &g.ID})
	if err != nil {
		t.Fatalf("create guest: %v", err)
	}
	if guest.ScopeGroupId == nil || *guest.ScopeGroupId != g.ID {
		t.Errorf("guest scope should be %d, got %v", g.ID, guest.ScopeGroupId)
	}
}

func TestAuthenticateUser(t *testing.T) {
	ctx := newAuthTestContext(t)
	if _, err := ctx.CreateUser(&UserInput{Username: "bob", Password: "hunter2", Role: models.RoleUser}); err != nil {
		t.Fatalf("create: %v", err)
	}

	if u, err := ctx.AuthenticateUser("bob", "hunter2"); err != nil || u == nil {
		t.Errorf("valid login should succeed, got err=%v", err)
	}
	if _, err := ctx.AuthenticateUser("bob", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password: got %v", err)
	}
	if _, err := ctx.AuthenticateUser("nobody", "x"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown user: got %v", err)
	}

	// Disabled accounts cannot authenticate.
	u, _ := ctx.GetUserByUsername("bob")
	u.Disabled = true
	ctx.db.Save(u)
	if _, err := ctx.AuthenticateUser("bob", "hunter2"); !errors.Is(err, ErrUserDisabled) {
		t.Errorf("disabled user: got %v", err)
	}
}

func TestUpdateUserAndPassword(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, _ := ctx.CreateUser(&UserInput{Username: "carol", Password: "orig", Role: models.RoleUser})
	origHash := u.PasswordHash

	// Update without password keeps the hash but changes other fields.
	updated, err := ctx.UpdateUser(u.ID, &UserInput{Username: "carol", DisplayName: "Carol C", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if updated.PasswordHash != origHash {
		t.Error("blank password should leave hash unchanged")
	}
	if updated.Role != models.RoleEditor || updated.DisplayName != "Carol C" {
		t.Error("role/displayname should update")
	}

	// SetUserPassword changes the hash and invalidates the old password.
	if err := ctx.SetUserPassword(u.ID, "newpass"); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	if _, err := ctx.AuthenticateUser("carol", "newpass"); err != nil {
		t.Errorf("new password should work: %v", err)
	}
	if _, err := ctx.AuthenticateUser("carol", "orig"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("old password should fail: %v", err)
	}

	if err := ctx.SetUserPassword(4242, "x"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("set password for missing user: %v", err)
	}
}

func TestCountUsersAndBootstrap(t *testing.T) {
	ctx := newAuthTestContext(t)
	if n, _ := ctx.CountUsers(); n != 0 {
		t.Errorf("fresh DB should have 0 users, got %d", n)
	}

	first, err := ctx.EnsureAdminUser("root", "pw1")
	if err != nil {
		t.Fatalf("EnsureAdminUser create: %v", err)
	}
	if first.Role != models.RoleAdmin {
		t.Error("bootstrapped user should be admin")
	}

	// Idempotent: second call resets password, keeps a single admin row.
	second, err := ctx.EnsureAdminUser("root", "pw2")
	if err != nil {
		t.Fatalf("EnsureAdminUser reset: %v", err)
	}
	if second.ID != first.ID {
		t.Error("EnsureAdminUser should reuse the existing account")
	}
	if n, _ := ctx.CountUsers(); n != 1 {
		t.Errorf("should still be 1 user, got %d", n)
	}
	if _, err := ctx.AuthenticateUser("root", "pw2"); err != nil {
		t.Errorf("reset password should authenticate: %v", err)
	}
}

func TestDeleteUserCleansDependents(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, _ := ctx.CreateUser(&UserInput{Username: "del", Password: "pw", Role: models.RoleUser})
	if _, _, err := ctx.CreateSession(u.ID, 0, "", ""); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, _, err := ctx.CreateApiToken(u.ID, "cli", nil); err != nil {
		t.Fatalf("CreateApiToken: %v", err)
	}

	if err := ctx.DeleteUser(u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := ctx.GetUser(u.ID); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("user should be gone, got %v", err)
	}
	var sessions, tokens int64
	ctx.db.Model(&models.Session{}).Where("user_id = ?", u.ID).Count(&sessions)
	ctx.db.Model(&models.ApiToken{}).Where("user_id = ?", u.ID).Count(&tokens)
	if sessions != 0 || tokens != 0 {
		t.Errorf("dependents should be removed, sessions=%d tokens=%d", sessions, tokens)
	}

	if err := ctx.DeleteUser(u.ID); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("deleting a missing user should error, got %v", err)
	}
}
