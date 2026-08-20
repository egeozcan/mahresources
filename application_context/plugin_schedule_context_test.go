package application_context

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/plugin_system"
)

// newScheduleTestContext opens a temp-file SQLite database rather than an
// in-memory one, for the reason newHistoryTestContext documents:
// `mode=memory&cache=private` gives every pooled connection its own database, so
// a row written on one connection is invisible on the next. Every test here
// contends two writers against one row, which is exactly the shape that would
// pass for the wrong reason.
func newScheduleTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schedules.db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.PluginSchedule{}, &models.User{}, &models.Group{},
		&models.RuntimeSetting{}, &models.LogEntry{}, &models.Session{}, &models.ApiToken{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	cfg := &MahresourcesConfig{DbType: constants.DbTypeSqlite}
	return NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), cfg)
}

// seedSchedule inserts one row due at `due`, owned by owner.
func seedSchedule(t *testing.T, ctx *MahresourcesContext, plugin, id string, due time.Time, owner *uint) models.PluginSchedule {
	t.Helper()
	row := models.PluginSchedule{
		PluginName:      plugin,
		ScheduleID:      id,
		EverySeconds:    900,
		Overlap:         models.PluginScheduleOverlapSkip,
		NextDueAt:       due,
		CreatedByUserId: owner,
	}
	if err := ctx.db.Create(&row).Error; err != nil {
		t.Fatalf("seed schedule: %v", err)
	}
	return row
}

func scheduleRow(t *testing.T, ctx *MahresourcesContext, id uint) models.PluginSchedule {
	t.Helper()
	var row models.PluginSchedule
	if err := ctx.db.Where("id = ?", id).First(&row).Error; err != nil {
		t.Fatalf("read schedule %d: %v", id, err)
	}
	return row
}

// The whole feature is graded on this one. Two processes ticking the same
// database must not both run one schedule, and the only thing standing between
// them is that the claim checks and writes in a single statement.
func TestClaimPluginScheduleAdmitsOneWinner(t *testing.T) {
	ctx := newScheduleTestContext(t)
	owner := uint(1)
	past := time.Now().Add(-time.Minute)
	row := seedSchedule(t, ctx, "poller", "feed", past, &owner)

	now := time.Now()
	claimed, err := ctx.ClaimPluginSchedule(row.ID, "claim-a", now)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !claimed {
		t.Fatal("the first claim was refused, so nothing would ever run")
	}

	// The second process read the same due row before either wrote. It must lose.
	claimed, err = ctx.ClaimPluginSchedule(row.ID, "claim-b", now)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed {
		t.Fatal("both processes claimed one schedule; the handler would run twice")
	}

	// Handing the claim back is its own operation, so a dispatch that could not
	// start does not hold the slot until the TTL expires.
	if err := ctx.ReleasePluginScheduleClaim(row.ID, "claim-a"); err != nil {
		t.Fatalf("release: %v", err)
	}
	after := scheduleRow(t, ctx, row.ID)
	if after.ClaimToken != "" {
		t.Fatalf("the holder could not hand its own claim back: token is still %q", after.ClaimToken)
	}

	// And a release from someone who does not hold it changes nothing.
	if _, err := ctx.ClaimPluginSchedule(row.ID, "claim-c", time.Now()); err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if err := ctx.ReleasePluginScheduleClaim(row.ID, "claim-b"); err != nil {
		t.Fatalf("foreign release: %v", err)
	}
	if got := scheduleRow(t, ctx, row.ID).ClaimToken; got != "claim-c" {
		t.Fatalf("a non-holder released someone else's claim: token is now %q", got)
	}
}

// The claim is a race, so contend it for real rather than sequentially: a
// read-then-write implementation can pass the sequential version by luck.
func TestClaimPluginScheduleUnderConcurrencyAdmitsExactlyOne(t *testing.T) {
	ctx := newScheduleTestContext(t)
	owner := uint(1)
	row := seedSchedule(t, ctx, "poller", "feed", time.Now().Add(-time.Minute), &owner)

	const racers = 8
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(racers)

	var mu sync.Mutex
	wins := 0
	errs := []error{}

	for i := 0; i < racers; i++ {
		go func(i int) {
			defer done.Done()
			start.Wait()
			claimed, err := ctx.ClaimPluginSchedule(row.ID, fmt.Sprintf("claim-%d", i), time.Now())
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			if claimed {
				wins++
			}
		}(i)
	}
	start.Done()
	done.Wait()

	for _, err := range errs {
		t.Errorf("claim errored under contention: %v", err)
	}
	if wins != 1 {
		t.Fatalf("%d of %d racers claimed one schedule; exactly one may", wins, racers)
	}
}

// The contended test above races real goroutines, and a race can be won by luck:
// a read-then-write implementation passes it whenever the scheduler happens not
// to interleave. This one removes the luck by opening the window deliberately.
//
// A competing process claims the row *after* this claim would have made its
// decision and *before* its write lands. A compare-and-set loses that, because
// its own predicate no longer matches and it updates nothing. An implementation
// that read, decided, and then wrote does not: its write is unconditional, so it
// silently overwrites the winner and reports success to a second caller.
//
// The competing write goes out on the raw *sql.DB so it does not re-enter the
// callback that triggered it. See docs/lessons.md, "A race test that only
// observes an operation attempting a lock proves nothing".
func TestClaimPluginScheduleLosesToAClaimThatLandsAfterItsRead(t *testing.T) {
	ctx := newScheduleTestContext(t)
	owner := uint(1)
	row := seedSchedule(t, ctx, "poller", "feed", time.Now().Add(-time.Minute), &owner)

	sqlDB, err := ctx.db.DB()
	if err != nil {
		t.Fatalf("raw handle: %v", err)
	}

	const callbackName = "test:claim_interleave"
	var once sync.Once
	if err := ctx.db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "plugin_schedules" {
			return
		}
		once.Do(func() {
			if _, err := sqlDB.Exec(
				`UPDATE plugin_schedules SET claim_token = ?, claimed_at = ? WHERE id = ?`,
				"claim-other", time.Now(), row.ID,
			); err != nil {
				t.Errorf("competing claim: %v", err)
			}
		})
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Update().Remove(callbackName) })

	claimed, err := ctx.ClaimPluginSchedule(row.ID, "claim-mine", time.Now())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed {
		t.Fatal("claimed a schedule that another process took between this claim's read and its " +
			"write; both callers now believe they own the run, and the handler fires twice")
	}
	if got := scheduleRow(t, ctx, row.ID).ClaimToken; got != "claim-other" {
		t.Fatalf("the losing claim overwrote the winner: token is %q, want \"claim-other\"", got)
	}
}

// A schedule that is not due yet is not claimable, however many ticks ask.
func TestClaimPluginScheduleRefusesARowThatIsNotDue(t *testing.T) {
	ctx := newScheduleTestContext(t)
	owner := uint(1)
	row := seedSchedule(t, ctx, "poller", "feed", time.Now().Add(time.Hour), &owner)

	claimed, err := ctx.ClaimPluginSchedule(row.ID, "early", time.Now())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed {
		t.Fatal("a schedule due in an hour was claimed now")
	}
}

// A claim whose process died must not wedge the schedule forever, and a claim
// that is merely slow must not be stolen. The cutoff between those is the TTL.
func TestClaimPluginScheduleReclaimsAnAbandonedClaimButNotALiveOne(t *testing.T) {
	ctx := newScheduleTestContext(t)
	owner := uint(1)
	row := seedSchedule(t, ctx, "poller", "feed", time.Now().Add(-time.Minute), &owner)

	if claimed, err := ctx.ClaimPluginSchedule(row.ID, "holder", time.Now()); err != nil || !claimed {
		t.Fatalf("seed claim: claimed=%v err=%v", claimed, err)
	}

	// Still inside the TTL: the holder may be mid-run, and stealing it is the
	// double-fire this whole mechanism exists to prevent. Note the row is still
	// due — the claim, not the due time, is what refuses this.
	if claimed, err := ctx.ClaimPluginSchedule(row.ID, "thief", time.Now()); err != nil {
		t.Fatalf("live-claim contest: %v", err)
	} else if claimed {
		t.Fatal("a live claim was stolen; its run may still be going")
	}

	// Past the TTL the holder cannot still be running, so the slot is reclaimable
	// or the schedule is dead forever.
	abandoned := time.Now().Add(-(ScheduleClaimTTL + time.Minute))
	if err := ctx.db.Model(&models.PluginSchedule{}).Where("id = ?", row.ID).
		Update("claimed_at", abandoned).Error; err != nil {
		t.Fatalf("age the claim: %v", err)
	}
	if claimed, err := ctx.ClaimPluginSchedule(row.ID, "reclaimer", time.Now()); err != nil {
		t.Fatalf("reclaim: %v", err)
	} else if !claimed {
		t.Fatal("a claim older than the TTL was not reclaimed, so this schedule never runs again")
	}
}

// The TTL is arithmetic over the constants, not a number somebody liked. A claim
// is held for the whole run here — unlike the download-history retry slot, which
// holds one only across a submit and then asks the live queue — because
// ActionJob is in-memory and per-process, so a second process cannot ask whether
// the first one's run is still going. If the TTL can expire while a legitimate
// run is still in flight, another process reclaims the row and the handler runs
// twice, which is the one defect this feature is graded on.
func TestScheduleClaimTTLExceedsTheLongestPossibleRun(t *testing.T) {
	longest := ScheduleDispatchWait + plugin_system.MaxAsyncJobDuration
	if ScheduleClaimTTL <= longest {
		t.Fatalf("ScheduleClaimTTL is %s, but a run can legitimately take %s "+
			"(%s waiting to start + %s executing). A claim that expires mid-run is a "+
			"cross-process double-fire.", ScheduleClaimTTL, longest, ScheduleDispatchWait,
			plugin_system.MaxAsyncJobDuration)
	}

	// The inequality is only as good as the enumeration, and the enumeration was
	// wrong once: RunSchedule waited for the job slot within the dispatch wait
	// and then waited for the plugin's VM lock through LockVM, which is
	// unbounded. That third wait is in neither term, so a schedule queued behind
	// a hook's mah.http call or another async job outlived its own claim and the
	// next tick fired it again — in one process, not merely across two.
	// RunSchedule now spends a single deadline on both, which is what makes
	// ScheduleDispatchWait an honest bound on "waiting to start"; see
	// TestRunScheduleGivesUpWhenTheVMStaysBusy. Anything new that waits before
	// the handler runs belongs inside that deadline or inside this sum.
	//
	// The bound applies only while a claim is held, which is overlap="skip".
	// Under "allow" the dispatcher advances the row and releases the claim
	// before the run, so there is no claim for a wait to outlive and the VM is
	// waited for indefinitely — that policy's whole promise is that an
	// overrunning run does not cause the next to be skipped. This inequality
	// therefore says nothing about "allow", and does not need to; see
	// TestOverlapAllowWaitsOutABusyVM.
}

// The sync is what turns a plugin's declaration into the durable row. It runs on
// the context that enabled the plugin, which is the only place the operator's
// identity is available: init() runs with its Lua context deliberately removed,
// and EnablePlugin takes no actor, so mah.schedule itself cannot know who asked.
func TestSyncPluginSchedulesCreatesARowOwnedByTheEnablingOperator(t *testing.T) {
	ctx := newScheduleTestContext(t)
	operator := &auth.Principal{UserID: 7, Role: models.RoleAdmin}

	regs := []plugin_system.ScheduleRegistration{
		{PluginName: "poller", ScheduleID: "feed", Every: 15 * time.Minute, EverySeconds: 900,
			Overlap: plugin_system.ScheduleOverlapSkip},
	}
	if err := ctx.WithPrincipal(operator).SyncPluginSchedules("poller", regs); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var row models.PluginSchedule
	if err := ctx.db.Where("plugin_name = ? AND schedule_id = ?", "poller", "feed").First(&row).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.CreatedByUserId == nil || *row.CreatedByUserId != 7 {
		t.Fatalf("owner = %v, want 7; a schedule with no owner never runs", row.CreatedByUserId)
	}
	if row.EverySeconds != 900 {
		t.Errorf("EverySeconds = %d, want 900", row.EverySeconds)
	}
	if row.NextDueAt.Before(time.Now()) {
		t.Error("a new schedule was created already due, so it fires the instant it is registered")
	}
}

// A restart re-enables every plugin with no principal at all. That must not
// reassign a schedule the operator owns — to root, which the create stamp would
// otherwise supply under no-auth, or to nobody.
func TestBootLoadDoesNotOverwriteAnOperatorRecordedOwner(t *testing.T) {
	ctx := newScheduleTestContext(t)
	regs := []plugin_system.ScheduleRegistration{
		{PluginName: "poller", ScheduleID: "feed", Every: 15 * time.Minute, EverySeconds: 900,
			Overlap: plugin_system.ScheduleOverlapSkip},
	}
	operator := &auth.Principal{UserID: 7, Role: models.RoleAdmin}
	if err := ctx.WithPrincipal(operator).SyncPluginSchedules("poller", regs); err != nil {
		t.Fatalf("enable-time sync: %v", err)
	}

	// The restart: same declaration, no principal, and the interval has changed
	// so the row is genuinely rewritten rather than skipped.
	regs[0].Every = 30 * time.Minute
	regs[0].EverySeconds = 1800
	if err := ctx.SyncPluginSchedules("poller", regs); err != nil {
		t.Fatalf("boot sync: %v", err)
	}

	var row models.PluginSchedule
	if err := ctx.db.Where("plugin_name = ? AND schedule_id = ?", "poller", "feed").First(&row).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.CreatedByUserId == nil || *row.CreatedByUserId != 7 {
		t.Fatalf("owner = %v after a restart, want 7; the schedule changed hands on a reboot",
			row.CreatedByUserId)
	}
	if row.EverySeconds != 1800 {
		t.Errorf("EverySeconds = %d, want 1800; the redeclared interval was not picked up", row.EverySeconds)
	}
}

// Deleting the operator NULLs the owner across every stamped model. For a
// schedule that has to mean "stop", not "carry on unowned": an unowned timer
// would run plugin Lua on an unbound handle with no account to revoke.
func TestAScheduleWithNoOwnerIsNeverClaimed(t *testing.T) {
	ctx := newScheduleTestContext(t)
	row := seedSchedule(t, ctx, "poller", "feed", time.Now().Add(-time.Minute), nil)

	claimed, err := ctx.ClaimPluginSchedule(row.ID, "tick", time.Now())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed {
		t.Fatal("an ownerless schedule was claimed; its Lua would run with no identity to bind")
	}

	// And it is the owner that decides it: give the row one and it runs.
	owner := uint(7)
	if err := ctx.db.Model(&models.PluginSchedule{}).Where("id = ?", row.ID).
		Update("created_by_user_id", owner).Error; err != nil {
		t.Fatalf("restore owner: %v", err)
	}
	if claimed, err := ctx.ClaimPluginSchedule(row.ID, "tick", time.Now()); err != nil || !claimed {
		t.Fatalf("an owned, due schedule was not claimed: claimed=%v err=%v", claimed, err)
	}
}

// PluginSchedule has to be in the sweep that runs when a user is deleted, or the
// row keeps a dangling id and the run binds a principal for an account that is
// gone.
func TestPluginScheduleIsInTheUserDeletionSweep(t *testing.T) {
	for _, m := range stampedModels() {
		if _, ok := m.(*models.PluginSchedule); ok {
			return
		}
	}
	t.Fatal("models.PluginSchedule is not in stampedModels(), so deleting its owner leaves the " +
		"row pointing at an account that no longer exists")
}

// The next due time is now+every, not previous+every. A schedule that was due
// forty times during an outage fires once and re-bases: forty identical
// catch-up polls have the cost of forty and the value of one.
func TestCompletingARunRebasesRatherThanCatchingUp(t *testing.T) {
	ctx := newScheduleTestContext(t)
	owner := uint(1)
	// Ten hours of downtime against a fifteen-minute schedule.
	row := seedSchedule(t, ctx, "poller", "feed", time.Now().Add(-10*time.Hour), &owner)

	if claimed, err := ctx.ClaimPluginSchedule(row.ID, "tick", time.Now()); err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}
	done := time.Now()
	if err := ctx.CompletePluginScheduleRun(row.ID, "tick", models.PluginScheduleStatusCompleted, "", done); err != nil {
		t.Fatalf("complete: %v", err)
	}

	after := scheduleRow(t, ctx, row.ID)
	if after.ClaimToken != "" {
		t.Error("completing a run left the claim held, so the schedule never runs again")
	}
	if after.Runs != 1 {
		t.Errorf("Runs = %d, want 1", after.Runs)
	}
	want := done.Add(15 * time.Minute)
	if diff := after.NextDueAt.Sub(want); diff > time.Second || diff < -time.Second {
		t.Fatalf("NextDueAt is %s, want about %s. A schedule that re-bases from its old due time "+
			"would fire forty times in a burst after a ten-hour outage.", after.NextDueAt, want)
	}
}

// A sync with no operator behind it must leave the row unowned, which means
// inert. The create stamp does not do this on its own: it overwrites
// CreatedByUserId from the db context and falls back to the root admin, so a
// row created by a boot-time load would come out owned by root and run
// unattended with full database access — an identity nobody chose and nobody
// can revoke short of disabling the plugin.
func TestSyncWithNoOperatorLeavesTheScheduleUnownedAndInert(t *testing.T) {
	// Auth on, and that is the whole point of the case. With auth off the acting
	// user resolves to the root admin and a schedule owned by root is correct —
	// it is how every other no-auth create is attributed, and every account is an
	// administrator anyway. With auth on there is a real answer to "who enabled
	// this", and at boot the answer is nobody.
	ctx := newScheduleTestContext(t)
	ctx.Config.AuthEnabled = true

	// A root admin has to exist, or this test cannot see the thing it is about:
	// the create stamp would have nothing to fall back to, the column would come
	// out NULL whatever the code under test did, and the test would pass for a
	// reason unrelated to its subject.
	admin := models.User{Username: "root", Role: models.RoleAdmin, PasswordHash: "x"}
	if err := ctx.db.Create(&admin).Error; err != nil {
		t.Fatalf("seed root admin: %v", err)
	}
	ctx.refreshRootAdmin()

	regs := []plugin_system.ScheduleRegistration{
		{PluginName: "poller", ScheduleID: "feed", Every: 15 * time.Minute, EverySeconds: 900,
			Overlap: plugin_system.ScheduleOverlapSkip},
	}
	if err := ctx.SyncPluginSchedules("poller", regs); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var row models.PluginSchedule
	if err := ctx.db.Where("plugin_name = ? AND schedule_id = ?", "poller", "feed").First(&row).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.CreatedByUserId != nil {
		t.Fatalf("owner = %d for a schedule nobody enabled; it would run unattended as that user "+
			"(the create stamp supplied it, and nothing wrote the real answer back)",
			*row.CreatedByUserId)
	}

	// And inert follows from unowned, through the claim rather than through a
	// second rule that could drift from it.
	if err := ctx.db.Model(&models.PluginSchedule{}).Where("id = ?", row.ID).
		Update("next_due_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("make it due: %v", err)
	}
	if claimed, err := ctx.ClaimPluginSchedule(row.ID, "tick", time.Now()); err != nil {
		t.Fatalf("claim: %v", err)
	} else if claimed {
		t.Fatal("an unowned schedule was claimed")
	}
}
