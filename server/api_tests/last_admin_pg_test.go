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
