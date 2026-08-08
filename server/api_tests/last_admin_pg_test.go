//go:build postgres

package api_tests

import (
	"errors"
	"sync"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// TestLastAdmin_PostgresConcurrency proves the last-admin guard holds under
// Postgres read-committed with true concurrency (separate pool connections): two
// goroutines each delete a different one of two admins. The FOR UPDATE lock on
// the enabled-admin row set serializes them, so exactly one succeeds, the other
// gets ErrLastAdmin, and at least one enabled admin remains. Without the lock,
// read-committed would let both observe two admins and each delete a different
// one down to zero.
func TestLastAdmin_PostgresMixedTransitionsUseExplicitBarrier(t *testing.T) {
	tests := []struct {
		name string
		op1  func(*application_context.MahresourcesContext, *models.User) error
		op2  func(*application_context.MahresourcesContext, *models.User) error
	}{
		{"delete-vs-demote", func(ctx *application_context.MahresourcesContext, u *models.User) error { return ctx.DeleteUser(u.ID) }, func(ctx *application_context.MahresourcesContext, u *models.User) error {
			_, err := ctx.UpdateUser(u.ID, &application_context.UserUpdate{Role: application_context.UserField[models.Role]{Set: true, Value: models.RoleEditor}})
			return err
		}},
		{"demote-vs-disable", func(ctx *application_context.MahresourcesContext, u *models.User) error {
			_, err := ctx.UpdateUser(u.ID, &application_context.UserUpdate{Role: application_context.UserField[models.Role]{Set: true, Value: models.RoleEditor}})
			return err
		}, func(ctx *application_context.MahresourcesContext, u *models.User) error {
			_, err := ctx.UpdateUser(u.ID, &application_context.UserUpdate{Disabled: application_context.UserField[bool]{Set: true, Value: true}})
			return err
		}},
		{"delete-vs-disable", func(ctx *application_context.MahresourcesContext, u *models.User) error { return ctx.DeleteUser(u.ID) }, func(ctx *application_context.MahresourcesContext, u *models.User) error {
			_, err := ctx.UpdateUser(u.ID, &application_context.UserUpdate{Disabled: application_context.UserField[bool]{Set: true, Value: true}})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tc := SetupPostgresTestEnv(t)
			a := makePGAdmin(t, tc, "barrier-a")
			b := makePGAdmin(t, tc, "barrier-b")
			barrier := installPostgresMutationBarrier(t, tc.DB, "last-admin-"+test.name)
			done := make(chan error, 2)
			go func() { done <- test.op1(tc.AppCtx, a) }()
			waitBarrier(t, barrier.acquired, "first transition did not acquire shared mutation lock")
			go func() { done <- test.op2(tc.AppCtx, b) }()
			waitBarrier(t, barrier.second, "second transition did not attempt shared mutation lock")
			assertStillBlocked(t, done)
			barrier.releaseOwner()
			first, second := <-done, <-done
			successes, conflicts := 0, 0
			for _, err := range []error{first, second} {
				switch {
				case err == nil:
					successes++
				case errors.Is(err, application_context.ErrLastAdmin):
					conflicts++
				default:
					t.Fatalf("unexpected transition error: %v", err)
				}
			}
			if successes != 1 || conflicts != 1 {
				t.Fatalf("successes=%d conflicts=%d, want one each", successes, conflicts)
			}
			if n, err := tc.AppCtx.CountEnabledAdmins(); err != nil || n != 1 {
				t.Fatalf("enabled admins=%d err=%v, want one", n, err)
			}
		})
	}
}

func makePGAdmin(t *testing.T, tc *TestContext, username string) *models.User {
	t.Helper()
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: username, Password: "password1", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create %s: %v", username, err)
	}
	return user
}

func TestLastAdmin_PostgresConcurrency(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	a1, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: "pg_a1", Password: "password1", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create a1: %v", err)
	}
	a2, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: "pg_a2", Password: "password1", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create a2: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	targets := []uint{a1.ID, a2.ID}
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = tc.AppCtx.DeleteUser(targets[idx])
		}(i)
	}
	wg.Wait()

	successes, lastAdmin := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			successes++
		case errors.Is(e, application_context.ErrLastAdmin):
			lastAdmin++
		default:
			t.Fatalf("unexpected error: %v", e)
		}
	}
	if successes != 1 || lastAdmin != 1 {
		t.Fatalf("want exactly one success + one ErrLastAdmin, got successes=%d lastAdmin=%d", successes, lastAdmin)
	}
	n, err := tc.AppCtx.CountEnabledAdmins()
	if err != nil {
		t.Fatalf("CountEnabledAdmins: %v", err)
	}
	if n < 1 {
		t.Fatalf("at least one enabled admin must remain, got %d", n)
	}
}
