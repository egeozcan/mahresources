//go:build postgres && json1 && fts5

package application_context

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"

	"mahresources/constants"
	"mahresources/models"
)

// The claim is one conditional UPDATE whose RowsAffected is the whole answer,
// and RowsAffected is exactly where the two supported databases can differ: a
// dialect that reported a matched-but-unchanged row as affected would hand the
// same schedule to two processes. SQLite alone cannot show that.
//
// The predicates carry dialect risk of their own — COALESCE over a nullable
// text column, a comparison against a NULL timestamp, and `runs = runs + 1` as
// an expression rather than a value.

func newPostgresScheduleContext(t *testing.T) *MahresourcesContext {
	t.Helper()

	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(&models.PluginSchedule{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open read-only handle: %v", err)
	}
	t.Cleanup(func() { readOnly.Close() })

	return NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{
		DbType: constants.DbTypePosgres,
	})
}

func seedPGSchedule(t *testing.T, ctx *MahresourcesContext, id string, due time.Time, owner *uint) models.PluginSchedule {
	t.Helper()
	row := models.PluginSchedule{
		PluginName: "poller", ScheduleID: id, EverySeconds: 900,
		Overlap: models.PluginScheduleOverlapSkip, NextDueAt: due, CreatedByUserId: owner,
	}
	if err := ctx.db.Create(&row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return row
}

func TestClaimPluginSchedulePG_AdmitsOneWinner(t *testing.T) {
	ctx := newPostgresScheduleContext(t)
	owner := uint(7)
	row := seedPGSchedule(t, ctx, "feed", time.Now().Add(-time.Minute), &owner)

	const racers = 8
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(racers)

	var mu sync.Mutex
	wins := 0
	for i := 0; i < racers; i++ {
		go func(i int) {
			defer done.Done()
			start.Wait()
			claimed, err := ctx.ClaimPluginSchedule(row.ID, fmt.Sprintf("claim-%d", i), time.Now())
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			if claimed {
				wins++
			}
		}(i)
	}
	start.Done()
	done.Wait()

	if wins != 1 {
		t.Fatalf("%d of %d processes claimed one schedule on Postgres; exactly one may, or the "+
			"handler runs twice in a multi-process deployment", wins, racers)
	}
}

// The three predicates, on the dialect where NULL comparisons and COALESCE over
// a nullable text column are most likely to behave differently.
func TestClaimPluginSchedulePG_RefusesNotDueUnownedAndHeld(t *testing.T) {
	ctx := newPostgresScheduleContext(t)
	owner := uint(7)

	notDue := seedPGSchedule(t, ctx, "later", time.Now().Add(time.Hour), &owner)
	if claimed, err := ctx.ClaimPluginSchedule(notDue.ID, "tick", time.Now()); err != nil {
		t.Fatalf("not-due claim: %v", err)
	} else if claimed {
		t.Error("claimed a schedule that is not due")
	}

	// A NULL owner, against a NULL claimed_at: both nullable columns in one row.
	unowned := seedPGSchedule(t, ctx, "orphan", time.Now().Add(-time.Minute), nil)
	if claimed, err := ctx.ClaimPluginSchedule(unowned.ID, "tick", time.Now()); err != nil {
		t.Fatalf("unowned claim: %v", err)
	} else if claimed {
		t.Error("claimed a schedule whose owner has been deleted")
	}

	held := seedPGSchedule(t, ctx, "busy", time.Now().Add(-time.Minute), &owner)
	if claimed, err := ctx.ClaimPluginSchedule(held.ID, "first", time.Now()); err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}
	if claimed, err := ctx.ClaimPluginSchedule(held.ID, "second", time.Now()); err != nil {
		t.Fatalf("second claim: %v", err)
	} else if claimed {
		t.Error("claimed a schedule another process is running")
	}
}

// runs = runs + 1 is an expression rather than a value, and the completion has
// to release the claim and advance the due time in the same statement.
func TestCompletePluginScheduleRunPG_AdvancesReleasesAndCounts(t *testing.T) {
	ctx := newPostgresScheduleContext(t)
	owner := uint(7)
	row := seedPGSchedule(t, ctx, "feed", time.Now().Add(-10*time.Hour), &owner)

	if claimed, err := ctx.ClaimPluginSchedule(row.ID, "tick", time.Now()); err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}
	done := time.Now()
	if err := ctx.CompletePluginScheduleRun(row.ID, "tick", models.PluginScheduleStatusCompleted, "", done); err != nil {
		t.Fatalf("complete: %v", err)
	}

	var after models.PluginSchedule
	if err := ctx.db.Where("id = ?", row.ID).First(&after).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if after.ClaimToken != "" {
		t.Error("the claim was not released")
	}
	if after.Runs != 1 {
		t.Errorf("Runs = %d, want 1", after.Runs)
	}
	if after.LastStatus != models.PluginScheduleStatusCompleted {
		t.Errorf("LastStatus = %q", after.LastStatus)
	}
	if want := done.Add(15 * time.Minute); after.NextDueAt.Sub(want) > 2*time.Second || after.NextDueAt.Sub(want) < -2*time.Second {
		t.Errorf("NextDueAt = %s, want about %s (re-base, not catch-up)", after.NextDueAt, want)
	}
}

// The run-now claim on Postgres.
//
// It is the same conditional UPDATE with one predicate removed, so it rests on
// the same RowsAffected semantics the tests above exist for — and it is the
// variant an operator can fire repeatedly from a button, which makes "exactly
// one winner" the property that matters most here.
func TestClaimPluginScheduleNowPG_TakesNotDueButKeepsTheOtherTwo(t *testing.T) {
	ctx := newPostgresScheduleContext(t)
	owner := uint(7)

	// The whole difference: a row an hour away, which the ticked claim refuses.
	notDue := seedPGSchedule(t, ctx, "later", time.Now().Add(time.Hour), &owner)
	if claimed, err := ctx.ClaimPluginSchedule(notDue.ID, "tick", time.Now()); err != nil {
		t.Fatalf("ticked claim: %v", err)
	} else if claimed {
		t.Fatal("the ticker claimed a row due in an hour, so this proves nothing about the run-now variant")
	}
	if claimed, err := ctx.ClaimPluginScheduleNow(notDue.ID, "manual", time.Now()); err != nil {
		t.Fatalf("run-now claim: %v", err)
	} else if !claimed {
		t.Fatal("a manual run could not claim a row that is not yet due, which is every row the control is for")
	}

	// A NULL owner against a NULL claimed_at, the two-nullable-column row.
	unowned := seedPGSchedule(t, ctx, "orphan", time.Now().Add(time.Hour), nil)
	if claimed, err := ctx.ClaimPluginScheduleNow(unowned.ID, "manual", time.Now()); err != nil {
		t.Fatalf("unowned run-now claim: %v", err)
	} else if claimed {
		t.Error("a manual run claimed a schedule whose owner has been deleted")
	}

	held := seedPGSchedule(t, ctx, "busy", time.Now().Add(time.Hour), &owner)
	if claimed, err := ctx.ClaimPluginSchedule(held.ID, "tick", time.Now().Add(2*time.Hour)); err != nil || !claimed {
		t.Fatalf("seeding a live claim: claimed=%v err=%v", claimed, err)
	}
	if claimed, err := ctx.ClaimPluginScheduleNow(held.ID, "manual", time.Now()); err != nil {
		t.Fatalf("held run-now claim: %v", err)
	} else if claimed {
		t.Error("a manual run claimed a schedule a tick was already running; overlap=skip promises this cannot happen")
	}
}

// Two operators clicking "Run now" in the same instant is the reachable version
// of the multi-process race, and it is easier to reach than the ticked one: the
// button is not rate-limited and there is no clock spacing the attempts.
func TestClaimPluginScheduleNowPG_AdmitsOneWinner(t *testing.T) {
	ctx := newPostgresScheduleContext(t)
	owner := uint(7)
	row := seedPGSchedule(t, ctx, "feed", time.Now().Add(time.Hour), &owner)

	const racers = 8
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(racers)

	var mu sync.Mutex
	wins := 0
	for i := 0; i < racers; i++ {
		go func(i int) {
			defer done.Done()
			start.Wait()
			claimed, err := ctx.ClaimPluginScheduleNow(row.ID, fmt.Sprintf("manual-%d", i), time.Now())
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				t.Errorf("run-now claim: %v", err)
				return
			}
			if claimed {
				wins++
			}
		}(i)
	}
	start.Done()
	done.Wait()

	if wins != 1 {
		t.Fatalf("%d of %d concurrent run-now requests claimed one schedule; exactly one may, "+
			"or two copies of the handler run at once", wins, racers)
	}
}
