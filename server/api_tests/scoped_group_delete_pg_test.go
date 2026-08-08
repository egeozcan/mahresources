//go:build postgres

package api_tests

import (
	"errors"
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
