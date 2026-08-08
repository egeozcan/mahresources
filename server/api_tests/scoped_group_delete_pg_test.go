//go:build postgres

package api_tests

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"

	"gorm.io/gorm"
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

			locked := make(chan uint, 1)
			secondAttempt := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseLock := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseLock()
			var acquired atomic.Bool
			var attempts atomic.Int32
			attemptCallback := "test:observe_merge_scope_group_lock_" + name
			if err := tc.DB.Callback().Query().Before("gorm:query").Register(attemptCallback, func(db *gorm.DB) {
				if db.Statement.Table != "groups" {
					return
				}
				if _, ok := db.Statement.Clauses["FOR"]; !ok {
					return
				}
				if attempts.Add(1) == 2 {
					close(secondAttempt)
				}
			}); err != nil {
				t.Fatalf("register lock-attempt callback: %v", err)
			}
			acquiredCallback := "test:pause_first_merge_scope_group_lock_" + name
			if err := tc.DB.Callback().Query().After("gorm:query").Register(acquiredCallback, func(db *gorm.DB) {
				if db.Statement.Table != "groups" {
					return
				}
				if _, ok := db.Statement.Clauses["FOR"]; !ok || !acquired.CompareAndSwap(false, true) {
					return
				}
				groupID := uint(0)
				if group, ok := db.Statement.Dest.(*models.Group); ok {
					groupID = group.ID
				}
				locked <- groupID
				<-release
			}); err != nil {
				t.Fatalf("register acquired-lock callback: %v", err)
			}

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

			var firstLockID uint
			select {
			case firstLockID = <-locked:
			case <-time.After(3 * time.Second):
				t.Fatal("first operation never acquired a group FOR UPDATE lock")
			}
			if firstLockID != winner.ID {
				releaseLock()
				if updateWins {
					<-updateDone
				} else {
					<-mergeDone
				}
				t.Fatalf("first lock group=%d, want winner %d before any user mutation", firstLockID, winner.ID)
			}
			if updateWins {
				startMerge()
			} else {
				startUpdate()
			}
			select {
			case <-secondAttempt:
			case <-time.After(3 * time.Second):
				releaseLock()
				<-mergeDone
				<-updateDone
				t.Fatal("opposing operation reached a user lock before attempting the winner group lock")
			}
			select {
			case err := <-mergeDone:
				releaseLock()
				<-updateDone
				t.Fatalf("merge completed while winner lock was held: %v", err)
			case err := <-updateDone:
				releaseLock()
				<-mergeDone
				t.Fatalf("scope update completed while winner lock was held: %v", err)
			case <-time.After(75 * time.Millisecond):
			}
			releaseLock()

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
			locked := make(chan struct{})
			secondAttempt := make(chan struct{})
			release := make(chan struct{})
			var used atomic.Bool
			var attempts atomic.Int32
			attemptCallback := "test:observe_scope_group_lock_" + name
			if err := tc.DB.Callback().Query().Before("gorm:query").Register(attemptCallback, func(db *gorm.DB) {
				if db.Statement.Table == "groups" && attempts.Add(1) == 2 {
					close(secondAttempt)
				}
			}); err != nil {
				t.Fatalf("register attempt barrier: %v", err)
			}
			callbackName := "test:pause_scope_group_lock_" + name
			if err := tc.DB.Callback().Query().After("gorm:query").Register(callbackName, func(db *gorm.DB) {
				if db.Statement.Table == "groups" && used.CompareAndSwap(false, true) {
					close(locked)
					<-release
				}
			}); err != nil {
				t.Fatalf("register barrier: %v", err)
			}
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
			<-locked
			if assignmentWins {
				startDelete()
			} else {
				startAssign()
			}
			select {
			case <-secondAttempt:
			case <-time.After(2 * time.Second):
				t.Fatal("second operation never reached the locked group query")
			}
			select {
			case err := <-deleteDone:
				t.Fatalf("operation completed before row-lock release: %v", err)
			case err := <-assignDone:
				t.Fatalf("operation completed before row-lock release: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			close(release)
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
