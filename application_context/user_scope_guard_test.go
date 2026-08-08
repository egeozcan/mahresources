package application_context

import (
	"errors"
	"testing"

	"mahresources/models"
	"mahresources/models/query_models"
)

func newUserScopeGuardTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := newAuthTestContext(t)
	sqlDB, err := ctx.db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	// SQLite foreign-key enforcement is per connection. Pin this fixture to the
	// connection on which enforcement is enabled so it matches production.
	sqlDB.SetMaxOpenConns(1)
	if err := ctx.db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	return ctx
}

func createScopedTestUser(t *testing.T, ctx *MahresourcesContext, username string, groupID uint) *models.User {
	t.Helper()
	user, err := ctx.CreateUser(&UserInput{
		Username:     username,
		Password:     "password1",
		Role:         models.RoleUser,
		ScopeGroupId: &groupID,
	})
	if err != nil {
		t.Fatalf("create scoped user: %v", err)
	}
	return user
}

func assertUserScope(t *testing.T, ctx *MahresourcesContext, userID, wantGroupID uint) {
	t.Helper()
	var user models.User
	if err := ctx.db.First(&user, userID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if user.ScopeGroupId == nil || *user.ScopeGroupId != wantGroupID {
		t.Fatalf("scope group = %v, want %d", user.ScopeGroupId, wantGroupID)
	}
}

func assertGroupPresent(t *testing.T, ctx *MahresourcesContext, groupID uint) {
	t.Helper()
	var count int64
	if err := ctx.db.Model(&models.Group{}).Where("id = ?", groupID).Count(&count).Error; err != nil {
		t.Fatalf("count group %d: %v", groupID, err)
	}
	if count != 1 {
		t.Fatalf("group %d count = %d, want 1", groupID, count)
	}
}

func TestDeleteGroupReferencedByUserScopeReturnsConflict(t *testing.T) {
	ctx := newUserScopeGuardTestContext(t)
	scope := makeTestGroup(t, ctx, "scope")
	user := createScopedTestUser(t, ctx, "scoped-delete", scope.ID)

	err := ctx.DeleteGroup(scope.ID)
	if !errors.Is(err, ErrGroupIsUserScope) {
		t.Fatalf("DeleteGroup error = %v, want ErrGroupIsUserScope", err)
	}

	assertGroupPresent(t, ctx, scope.ID)
	assertUserScope(t, ctx, user.ID, scope.ID)
}

func TestMergeGroupsTransfersUserScopeToWinner(t *testing.T) {
	ctx := newUserScopeGuardTestContext(t)
	winner := makeTestGroup(t, ctx, "winner")
	loser := makeTestGroup(t, ctx, "loser")
	user := createScopedTestUser(t, ctx, "scoped-merge", loser.ID)

	if err := ctx.MergeGroups(winner.ID, []uint{loser.ID}); err != nil {
		t.Fatalf("MergeGroups: %v", err)
	}

	assertUserScope(t, ctx, user.ID, winner.ID)
	var loserCount int64
	if err := ctx.db.Model(&models.Group{}).Where("id = ?", loser.ID).Count(&loserCount).Error; err != nil {
		t.Fatalf("count loser: %v", err)
	}
	if loserCount != 0 {
		t.Fatalf("loser group count = %d, want 0", loserCount)
	}
}

func TestBulkDeleteGroupsReferencedByUserScopeRollsBack(t *testing.T) {
	ctx := newUserScopeGuardTestContext(t)
	unreferenced := makeTestGroup(t, ctx, "unreferenced")
	scope := makeTestGroup(t, ctx, "scope")
	user := createScopedTestUser(t, ctx, "scoped-bulk", scope.ID)

	err := ctx.BulkDeleteGroups(&query_models.BulkQuery{ID: []uint{unreferenced.ID, scope.ID}})
	if !errors.Is(err, ErrGroupIsUserScope) {
		t.Fatalf("BulkDeleteGroups error = %v, want ErrGroupIsUserScope", err)
	}

	// The first deletion must roll back when a later selected group is a scope.
	assertGroupPresent(t, ctx, unreferenced.ID)
	assertGroupPresent(t, ctx, scope.ID)
	assertUserScope(t, ctx, user.ID, scope.ID)
}
