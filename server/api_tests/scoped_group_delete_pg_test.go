//go:build postgres

package api_tests

import (
	"errors"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

func TestScopedGroupDeletePostgresPreservesScopeConfinement(t *testing.T) {
	assertScopedGroupDeleteConflict(t, SetupPostgresTestEnv(t), "postgres-scope-delete")
}

func TestPostgresMergeAndScopeUpdateLockWinnerBeforeUser(t *testing.T) {
	for _, updateWins := range []bool{false, true} {
		name := "merge-wins"
		if updateWins {
			name = "update-wins"
		}
		t.Run(name, func(t *testing.T) {
			tc := SetupPostgresTestEnv(t)
			winner := &models.Group{Name: "pg-merge-winner-" + name}
			loser := &models.Group{Name: "pg-merge-loser-" + name}
			if err := tc.DB.Create(winner).Error; err != nil {
				t.Fatalf("create winner: %v", err)
			}
			if err := tc.DB.Create(loser).Error; err != nil {
				t.Fatalf("create loser: %v", err)
			}
			user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
				Username: "pg-merge-update-" + name, Password: "password1", Role: models.RoleUser, ScopeGroupId: &loser.ID,
			})
			if err != nil {
				t.Fatalf("create scoped user: %v", err)
			}

			barrier := installPostgresMutationBarrier(t, tc.DB, "merge-scope-"+name)

			mergeDone := make(chan error, 1)
			updateDone := make(chan error, 1)
			startMerge := func() { go func() { mergeDone <- tc.AppCtx.MergeGroups(winner.ID, []uint{loser.ID}) }() }
			startUpdate := func() {
				go func() {
					_, updateErr := tc.AppCtx.UpdateUser(user.ID, &application_context.UserUpdate{
						ScopeGroupID: application_context.UserField[*uint]{Set: true, Value: &winner.ID},
					})
					updateDone <- updateErr
				}()
			}
			if updateWins {
				startUpdate()
			} else {
				startMerge()
			}

			waitBarrier(t, barrier.acquired, "first operation never acquired the shared mutation lock")
			if updateWins {
				startMerge()
			} else {
				startUpdate()
			}
			waitBarrier(t, barrier.second, "opposing operation never attempted the shared mutation lock")
			assertStillBlocked(t, mergeDone, updateDone)
			barrier.releaseOwner()

			mergeErr, updateErr := <-mergeDone, <-updateDone
			if mergeErr != nil || updateErr != nil {
				t.Fatalf("merge=%v update=%v; want no deadlock/SQLSTATE 40P01", mergeErr, updateErr)
			}
			var reloaded models.User
			if err := tc.DB.First(&reloaded, user.ID).Error; err != nil {
				t.Fatalf("reload user: %v", err)
			}
			if reloaded.ScopeGroupId == nil || *reloaded.ScopeGroupId != winner.ID {
				t.Fatalf("scope=%v, want winner %d", reloaded.ScopeGroupId, winner.ID)
			}
			var winnerCount, loserCount int64
			if err := tc.DB.Model(&models.Group{}).Where("id = ?", winner.ID).Count(&winnerCount).Error; err != nil {
				t.Fatalf("count winner: %v", err)
			}
			if err := tc.DB.Model(&models.Group{}).Where("id = ?", loser.ID).Count(&loserCount).Error; err != nil {
				t.Fatalf("count loser: %v", err)
			}
			if winnerCount != 1 || loserCount != 0 {
				t.Fatalf("winner count=%d loser count=%d, want 1/0", winnerCount, loserCount)
			}
		})
	}
}

func TestPostgresScopeAssignmentAndDeleteShareRowLock(t *testing.T) {
	for _, assignmentWins := range []bool{false, true} {
		name := "delete-wins"
		if assignmentWins {
			name = "assignment-wins"
		}
		t.Run(name, func(t *testing.T) {
			tc := SetupPostgresTestEnv(t)
			group := &models.Group{Name: "pg-scope-" + name}
			if err := tc.DB.Create(group).Error; err != nil {
				t.Fatalf("create group: %v", err)
			}
			barrier := installPostgresMutationBarrier(t, tc.DB, "assign-delete-"+name)
			deleteDone := make(chan error, 1)
			assignDone := make(chan error, 1)
			startDelete := func() { go func() { deleteDone <- tc.AppCtx.DeleteGroup(group.ID) }() }
			startAssign := func() {
				go func() {
					_, err := tc.AppCtx.CreateUser(&application_context.UserInput{
						Username: "pg-racer-" + name, Password: "password1", Role: models.RoleUser, ScopeGroupId: &group.ID,
					})
					assignDone <- err
				}()
			}
			if assignmentWins {
				startAssign()
			} else {
				startDelete()
			}
			waitBarrier(t, barrier.acquired, "first operation never acquired shared mutation lock")
			if assignmentWins {
				startDelete()
			} else {
				startAssign()
			}
			waitBarrier(t, barrier.second, "second operation never attempted shared mutation lock")
			assertStillBlocked(t, deleteDone, assignDone)
			barrier.releaseOwner()
			deleteErr, assignErr := <-deleteDone, <-assignDone
			if assignmentWins {
				if assignErr != nil || !errors.Is(deleteErr, application_context.ErrGroupIsUserScope) {
					t.Fatalf("assignment wins: assign=%v delete=%v", assignErr, deleteErr)
				}
			} else if deleteErr != nil || !errors.Is(assignErr, application_context.ErrScopeGroupMissing) {
				t.Fatalf("delete wins: delete=%v assign=%v", deleteErr, assignErr)
			}
			var dangling int64
			if err := tc.DB.Model(&models.User{}).Where("scope_group_id = ? AND NOT EXISTS (SELECT 1 FROM groups WHERE groups.id = users.scope_group_id)", group.ID).Count(&dangling).Error; err != nil {
				t.Fatalf("count dangling: %v", err)
			}
			if dangling != 0 {
				t.Fatalf("dangling scopes=%d", dangling)
			}
		})
	}
}
