package application_context

import (
	"errors"
	"testing"
	"time"
)

// Two checks of the read-only role can run at once. One that started before a
// GRANT and finished after a later check had found it must not replace that
// failure with its stale pass, which would reopen saved SQL for a minute.
func TestSavedSQLRoleCheckKeepsTheLaterVerdict(t *testing.T) {
	var c savedSQLRoleCheck
	early := time.Now()
	late := early.Add(time.Millisecond)
	granted := errors.New("it can SELECT from public.users")

	if got := c.store(late, granted, true); !errors.Is(got, granted) {
		t.Fatalf("store of the later check = %v, want its failure", got)
	}
	if got := c.store(early, nil, true); !errors.Is(got, granted) {
		t.Fatalf("a check that started earlier and finished later = %v, want the later check's failure kept", got)
	}
	if !errors.Is(c.err, granted) || !c.checkedAt.Equal(late) {
		t.Fatalf("stored verdict = %v at %v, want the later failure at %v", c.err, c.checkedAt, late)
	}
	// A check that started after it replaces it, pass or fail.
	if got := c.store(late.Add(time.Millisecond), nil, true); got != nil {
		t.Fatalf("a newer passing check = %v, want nil", got)
	}
}
