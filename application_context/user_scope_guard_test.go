package application_context

import (
	"errors"
	"sync"
	"testing"
	"time"

	"mahresources/auth"
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

// mergeScopeVictimFixture builds root > {child, sibling}, a note under each of
// child and sibling, and two accounts confined to child: a guest and a user.
func mergeScopeVictimFixture(t *testing.T, ctx *MahresourcesContext) (root, child *models.Group, childNote, siblingNote *models.Note, victims []*models.User) {
	t.Helper()
	root = makeTestGroup(t, ctx, "root")
	child = &models.Group{Name: "child", OwnerId: &root.ID}
	sibling := &models.Group{Name: "sibling", OwnerId: &root.ID}
	for _, g := range []*models.Group{child, sibling} {
		if err := ctx.db.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
	}
	childNote = &models.Note{Name: "child-note", OwnerId: &child.ID}
	siblingNote = &models.Note{Name: "sibling-note", OwnerId: &sibling.ID}
	for _, n := range []*models.Note{childNote, siblingNote} {
		if err := ctx.db.Create(n).Error; err != nil {
			t.Fatalf("create note: %v", err)
		}
	}
	guest, err := ctx.CreateUser(&UserInput{Username: "victim-guest", Password: "password1", Role: models.RoleGuest, ScopeGroupId: &child.ID})
	if err != nil {
		t.Fatalf("create guest victim: %v", err)
	}
	user := createScopedTestUser(t, ctx, "victim-user", child.ID)
	return root, child, childNote, siblingNote, []*models.User{guest, user}
}

func assertNoteOwner(t *testing.T, ctx *MahresourcesContext, noteID, wantOwner uint) {
	t.Helper()
	var note models.Note
	if err := ctx.db.First(&note, noteID).Error; err != nil {
		t.Fatalf("reload note %d: %v", noteID, err)
	}
	if note.OwnerId == nil || *note.OwnerId != wantOwner {
		t.Fatalf("note %d owner = %v, want %d", noteID, note.OwnerId, wantOwner)
	}
}

// Assigning a scope group is admin-only (/v1/user* is capSystem), but a merge
// moved every account scoped to a loser onto the winner whoever ran it. A
// scope-limited user merging a tidy-looking child into its parent therefore
// widened other accounts' confinement to the parent's whole subtree.
func TestMergeGroupsByScopedUserRefusesUserScopeLoser(t *testing.T) {
	ctx := newUserScopeGuardTestContext(t)
	root, child, childNote, siblingNote, victims := mergeScopeVictimFixture(t, ctx)
	actor := createScopedTestUser(t, ctx, "merging-user", root.ID)

	err := ctx.WithPrincipal(auth.FromUser(actor)).MergeGroups(root.ID, []uint{child.ID})
	if !errors.Is(err, ErrGroupIsUserScope) {
		t.Fatalf("scoped user's merge of a scope group: err = %v, want ErrGroupIsUserScope", err)
	}

	assertGroupPresent(t, ctx, child.ID)
	for _, v := range victims {
		assertUserScope(t, ctx, v.ID, child.ID)
	}
	assertNoteOwner(t, ctx, childNote.ID, child.ID)
	if ctx.WithPrincipal(auth.FromUser(victims[0])).NoteVisible(siblingNote.ID) {
		t.Fatal("the guest scoped to child can now read a note under its sibling")
	}
}

// Editors are never scoped, but they cannot assign a scope either, so the
// refusal keys on the role, not on the caller's own confinement.
func TestMergeGroupsByEditorRefusesUserScopeLoser(t *testing.T) {
	ctx := newUserScopeGuardTestContext(t)
	root, child, childNote, _, victims := mergeScopeVictimFixture(t, ctx)
	editor, err := ctx.CreateUser(&UserInput{Username: "merging-editor", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create editor: %v", err)
	}

	err = ctx.WithPrincipal(auth.FromUser(editor)).MergeGroups(root.ID, []uint{child.ID})
	if !errors.Is(err, ErrGroupIsUserScope) {
		t.Fatalf("editor's merge of a scope group: err = %v, want ErrGroupIsUserScope", err)
	}
	assertGroupPresent(t, ctx, child.ID)
	for _, v := range victims {
		assertUserScope(t, ctx, v.ID, child.ID)
	}
	assertNoteOwner(t, ctx, childNote.ID, child.ID)
}

// An admin keeps the atomic transfer: the merge succeeds and the scope follows
// the winner, so it is never orphaned.
func TestMergeGroupsByAdminTransfersUserScope(t *testing.T) {
	ctx := newUserScopeGuardTestContext(t)
	root, child, childNote, _, victims := mergeScopeVictimFixture(t, ctx)
	admin, err := ctx.CreateUser(&UserInput{Username: "merging-admin", Password: "password1", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	if err := ctx.WithPrincipal(auth.FromUser(admin)).MergeGroups(root.ID, []uint{child.ID}); err != nil {
		t.Fatalf("admin merge: %v", err)
	}
	for _, v := range victims {
		assertUserScope(t, ctx, v.ID, root.ID)
	}
	assertNoteOwner(t, ctx, childNote.ID, root.ID)
}

// The refusal is about scope references only: a scoped user may still merge a
// loser nobody is confined to.
func TestMergeGroupsByScopedUserStillMergesUnreferencedLoser(t *testing.T) {
	ctx := newUserScopeGuardTestContext(t)
	root := makeTestGroup(t, ctx, "root")
	loser := &models.Group{Name: "plain-loser", OwnerId: &root.ID}
	if err := ctx.db.Create(loser).Error; err != nil {
		t.Fatalf("create loser: %v", err)
	}
	actor := createScopedTestUser(t, ctx, "merging-user", root.ID)

	if err := ctx.WithPrincipal(auth.FromUser(actor)).MergeGroups(root.ID, []uint{loser.ID}); err != nil {
		t.Fatalf("scoped merge of an unreferenced loser: %v", err)
	}
	var count int64
	if err := ctx.db.Model(&models.Group{}).Where("id = ?", loser.ID).Count(&count).Error; err != nil {
		t.Fatalf("count loser: %v", err)
	}
	if count != 0 {
		t.Fatalf("loser group count = %d, want 0", count)
	}
}

func TestMergeAndScopeUpdateLockGroupsBeforeUsers(t *testing.T) {
	for _, updateWins := range []bool{false, true} {
		name := "merge-wins"
		winningOperation := "merge"
		if updateWins {
			name = "update-wins"
			winningOperation = "assign"
		}
		t.Run(name, func(t *testing.T) {
			ctx := newSharedFileContext(t)
			winner := makeTestGroup(t, ctx, "winner-"+name)
			loser := makeTestGroup(t, ctx, "loser-"+name)
			user := createScopedTestUser(t, ctx, "merge-update-"+name, loser.ID)

			locked := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			ctx.scopeLockBarrier = func(operation string, groupID uint) {
				if operation == winningOperation && groupID == winner.ID {
					once.Do(func() {
						close(locked)
						<-release
					})
				}
			}

			mergeDone := make(chan error, 1)
			updateDone := make(chan error, 1)
			startMerge := func() { go func() { mergeDone <- ctx.MergeGroups(winner.ID, []uint{loser.ID}) }() }
			startUpdate := func() {
				go func() {
					_, err := ctx.UpdateUser(user.ID, &UserUpdate{
						ScopeGroupID: UserField[*uint]{Set: true, Value: &winner.ID},
					})
					updateDone <- err
				}()
			}
			if updateWins {
				startUpdate()
			} else {
				startMerge()
			}
			select {
			case <-locked:
			case <-time.After(2 * time.Second):
				t.Fatal("first operation never acquired the winner group lock")
			}
			if updateWins {
				startMerge()
			} else {
				startUpdate()
			}
			select {
			case err := <-mergeDone:
				close(release)
				<-updateDone
				t.Fatalf("merge completed before winner lock release: %v", err)
			case err := <-updateDone:
				close(release)
				<-mergeDone
				t.Fatalf("scope update completed before winner lock release: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			close(release)

			if mergeErr, updateErr := <-mergeDone, <-updateDone; mergeErr != nil || updateErr != nil {
				t.Fatalf("merge=%v update=%v, want both successful", mergeErr, updateErr)
			}
			assertUserScope(t, ctx, user.ID, winner.ID)
			var loserCount int64
			if err := ctx.db.Model(&models.Group{}).Where("id = ?", loser.ID).Count(&loserCount).Error; err != nil {
				t.Fatalf("count loser: %v", err)
			}
			if loserCount != 0 {
				t.Fatalf("loser group count=%d, want 0", loserCount)
			}
		})
	}
}

func TestScopeAssignmentAndDeleteSerializeWithoutDanglingReference(t *testing.T) {
	for _, useUpdate := range []bool{false, true} {
		method := "create"
		if useUpdate {
			method = "update"
		}
		for _, assignmentWins := range []bool{false, true} {
			name := method + "/delete-wins"
			if assignmentWins {
				name = method + "/assignment-wins"
			}
			t.Run(name, func(t *testing.T) {
				ctx := newSharedFileContext(t)
				group := makeTestGroup(t, ctx, name)
				var assignmentUser *models.User
				if useUpdate {
					var err error
					assignmentUser, err = ctx.CreateUser(&UserInput{Username: "racer-" + method, Password: "password1", Role: models.RoleUser})
					if err != nil {
						t.Fatalf("create update target: %v", err)
					}
				}
				locked := make(chan struct{})
				release := make(chan struct{})
				var once sync.Once
				winningOperation := "delete"
				if assignmentWins {
					winningOperation = "assign"
				}
				ctx.scopeLockBarrier = func(operation string, groupID uint) {
					if operation == winningOperation && groupID == group.ID {
						once.Do(func() {
							close(locked)
							<-release
						})
					}
				}

				deleteDone := make(chan error, 1)
				assignDone := make(chan error, 1)
				startAssign := func() {
					go func() {
						if useUpdate {
							_, err := ctx.UpdateUser(assignmentUser.ID, &UserUpdate{ScopeGroupID: UserField[*uint]{Set: true, Value: &group.ID}})
							assignDone <- err
							return
						}
						var err error
						assignmentUser, err = ctx.CreateUser(&UserInput{
							Username: "racer-" + method, Password: "password1", Role: models.RoleUser, ScopeGroupId: &group.ID,
						})
						assignDone <- err
					}()
				}
				startDelete := func() { go func() { deleteDone <- ctx.DeleteGroup(group.ID) }() }
				if assignmentWins {
					startAssign()
				} else {
					startDelete()
				}
				<-locked // real barrier: the winning transaction holds the DB lock
				if assignmentWins {
					startDelete()
				} else {
					startAssign()
				}
				select {
				case err := <-deleteDone:
					t.Fatalf("losing operation completed before lock release: %v", err)
				case err := <-assignDone:
					t.Fatalf("losing operation completed before lock release: %v", err)
				case <-time.After(50 * time.Millisecond):
				}
				close(release)

				deleteErr, assignErr := <-deleteDone, <-assignDone
				if assignmentWins {
					if assignErr != nil || !errors.Is(deleteErr, ErrGroupIsUserScope) {
						t.Fatalf("assignment wins: assign=%v delete=%v", assignErr, deleteErr)
					}
					if assignmentUser == nil {
						t.Fatal("assignment succeeded without a user")
					}
					assertGroupPresent(t, ctx, group.ID)
					assertUserScope(t, ctx, assignmentUser.ID, group.ID)
				} else {
					if deleteErr != nil || !errors.Is(assignErr, ErrScopeGroupMissing) {
						t.Fatalf("delete wins: delete=%v assign=%v", deleteErr, assignErr)
					}
					var groupCount, danglingCount int64
					ctx.db.Model(&models.Group{}).Where("id = ?", group.ID).Count(&groupCount)
					ctx.db.Model(&models.User{}).Where("scope_group_id = ?", group.ID).Count(&danglingCount)
					if groupCount != 0 || danglingCount != 0 {
						t.Fatalf("delete winner left group=%d dangling users=%d", groupCount, danglingCount)
					}
				}
			})
		}
	}
}

func TestBulkDeleteGroupsSortsAndDeduplicatesLockIDs(t *testing.T) {
	ctx := newSharedFileContext(t)
	first := makeTestGroup(t, ctx, "bulk-order-first")
	second := makeTestGroup(t, ctx, "bulk-order-second")
	var locked []uint
	ctx.scopeLockBarrier = func(operation string, groupID uint) {
		if operation == "delete" {
			locked = append(locked, groupID)
		}
	}
	if err := ctx.BulkDeleteGroups(&query_models.BulkQuery{ID: []uint{second.ID, first.ID, second.ID}}); err != nil {
		t.Fatalf("BulkDeleteGroups: %v", err)
	}
	if len(locked) != 2 || locked[0] != first.ID || locked[1] != second.ID {
		t.Fatalf("lock order=%v, want sorted unique [%d %d]", locked, first.ID, second.ID)
	}
}

type recordingGroupDeleteEffects struct {
	logs, hooks, invalidations int
}

func (r *recordingGroupDeleteEffects) LogDeleted(*MahresourcesContext, groupDeleteEffect) { r.logs++ }
func (r *recordingGroupDeleteEffects) RunAfterHook(*MahresourcesContext, groupDeleteEffect) {
	r.hooks++
}
func (r *recordingGroupDeleteEffects) InvalidateCache(*MahresourcesContext, groupDeleteEffect) {
	r.invalidations++
}

func TestBulkDeleteRollbackEmitsNoExternalEffects(t *testing.T) {
	ctx := newUserScopeGuardTestContext(t)
	first := makeTestGroup(t, ctx, "first")
	recorder := &recordingGroupDeleteEffects{}
	ctx.groupDeleteEffectSink = recorder

	err := ctx.BulkDeleteGroups(&query_models.BulkQuery{ID: []uint{first.ID, first.ID + 99999}})
	if err == nil {
		t.Fatal("bulk delete with a missing later group unexpectedly succeeded")
	}
	assertGroupPresent(t, ctx, first.ID)
	if recorder.logs != 0 || recorder.hooks != 0 || recorder.invalidations != 0 {
		t.Fatalf("rolled-back bulk delete emitted log=%d hook=%d invalidation=%d", recorder.logs, recorder.hooks, recorder.invalidations)
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
