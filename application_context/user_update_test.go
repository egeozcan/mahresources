package application_context

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"mahresources/models"

	"gorm.io/gorm"
)

func TestPartialUserUpdateCannotRestoreConcurrentPassword(t *testing.T) {
	ctx := newSharedFileContext(t)
	user, err := ctx.CreateUser(&UserInput{Username: "partial", DisplayName: "Before", Password: "old-password", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	loaded := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	if err := ctx.db.Callback().Query().After("gorm:query").Register("test:pause_partial_user_load", func(db *gorm.DB) {
		if db.Statement.Table == "users" {
			once.Do(func() {
				close(loaded)
				<-release
			})
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}

	updateDone := make(chan error, 1)
	go func() {
		_, updateErr := ctx.UpdateUser(user.ID, &UserUpdate{
			DisplayName: UserField[string]{Set: true, Value: "After"},
		})
		updateDone <- updateErr
	}()
	<-loaded

	if err := ctx.SetUserPassword(user.ID, "new-password"); err != nil {
		t.Fatalf("concurrent SetUserPassword: %v", err)
	}
	close(release)
	if err := <-updateDone; err != nil {
		t.Fatalf("display-name update: %v", err)
	}
	if _, err := ctx.AuthenticateUser("partial", "new-password"); err != nil {
		t.Fatalf("partial update restored stale password hash: %v", err)
	}
}

func TestStaleNonAdminUpdateCannotRemoveLastAdmin(t *testing.T) {
	ctx := newSharedFileContext(t)
	oldAdmin := makeAdmin(t, ctx, "old-admin")
	editor, err := ctx.CreateUser(&UserInput{Username: "editor", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create editor: %v", err)
	}

	paused := make(chan struct{})
	release := make(chan struct{})
	var barrierUsed atomic.Bool
	ctx.userUpdateBarrier = func() {
		if barrierUsed.CompareAndSwap(false, true) {
			close(paused)
			<-release
		}
	}
	staleDone := make(chan error, 1)
	go func() {
		_, staleErr := ctx.UpdateUser(editor.ID, &UserUpdate{
			DisplayName: UserField[string]{Set: true, Value: "stale write"},
			Role:        UserField[models.Role]{Set: true, Value: models.RoleEditor},
		})
		staleDone <- staleErr
	}()
	<-paused // stale editor-valued update is now genuinely in flight

	if _, err := ctx.UpdateUser(editor.ID, &UserUpdate{
		Role: UserField[models.Role]{Set: true, Value: models.RoleAdmin},
	}); err != nil {
		t.Fatalf("promote editor: %v", err)
	}
	if _, err := ctx.UpdateUser(oldAdmin.ID, &UserUpdate{
		Role: UserField[models.Role]{Set: true, Value: models.RoleEditor},
	}); err != nil {
		t.Fatalf("demote old admin: %v", err)
	}
	close(release)

	if err := <-staleDone; !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("stale demotion: want ErrLastAdmin, got %v", err)
	}
	if n, err := ctx.CountEnabledAdmins(); err != nil || n != 1 {
		t.Fatalf("enabled admins=%d err=%v, want one", n, err)
	}
}

func TestLastAdminMixedTransitionsSerialize(t *testing.T) {
	tests := []struct {
		name string
		op1  func(*MahresourcesContext, *models.User) error
		op2  func(*MahresourcesContext, *models.User) error
	}{
		{
			name: "delete-vs-demote",
			op1:  func(ctx *MahresourcesContext, u *models.User) error { return ctx.DeleteUser(u.ID) },
			op2: func(ctx *MahresourcesContext, u *models.User) error {
				_, err := ctx.UpdateUser(u.ID, &UserUpdate{Role: UserField[models.Role]{Set: true, Value: models.RoleEditor}})
				return err
			},
		},
		{
			name: "demote-vs-disable",
			op1: func(ctx *MahresourcesContext, u *models.User) error {
				_, err := ctx.UpdateUser(u.ID, &UserUpdate{Role: UserField[models.Role]{Set: true, Value: models.RoleEditor}})
				return err
			},
			op2: func(ctx *MahresourcesContext, u *models.User) error {
				_, err := ctx.UpdateUser(u.ID, &UserUpdate{Disabled: UserField[bool]{Set: true, Value: true}})
				return err
			},
		},
		{
			name: "delete-vs-disable",
			op1:  func(ctx *MahresourcesContext, u *models.User) error { return ctx.DeleteUser(u.ID) },
			op2: func(ctx *MahresourcesContext, u *models.User) error {
				_, err := ctx.UpdateUser(u.ID, &UserUpdate{Disabled: UserField[bool]{Set: true, Value: true}})
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newSharedFileContext(t)
			a := makeAdmin(t, ctx, "a")
			b := makeAdmin(t, ctx, "b")
			start := make(chan struct{})
			errs := make(chan error, 2)
			go func() { <-start; errs <- tc.op1(ctx, a) }()
			go func() { <-start; errs <- tc.op2(ctx, b) }()
			close(start)

			first, second := <-errs, <-errs
			successes, blocked := 0, 0
			for _, err := range []error{first, second} {
				switch {
				case err == nil:
					successes++
				case errors.Is(err, ErrLastAdmin):
					blocked++
				default:
					t.Fatalf("unexpected transition error: %v", err)
				}
			}
			if successes != 1 || blocked != 1 {
				t.Fatalf("successes=%d blocked=%d, want one each", successes, blocked)
			}
			if n, err := ctx.CountEnabledAdmins(); err != nil || n != 1 {
				t.Fatalf("enabled admins=%d err=%v, want one", n, err)
			}
		})
	}
}

func TestDisableRollbackWhenCredentialCleanupFails(t *testing.T) {
	ctx := newAuthTestContext(t)
	makeAdmin(t, ctx, "keeper")
	user, err := ctx.CreateUser(&UserInput{Username: "rollback", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, _, err := ctx.CreateSession(user.ID, 0, "", ""); err != nil {
		t.Fatalf("create session: %v", err)
	}

	cleanupErr := errors.New("injected credential cleanup failure")
	if err := ctx.db.Callback().Delete().Before("gorm:delete").Register("test:fail_session_cleanup", func(db *gorm.DB) {
		if db.Statement.Table == "sessions" {
			db.AddError(cleanupErr)
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}

	_, err = ctx.UpdateUser(user.ID, FullUserUpdate(&UserInput{
		Username: "rollback", Role: models.RoleUser, Disabled: true,
	}))
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("disable should return cleanup error, got %v", err)
	}
	reloaded, err := ctx.GetUser(user.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Disabled {
		t.Fatal("disable committed despite credential cleanup failure")
	}
}
