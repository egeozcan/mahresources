package application_context

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/plugin_system"
)

// How long a tick will wait for a background job slot before giving the
// schedule back to the next tick.
//
// It exists because the claim below is held across the dispatch, and the job
// budget's own acquire is a blocking send with no escape: waiting there while
// holding a claim would make the claim's lifetime unbounded, and an unbounded
// claim cannot have a TTL that means anything.
const ScheduleDispatchWait = 10 * time.Second

// ScheduleClaimTTL is how long a claim is honoured before another process may
// treat it as abandoned.
//
// It is derived, not chosen. A claim on a schedule is held for the *whole run*,
// which is what makes this different from the download-history retry slot it is
// otherwise copied from: that slot is held only from claim to submit, because
// afterwards "is it still running" can be answered by looking in the live queue.
// Here it cannot. ActionJob is in-memory and per-process, so a second process
// has no way to ask whether the first one's run is still going, and the claim is
// the only thing that says so.
//
// Therefore the TTL must exceed the longest run that is still legitimate: the
// bounded wait for a job slot, plus the full execution allowance, plus a margin
// for the write that releases it. Too short is a cross-process double-fire.
// TestScheduleClaimTTLExceedsTheLongestPossibleRun fails the build if a change
// to either term stops that holding, which is why this is an expression and not
// a literal.
const ScheduleClaimTTL = ScheduleDispatchWait + plugin_system.MaxAsyncJobDuration + 2*time.Minute

// The four ways a manual run is refused.
//
// They are sentinels rather than messages because api_handlers classifies by
// type: TestStatusCodeForError_AuthorizationRefusalsAre403 exists precisely
// because a status derived from wording changes the day someone rewrites a
// sentence. Each says something different to an operator, and the difference is
// what the manage page and the CLI report:
//
//   - NotFound:    no such row. The plugin has never declared that id.
//   - NotDeclared: the row is there, but nothing live answers to it — the plugin
//     is disabled, or was downgraded, or renamed the id.
//   - Unowned:     the row has no operator to run as, so it has stopped. This is
//     the deliberate resolution of "the owner was deleted", and running it as the
//     admin who clicked would be exactly the fallback the model refuses.
//   - Busy:        someone already holds the claim. A tick is running it, or
//     another operator clicked first.
//
// Busy is not an error in the sense of something being wrong; it is the correct
// answer to "run a second copy of this", and overlap = "skip" is the promise it
// keeps.
var (
	ErrScheduleNotFound    = errors.New("no such plugin schedule")
	ErrScheduleNotDeclared = errors.New("the plugin does not currently declare this schedule")
	ErrScheduleUnowned     = errors.New("this schedule has stopped because it has no owner")
	ErrScheduleBusy        = errors.New("this schedule is already running")
)

// ClaimPluginSchedule takes a due schedule's run slot, and reports whether it
// got it.
//
// One conditional UPDATE, whose RowsAffected is the answer. Reading the row,
// deciding it is claimable and then writing would be three statements for one
// decision, and two processes ticking a shared database would both pass the read
// — which is the whole failure this exists to prevent. See docs/lessons.md, "A
// control that reads state, decides, and then writes is one operation".
//
// The claim is taken before the dispatch rather than recorded after it, for the
// same reason the download retry slot is: the duplicate is created by the
// dispatch, so a marker written afterwards is a note about a race that has
// already happened.
//
// Three predicates, each refusing a different thing:
//
//   - due:     next_due_at <= now.
//   - owned:   a row whose owner has been deleted is nobody's. It stops rather
//     than falling back to an unbound handle — see the model's comment.
//   - unheld:  no live claim. A claim older than ScheduleClaimTTL cannot still
//     be running, so it is treated as abandoned rather than wedging the schedule
//     forever.
func (ctx *MahresourcesContext) ClaimPluginSchedule(id uint, claimToken string, now time.Time) (bool, error) {
	return ctx.claimPluginSchedule(id, claimToken, now, true)
}

// ClaimPluginScheduleNow takes the run slot for a run an operator asked for.
//
// The same statement as ClaimPluginSchedule minus the due predicate, because
// "run it now" is a request to ignore next_due_at and nothing else about the
// row. It keeps the other two, and each for a reason that a manual run makes
// sharper rather than weaker:
//
//   - owned, because a manual run executes as the operator who enabled the
//     plugin, exactly as a scheduled one does. Falling back to whoever clicked
//     the button would turn this control into a way to run plugin code as
//     yourself on a schedule whose owner never granted that — and an unowned row
//     is stopped, which is a state an operator can see on the page before
//     clicking.
//   - unheld, because a manual run that ignored a live claim would start a
//     second copy alongside a tick already running, which is the one thing
//     overlap = "skip" promises cannot happen. The claim is the only thing that
//     knows: ActionJob is per-process and in-memory.
//
// It also does not advance next_due_at, here or when the run finishes. A manual
// run is an extra run, not a re-phasing of the cadence, so the schedule stays on
// whatever clock it was already on.
func (ctx *MahresourcesContext) ClaimPluginScheduleNow(id uint, claimToken string, now time.Time) (bool, error) {
	return ctx.claimPluginSchedule(id, claimToken, now, false)
}

// claimPluginSchedule is the compare-and-set both claim paths share.
//
// requireDue is a parameter rather than a second copy of the statement because
// the other two predicates are the safety ones. A claim that stopped checking
// ownership, or stopped checking for a live claim, would be wrong on both paths
// — and a rule written twice is a rule that drifts. Only the due check is
// actually a difference of intent between a tick and an operator.
func (ctx *MahresourcesContext) claimPluginSchedule(id uint, claimToken string, now time.Time, requireDue bool) (bool, error) {
	if claimToken == "" {
		return false, errors.New("a schedule claim needs a token nobody else can produce")
	}
	q := ctx.db.Model(&models.PluginSchedule{}).Where("id = ?", id)
	if requireDue {
		q = q.Where("next_due_at <= ?", now)
	}
	res := q.
		Where("created_by_user_id IS NOT NULL").
		Where("COALESCE(claim_token, '') = '' OR claimed_at IS NULL OR claimed_at < ?",
			now.Add(-ScheduleClaimTTL)).
		Updates(map[string]any{"claim_token": claimToken, "claimed_at": now})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// ReleasePluginScheduleClaim hands a claim back without advancing the schedule,
// for a dispatch that could not start.
//
// Conditional on still holding it. A tick whose claim has already been taken
// over by another process — because this one overran the TTL — must not clear
// that process's claim on its way out; it has lost the row and releasing it
// would hand a running schedule to a third.
//
// A release that matches nothing is not an error. The row may have been deleted,
// or the claim already reclaimed, and both are ordinary.
func (ctx *MahresourcesContext) ReleasePluginScheduleClaim(id uint, claimToken string) error {
	return ctx.db.Model(&models.PluginSchedule{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Updates(map[string]any{"claim_token": "", "claimed_at": nil}).Error
}

// CompletePluginScheduleRun records an outcome, advances the schedule and
// releases the claim, in one statement so a schedule cannot end up advanced but
// still claimed (or released but not advanced).
//
// The next due time is `now + every`, not `previous + every`. A schedule that
// was due forty times during an outage therefore fires once and re-bases, rather
// than firing forty times in a burst on the way back up. That is a decision, and
// it is the one a poller wants: forty identical catch-up polls have the cost of
// forty and the value of one.
//
// Conditional on holding the claim, for the reason ReleasePluginScheduleClaim
// is.
func (ctx *MahresourcesContext) CompletePluginScheduleRun(id uint, claimToken, status, runErr string, now time.Time) error {
	var row models.PluginSchedule
	if err := ctx.db.Where("id = ? AND claim_token = ?", id, claimToken).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	every := time.Duration(row.EverySeconds) * time.Second
	if every <= 0 {
		every = time.Minute
	}
	return ctx.db.Model(&models.PluginSchedule{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Updates(map[string]any{
			"claim_token": "",
			"claimed_at":  nil,
			"next_due_at": now.Add(every),
			"last_run_at": now,
			"last_status": status,
			"last_error":  runErr,
			"runs":        gorm.Expr("runs + 1"),
		}).Error
}

// AdvancePluginScheduleAtDispatch is the overlap = "allow" half of the same
// bookkeeping: the schedule's next due time moves as soon as the run is handed
// off, and the claim is released immediately, so the following tick may start
// another run while this one is still going.
//
// "Allow" buys queueing, not concurrency. Two runs of one plugin still serialize
// on that plugin's VM lock, which is the guarantee plugin-lua-api.md states.
// What it actually buys is that a run overrunning its interval does not cause
// the next one to be skipped.
func (ctx *MahresourcesContext) AdvancePluginScheduleAtDispatch(id uint, claimToken string, now time.Time) error {
	var row models.PluginSchedule
	if err := ctx.db.Where("id = ? AND claim_token = ?", id, claimToken).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	every := time.Duration(row.EverySeconds) * time.Second
	if every <= 0 {
		every = time.Minute
	}
	return ctx.db.Model(&models.PluginSchedule{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Updates(map[string]any{
			"claim_token": "",
			"claimed_at":  nil,
			"next_due_at": now.Add(every),
		}).Error
}

// SyncPluginSchedules reconciles the durable rows for one plugin against what it
// has just declared.
//
// It runs on the context that enabled the plugin, and that is not an incidental
// detail: it is the only place the operator's identity exists. loadPlugin
// removes the Lua context before calling init() — gopher-lua copies a parent
// context into a coroutine at creation and never refreshes it — and EnablePlugin
// takes no actor, so mah.schedule itself has no way to know who asked. The
// registration records the handler; this records who it runs as.
//
// Two rules, both of which are about a restart:
//
//   - The owner is written only when there is one, and never cleared. Every
//     plugin is re-enabled at boot with no principal, and the create stamp's
//     no-auth default is the root admin — so without this a reboot would either
//     blank the owner or quietly promote every schedule to root.
//   - Everything else is overwritten, because the plugin file is the authority
//     on its own interval and overlap policy. Only the claim and the due time
//     are left alone, since a sync can land while a run is in flight.
//
// Rows this plugin no longer declares are deliberately NOT deleted. They may be
// a downgrade that will be rolled back, and a row with no live registration is
// already inert — nothing claims it. Deleting them would throw away the operator
// binding for a schedule that is about to come back.
func (ctx *MahresourcesContext) SyncPluginSchedules(pluginName string, regs []plugin_system.ScheduleRegistration) error {
	owner := ctx.actingUserIDPtr()
	now := time.Now()

	for _, reg := range regs {
		overlap := reg.Overlap
		if !models.ValidPluginScheduleOverlap(overlap) {
			overlap = models.PluginScheduleOverlapSkip
		}

		var existing models.PluginSchedule
		err := ctx.db.Where("plugin_name = ? AND schedule_id = ?", pluginName, reg.ScheduleID).
			First(&existing).Error
		switch {
		case err == nil:
			updates := map[string]any{
				"every_seconds": reg.EverySeconds,
				"overlap":       overlap,
			}
			// Only ever set, never clear. See the second rule above.
			if owner != nil {
				updates["created_by_user_id"] = *owner
			}
			if err := ctx.db.Model(&models.PluginSchedule{}).Where("id = ?", existing.ID).
				Updates(updates).Error; err != nil {
				return fmt.Errorf("updating schedule %q/%q: %w", pluginName, reg.ScheduleID, err)
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			row := models.PluginSchedule{
				PluginName:   pluginName,
				ScheduleID:   reg.ScheduleID,
				EverySeconds: reg.EverySeconds,
				Overlap:      overlap,
				// One interval from now, not now: a schedule should not fire the
				// instant it is registered, and least of all on every restart.
				NextDueAt:       now.Add(time.Duration(reg.EverySeconds) * time.Second),
				CreatedByUserId: owner,
			}
			if err := ctx.db.Create(&row).Error; err != nil {
				return fmt.Errorf("creating schedule %q/%q: %w", pluginName, reg.ScheduleID, err)
			}
			// CreatedByUserId is set on the struct above and then overwritten by
			// the create stamp, which is fine, because the two agree in every
			// path: both read the acting user first and fall back to
			// defaultActorID. Setting it here is a statement of intent that costs
			// nothing, not a correction.
			//
			// What makes the auth-on boot case come out NULL is defaultActorID's
			// own rule — it returns 0 whenever auth is enabled — and not anything
			// this function does. An earlier version of this wrote the owner back
			// over the stamp to "fix" that case; it was measured and it changed
			// nothing in any path, so it is gone rather than sitting here looking
			// load-bearing. Under no-auth both write the root admin, which is how
			// every other no-auth create is attributed.
		default:
			return fmt.Errorf("reading schedule %q/%q: %w", pluginName, reg.ScheduleID, err)
		}
	}
	return nil
}

// DuePluginSchedules lists the rows a tick should try to claim.
//
// It is a hint, not a decision: two processes run this same query and get the
// same rows. What decides is ClaimPluginSchedule, whose predicates repeat every
// condition here. Filtering twice is deliberate — asking the database for the
// short list is much cheaper than attempting a claim on every row, and repeating
// the predicates on the claim is what makes the attempt safe.
func (ctx *MahresourcesContext) DuePluginSchedules(now time.Time, limit int) ([]models.PluginSchedule, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []models.PluginSchedule
	err := ctx.db.
		Where("next_due_at <= ?", now).
		Where("created_by_user_id IS NOT NULL").
		Where("COALESCE(claim_token, '') = '' OR claimed_at IS NULL OR claimed_at < ?",
			now.Add(-ScheduleClaimTTL)).
		Order("next_due_at asc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// RecordPluginScheduleOutcome writes what a run did, without touching the claim
// or the due time.
//
// It is the overlap = "allow" half. That policy releases the claim and advances
// the schedule at dispatch, so by the time the run finishes it holds nothing,
// and a claim-conditional write would match no rows. The trade is deliberate:
// "allow" gives up the ability to say "this exact run finished" in exchange for
// not skipping an interval when a run overruns.
func (ctx *MahresourcesContext) RecordPluginScheduleOutcome(id uint, status, runErr string, now time.Time) error {
	return ctx.db.Model(&models.PluginSchedule{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_run_at": now,
			"last_status": status,
			"last_error":  runErr,
			"runs":        gorm.Expr("runs + 1"),
		}).Error
}

// PluginSchedulesFor lists a plugin's stored schedules by id, for the manage
// page.
//
// No visibility predicate: /plugins/manage is admin-only (isSystemPath), and an
// operator managing plugins is looking at the deployment rather than at their
// own work. What the surfaces render is whether a row still *has* an owner — an
// unowned row shows as stopped, because nothing claims it — not who that owner
// is. A row can be unowned either because the operator's account was deleted or
// because it was created by a principal-less sync, so the badge deliberately
// does not name a cause.
func (ctx *MahresourcesContext) PluginSchedulesFor(pluginName string) ([]models.PluginSchedule, error) {
	var rows []models.PluginSchedule
	err := ctx.db.Where("plugin_name = ?", pluginName).Order("schedule_id asc").Find(&rows).Error
	return rows, err
}

// RunPluginScheduleNow starts one schedule outside its own cadence.
//
// It is a method on the context rather than a call into the scheduler because
// the scheduler is not reachable from the HTTP layer: it is built from this
// context, started in main, and held nowhere else. Routing through here keeps
// the seam the rest of the tree uses — handlers depend on a contracts interface
// this context satisfies — instead of threading a second object down beside it.
//
// It returns as soon as the run has started; see PluginScheduler.RunNow.
func (ctx *MahresourcesContext) RunPluginScheduleNow(pluginName, scheduleID string) error {
	if ctx.pluginScheduler == nil {
		// Every deployment that loads plugins starts one, so this is the test
		// harness and the -skip case rather than a state an operator can reach.
		return fmt.Errorf("%w: the plugin scheduler is not running", ErrScheduleNotDeclared)
	}
	return ctx.pluginScheduler.RunNow(pluginName, scheduleID)
}

// PluginScheduleByKey reads one row by the pairing the two halves meet on.
//
// It returns ErrScheduleNotFound rather than gorm.ErrRecordNotFound so the
// refusal survives the trip up to the HTTP layer as a type, which is how
// statusCodeForError is required to classify: by type, never by wording.
func (ctx *MahresourcesContext) PluginScheduleByKey(pluginName, scheduleID string) (*models.PluginSchedule, error) {
	var row models.PluginSchedule
	err := ctx.db.Where("plugin_name = ? AND schedule_id = ?", pluginName, scheduleID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: %s/%s", ErrScheduleNotFound, pluginName, scheduleID)
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
