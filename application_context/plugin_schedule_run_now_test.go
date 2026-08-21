package application_context

import (
	"errors"
	"testing"
	"time"

	"mahresources/models"
)

// The one thing the run-now claim does differently, and the whole reason it
// exists: it takes a row that is not due.
//
// Recorded mutation: put `next_due_at <= ?` back into claimPluginSchedule's
// unconditional path (or pass requireDue = true from ClaimPluginScheduleNow) and
// this fails. That mutation is not hypothetical — it is what reusing
// ClaimPluginSchedule unchanged would do, and it is silent: the button returns
// "started" and nothing runs, for every schedule that is not already due, which
// is every schedule the control exists for.
func TestClaimPluginScheduleNowTakesARowThatIsNotDue(t *testing.T) {
	ctx := newScheduleTestContext(t)
	owner := uint(1)
	future := time.Now().Add(time.Hour)
	row := seedSchedule(t, ctx, "poller", "feed", future, &owner)

	// The ticker refuses it, which is the control: this row is genuinely not due.
	claimed, err := ctx.ClaimPluginSchedule(row.ID, "tick", time.Now())
	if err != nil {
		t.Fatalf("ticked claim: %v", err)
	}
	if claimed {
		t.Fatal("the ticker claimed a row due in an hour; the test proves nothing about the run-now variant")
	}

	claimed, err = ctx.ClaimPluginScheduleNow(row.ID, "manual", time.Now())
	if err != nil {
		t.Fatalf("run-now claim: %v", err)
	}
	if !claimed {
		t.Fatal("a manual run could not claim a row that is not yet due, which is every row the control is for")
	}
}

// Ownership survives into the run-now variant.
//
// Recorded mutation: drop `created_by_user_id IS NOT NULL` from
// claimPluginSchedule and this fails. An unowned schedule has stopped, and the
// model's own comment says why running it as anyone else is not an option: there
// is no identity to fall back to that the operator ever granted.
func TestClaimPluginScheduleNowRefusesAnUnownedRow(t *testing.T) {
	ctx := newScheduleTestContext(t)
	row := seedSchedule(t, ctx, "poller", "orphan", time.Now().Add(-time.Minute), nil)

	claimed, err := ctx.ClaimPluginScheduleNow(row.ID, "manual", time.Now())
	if err != nil {
		t.Fatalf("run-now claim: %v", err)
	}
	if claimed {
		t.Fatal("a manual run claimed a schedule with no owner; it would run as nobody, or as whoever asked")
	}
}

// And so does the live-claim check, which is what keeps overlap = "skip" true
// when the request comes from a button instead of from the clock.
//
// Recorded mutation: drop the COALESCE(claim_token…) predicate and this fails —
// a manual run would start a second copy alongside a tick already running.
func TestClaimPluginScheduleNowRefusesARowSomeoneIsAlreadyRunning(t *testing.T) {
	ctx := newScheduleTestContext(t)
	owner := uint(1)
	row := seedSchedule(t, ctx, "poller", "feed", time.Now().Add(-time.Minute), &owner)

	claimed, err := ctx.ClaimPluginSchedule(row.ID, "tick", time.Now())
	if err != nil || !claimed {
		t.Fatalf("the ticker could not take the claim this test needs held: claimed=%v err=%v", claimed, err)
	}

	claimed, err = ctx.ClaimPluginScheduleNow(row.ID, "manual", time.Now())
	if err != nil {
		t.Fatalf("run-now claim: %v", err)
	}
	if claimed {
		t.Fatal("a manual run claimed a schedule a tick was already running; overlap=skip promises this cannot happen")
	}
}

// A claim old enough to be abandoned is reclaimable by a manual run, exactly as
// it is by a tick. Without this a crashed process would wedge the button for
// ScheduleClaimTTL with no way to say so.
func TestClaimPluginScheduleNowReclaimsAnAbandonedClaim(t *testing.T) {
	ctx := newScheduleTestContext(t)
	owner := uint(1)
	row := seedSchedule(t, ctx, "poller", "feed", time.Now().Add(time.Hour), &owner)

	stale := time.Now().Add(-ScheduleClaimTTL - time.Minute)
	if err := ctx.db.Model(&models.PluginSchedule{}).Where("id = ?", row.ID).
		Updates(map[string]any{"claim_token": "dead-process", "claimed_at": stale}).Error; err != nil {
		t.Fatalf("age the claim: %v", err)
	}

	claimed, err := ctx.ClaimPluginScheduleNow(row.ID, "manual", time.Now())
	if err != nil {
		t.Fatalf("run-now claim: %v", err)
	}
	if !claimed {
		t.Fatal("a claim older than the TTL wedged the run-now control; a crashed process would disable it for over seven minutes")
	}
}

// The bookkeeping half: a manual run records what it did without moving the
// cadence.
//
// Recorded mutation: swap RecordPluginScheduleOutcome for
// CompletePluginScheduleRun in dispatchManual — the natural mistake, since it is
// the call the "skip" path makes and the one named "complete" — and the
// next-due-at assertion fails. AdvancePluginScheduleAtDispatch fails it the same
// way, which matters because it is what copying dispatch's "allow" branch would
// reach for, and the audit names only CompletePluginScheduleRun.
func TestAManualRunRecordsItsOutcomeWithoutRebasingTheCadence(t *testing.T) {
	ctx := newScheduleTestContext(t)
	owner := uint(1)
	due := time.Now().Add(42 * time.Minute).Truncate(time.Second)
	row := seedSchedule(t, ctx, "poller", "feed", due, &owner)

	claimed, err := ctx.ClaimPluginScheduleNow(row.ID, "manual", time.Now())
	if err != nil || !claimed {
		t.Fatalf("run-now claim: claimed=%v err=%v", claimed, err)
	}

	// What dispatchManual does when a run finishes, in the order it does it:
	// record while still holding the claim, then release.
	if err := ctx.RecordPluginScheduleOutcome(row.ID, models.PluginScheduleStatusCompleted, "", time.Now()); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := ctx.ReleasePluginScheduleClaim(row.ID, "manual"); err != nil {
		t.Fatalf("release: %v", err)
	}

	after := scheduleRow(t, ctx, row.ID)
	if !after.NextDueAt.Truncate(time.Second).Equal(due) {
		t.Fatalf("a manual run re-phased the schedule: next due was %s, is now %s. "+
			"An extra run is not a re-phasing, and the operator's cadence is not the button's to change.",
			due, after.NextDueAt)
	}
	if after.Runs != 1 {
		t.Fatalf("runs = %d, want 1: a manual run is a run, and the row is where its history lives", after.Runs)
	}
	if after.LastStatus != models.PluginScheduleStatusCompleted {
		t.Fatalf("lastStatus = %q, want %q", after.LastStatus, models.PluginScheduleStatusCompleted)
	}
	if after.ClaimToken != "" {
		t.Fatalf("the claim was not handed back: token is still %q, so nothing runs for the next %s",
			after.ClaimToken, ScheduleClaimTTL)
	}
}

// The refusals reach the HTTP layer as types, because that is what
// statusCodeForError classifies on. A refusal that arrived as a bare string
// would land on the substring scan, and "no such plugin schedule" contains no
// "not found" — it would be answered 500, an outage's status for an answer that
// is simply no.
func TestRunPluginScheduleNowRefusalsAreTyped(t *testing.T) {
	ctx := newScheduleTestContext(t)

	_, err := ctx.PluginScheduleByKey("poller", "nope")
	if !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("reading an absent schedule gave %v, want ErrScheduleNotFound", err)
	}

	// No plugin manager on a bare test context, which is the same shape as a
	// deployment with plugins switched off: the schedule is not declared.
	err = ctx.RunPluginScheduleNow("poller", "feed")
	if !errors.Is(err, ErrScheduleNotDeclared) {
		t.Fatalf("running a schedule with no plugin manager gave %v, want ErrScheduleNotDeclared", err)
	}
}

// A manual run must not be able to hold its claim past the TTL either, and the
// only thing that guarantees that is passing the same dispatch wait the ticked
// path passes. This pins the pairing rather than the arithmetic —
// TestScheduleClaimTTLExceedsTheLongestPossibleRun owns the inequality — so that
// a run-now path which quietly used a different wait is caught here.
func TestManualDispatchUsesTheSameBoundedWaitAsATick(t *testing.T) {
	ctx := newScheduleTestContext(t)
	s := NewPluginScheduler(ctx, time.Minute)
	if s.dispatchWait != ScheduleDispatchWait {
		t.Fatalf("dispatchWait = %s, want %s. Both dispatch paths spend this one budget, and "+
			"ScheduleClaimTTL is derived from it; a run-now that waited differently would make "+
			"TestScheduleClaimTTLExceedsTheLongestPossibleRun a statement about a path it does not cover.",
			s.dispatchWait, ScheduleDispatchWait)
	}
}
