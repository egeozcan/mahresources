//go:build postgres

package api_tests

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"

	"gorm.io/gorm"
)

type postgresMutationBarrier struct {
	acquired chan struct{}
	second   chan struct{}
	release  chan struct{}
	close    sync.Once
}

func installPostgresMutationBarrier(t *testing.T, db *gorm.DB, name string) *postgresMutationBarrier {
	t.Helper()
	barrier := &postgresMutationBarrier{
		acquired: make(chan struct{}), second: make(chan struct{}), release: make(chan struct{}),
	}
	var attempts atomic.Int32
	var first atomic.Bool
	before := "test:observe_user_mutation_advisory_attempt:" + name
	if err := db.Callback().Raw().Before("gorm:raw").Register(before, func(tx *gorm.DB) {
		if !strings.Contains(tx.Statement.SQL.String(), "pg_advisory_xact_lock") {
			return
		}
		if attempts.Add(1) == 2 {
			close(barrier.second)
		}
	}); err != nil {
		t.Fatalf("register advisory attempt callback: %v", err)
	}
	after := "test:pause_user_mutation_advisory_owner:" + name
	if err := db.Callback().Raw().After("gorm:raw").Register(after, func(tx *gorm.DB) {
		if tx.Error != nil || !strings.Contains(tx.Statement.SQL.String(), "pg_advisory_xact_lock") || !first.CompareAndSwap(false, true) {
			return
		}
		close(barrier.acquired)
		<-barrier.release
	}); err != nil {
		t.Fatalf("register advisory owner callback: %v", err)
	}
	t.Cleanup(func() {
		barrier.close.Do(func() { close(barrier.release) })
		_ = db.Callback().Raw().Remove(before)
		_ = db.Callback().Raw().Remove(after)
	})
	return barrier
}

func (b *postgresMutationBarrier) releaseOwner() { b.close.Do(func() { close(b.release) }) }

func waitBarrier(t *testing.T, channel <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(5 * time.Second):
		t.Fatal(message)
	}
}

func assertStillBlocked(t *testing.T, results ...<-chan error) {
	t.Helper()
	for _, result := range results {
		select {
		case err := <-result:
			t.Fatalf("mutation completed before advisory-lock release: %v", err)
		case <-time.After(75 * time.Millisecond):
		}
	}
}

func TestPostgresScopeUpdateAndCreatorAdminDeleteShareMutationLock(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	if _, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "pg-safety-admin", Password: "password1", Role: models.RoleAdmin,
	}); err != nil {
		t.Fatalf("create safety admin: %v", err)
	}
	creator, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "pg-creator-admin", Password: "password1", Role: models.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create creator admin: %v", err)
	}
	from := &models.Group{Name: "pg-scope-from"}
	to := &models.Group{Name: "pg-scope-to", CreatedByUserId: &creator.ID}
	if err := tc.DB.Create(from).Error; err != nil {
		t.Fatalf("create from group: %v", err)
	}
	if err := tc.DB.Create(to).Error; err != nil {
		t.Fatalf("create creator-stamped group: %v", err)
	}
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "pg-scope-update-user", Password: "password1", Role: models.RoleUser, ScopeGroupId: &from.ID,
	})
	if err != nil {
		t.Fatalf("create scoped user: %v", err)
	}

	barrier := installPostgresMutationBarrier(t, tc.DB, "scope-vs-creator-delete")
	updateDone := make(chan error, 1)
	deleteDone := make(chan error, 1)
	go func() {
		_, updateErr := tc.AppCtx.UpdateUser(user.ID, &application_context.UserUpdate{
			ScopeGroupID: application_context.UserField[*uint]{Set: true, Value: &to.ID},
		})
		updateDone <- updateErr
	}()
	waitBarrier(t, barrier.acquired, "scope update did not acquire the shared advisory lock")
	go func() { deleteDone <- tc.AppCtx.DeleteUser(creator.ID) }()
	waitBarrier(t, barrier.second, "creator-admin deletion did not attempt the shared advisory lock")
	assertStillBlocked(t, updateDone, deleteDone)
	barrier.releaseOwner()
	if updateErr, deleteErr := <-updateDone, <-deleteDone; updateErr != nil || deleteErr != nil {
		t.Fatalf("scope update=%v creator delete=%v; want no deadlock", updateErr, deleteErr)
	}
}

func TestPostgresUserDeleteSerializesPasswordResetAndTokenCreation(t *testing.T) {
	for _, credential := range []string{"password-reset", "token-create"} {
		t.Run(credential, func(t *testing.T) {
			tc := SetupPostgresTestEnv(t)
			if credential == "token-create" {
				tc.AppCtx.Config.MaxUserTokens = 2
			}
			user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
				Username: "pg-delete-credential-" + credential, Password: "password1", Role: models.RoleEditor,
			})
			if err != nil {
				t.Fatalf("create user: %v", err)
			}
			barrier := installPostgresMutationBarrier(t, tc.DB, credential)
			deleteDone := make(chan error, 1)
			credentialDone := make(chan error, 1)
			go func() { deleteDone <- tc.AppCtx.DeleteUser(user.ID) }()
			waitBarrier(t, barrier.acquired, "delete did not acquire the shared advisory lock")
			go func() {
				if credential == "password-reset" {
					credentialDone <- tc.AppCtx.SetUserPassword(user.ID, "reset-password")
					return
				}
				_, _, createErr := tc.AppCtx.CreateApiToken(user.ID, "racing-token", nil)
				credentialDone <- createErr
			}()
			waitBarrier(t, barrier.second, "credential mutation did not attempt the shared advisory lock")
			assertStillBlocked(t, deleteDone, credentialDone)
			barrier.releaseOwner()
			if err := <-deleteDone; err != nil {
				t.Fatalf("delete: %v", err)
			}
			if err := <-credentialDone; !errors.Is(err, application_context.ErrUserNotFound) {
				t.Fatalf("credential mutation err=%v, want ErrUserNotFound after delete", err)
			}
		})
	}
}

func TestPostgresConcurrentRootSuffixBootstrapConverges(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	if _, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "root", Password: "password1", Role: models.RoleEditor,
	}); err != nil {
		t.Fatalf("create non-admin root: %v", err)
	}

	bothObservedRoot2Absent := make(chan struct{})
	var observations atomic.Int32
	callback := "test:both_bootstraps_observe_root2_absent"
	if err := tc.DB.Callback().Query().After("gorm:query").Register(callback, func(db *gorm.DB) {
		if db.Statement.Table != "users" || db.Error == nil || !errors.Is(db.Error, gorm.ErrRecordNotFound) {
			return
		}
		for _, value := range db.Statement.Vars {
			if value == "root2" {
				if observations.Add(1) == 2 {
					close(bothObservedRoot2Absent)
				}
				<-bothObservedRoot2Absent
				return
			}
		}
	}); err != nil {
		t.Fatalf("register root2 observation barrier: %v", err)
	}
	defer tc.DB.Callback().Query().Remove(callback)

	results := make(chan struct {
		user *models.User
		err  error
	}, 2)
	for i := 0; i < 2; i++ {
		go func() {
			user, ensureErr := tc.AppCtx.EnsureRootAdmin()
			results <- struct {
				user *models.User
				err  error
			}{user, ensureErr}
		}()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("bootstrap results: %v / %v", first.err, second.err)
	}
	if first.user.ID != second.user.ID || first.user.Username != "root2" || second.user.Username != "root2" {
		t.Fatalf("bootstraps did not converge: first=%+v second=%+v", first.user, second.user)
	}
	var suffixAdmins int64
	if err := tc.DB.Model(&models.User{}).Where("username LIKE ? AND role = ?", "root%", models.RoleAdmin).Count(&suffixAdmins).Error; err != nil {
		t.Fatalf("count suffix admins: %v", err)
	}
	if suffixAdmins != 1 {
		t.Fatalf("suffix admin count=%d, want one root2 and no root3", suffixAdmins)
	}
}

func TestPostgresConcurrentBulkDeleteReversedIDsUsesSharedOrder(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	first := &models.Group{Name: "pg-bulk-order-first"}
	second := &models.Group{Name: "pg-bulk-order-second"}
	if err := tc.DB.Create(first).Error; err != nil {
		t.Fatalf("create first group: %v", err)
	}
	if err := tc.DB.Create(second).Error; err != nil {
		t.Fatalf("create second group: %v", err)
	}
	barrier := installPostgresMutationBarrier(t, tc.DB, "bulk-reversed")
	one := make(chan error, 1)
	two := make(chan error, 1)
	go func() { one <- tc.AppCtx.BulkDeleteGroups(&query_models.BulkQuery{ID: []uint{second.ID, first.ID}}) }()
	waitBarrier(t, barrier.acquired, "first bulk delete did not acquire advisory lock")
	go func() { two <- tc.AppCtx.BulkDeleteGroups(&query_models.BulkQuery{ID: []uint{first.ID, second.ID}}) }()
	waitBarrier(t, barrier.second, "second bulk delete did not attempt advisory lock")
	assertStillBlocked(t, one, two)
	barrier.releaseOwner()
	errOne, errTwo := <-one, <-two
	if errOne != nil {
		t.Fatalf("first reversed bulk delete: %v", errOne)
	}
	if errTwo == nil {
		t.Fatal("second bulk delete unexpectedly succeeded after the first removed both groups")
	}
	var remaining int64
	if err := tc.DB.Model(&models.Group{}).Where("id IN ?", []uint{first.ID, second.ID}).Count(&remaining).Error; err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining groups=%d", remaining)
	}
}
