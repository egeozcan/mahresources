//go:build postgres

package api_tests

import (
	"errors"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
)

// loginLockBoundCeiling is far above the two-second bound the login asks for and
// far below "waits for the holder": pg_advisory_xact_lock has no timeout of its
// own, so without SET LOCAL lock_timeout the login blocks until the mutation in
// front of it commits — which, for MergeGroups or BulkDeleteGroups over a large
// tree, can be minutes.
const loginLockBoundCeiling = 10 * time.Second

// The SQLite-side test for this path injects the contention error at the session
// insert. That proves the 503 is reported and not charged to the limiter, but it
// says nothing about the bound: delete lockUserManagementMutationBounded's
// SET LOCAL and the injected error still fires, while a real PostgreSQL login
// queues behind the group operation holding the lock with no deadline at all.
// This holds the advisory lock for real and asserts the login gives up.
func TestPostgresLoginDoesNotQueueBehindAHeldMutationLock(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	if _, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "pg-login-bounded", Password: "password1", Role: models.RoleEditor,
	}); err != nil {
		t.Fatalf("create logging-in user: %v", err)
	}
	blocker, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "pg-lock-holder", Password: "password1", Role: models.RoleEditor,
	})
	if err != nil {
		t.Fatalf("create lock-holding user: %v", err)
	}

	barrier := installPostgresMutationBarrier(t, tc.DB, "login-lock-bound")
	holderDone := make(chan error, 1)
	go func() { holderDone <- tc.AppCtx.SetUserPassword(blocker.ID, "password2") }()
	waitBarrier(t, barrier.acquired, "the blocking mutation never took the advisory lock")

	loginDone := make(chan error, 1)
	go func() {
		_, _, _, loginErr := tc.AppCtx.AuthenticateAndCreateSession(
			"pg-login-bounded", "password1", time.Hour, "agent", "127.0.0.1")
		loginDone <- loginErr
	}()

	select {
	case loginErr := <-loginDone:
		if !errors.Is(loginErr, application_context.ErrLoginUnavailable) {
			t.Fatalf("login against a held mutation lock err=%v, want ErrLoginUnavailable", loginErr)
		}
	case <-time.After(loginLockBoundCeiling):
		t.Fatalf("login queued behind the held mutation lock for over %v instead of failing fast", loginLockBoundCeiling)
	}

	barrier.releaseOwner()
	if err := <-holderDone; err != nil {
		t.Fatalf("blocking mutation: %v", err)
	}

	// The bound is a deadline, not a permanent refusal: once the lock is free the
	// same credential authenticates.
	if _, _, _, err := tc.AppCtx.AuthenticateAndCreateSession(
		"pg-login-bounded", "password1", time.Hour, "agent", "127.0.0.1"); err != nil {
		t.Fatalf("login after the lock was released: %v", err)
	}
}
