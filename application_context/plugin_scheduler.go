package application_context

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
)

// DefaultScheduleTick is how often the scheduler looks for due work.
//
// It bounds the resolution of every schedule, which is why plugins may not
// declare an interval shorter than plugin_system.MinScheduleInterval: a floor on
// what can be asked for is honest, and silently running a 5-second poller every
// 30 seconds is not.
const DefaultScheduleTick = 30 * time.Second

// scheduleDrainTimeout bounds how long Stop waits for in-flight runs.
//
// Shutdown must not be held open by a plugin, but a run that is abandoned here
// keeps its claim until the TTL expires, so the schedule is unavailable for that
// long rather than lost. That is the same trade DownloadManager.ShutdownDrainTimeout
// makes, and for the same reason: a worker that ignores its context must not
// hold a deployment open.
const scheduleDrainTimeout = 10 * time.Second

// maxSchedulesPerTick bounds how much one tick will take on. Anything left is
// still due on the next one, so this is a fairness bound rather than a cap on
// how many schedules a deployment may have.
const maxSchedulesPerTick = 100

// PluginScheduler is the bridge between the clock, the durable schedule rows and
// the plugin manager.
//
// It lives here rather than in either of the packages it joins, because it has
// to reach both: plugin_system may not import application_context, and
// download_queue sits below it too. That is the same shape the download history
// recorder uses — the layer that knows what the work *is* registers it with the
// layer that owns the clock, and neither reaches up.
//
// It owns its own ticker rather than borrowing the download manager's, whose
// interval is a hardcoded five minutes: a scheduler on a five-minute tick cannot
// honour anything finer, and the callbacks that ticker accepts take no context
// and cannot be drained.
type PluginScheduler struct {
	ctx      *MahresourcesContext
	interval time.Duration

	// dispatchWait is how long one dispatch will wait for a job slot before
	// giving the schedule back. A field rather than the constant so a test can
	// observe the give-up without sitting out the real bound.
	dispatchWait time.Duration

	done     chan struct{}
	stopOnce sync.Once
	// runs tracks in-flight dispatches so Stop can wait for them. Nothing else
	// can: an ActionJob lives in this process's memory only, so a run abandoned
	// here is a claim nobody releases until it expires.
	runs sync.WaitGroup

	// Test seams for deferred downloads. Production leaves these nil, selecting
	// the live queue helpers below; tests install fakes so a scheduler tick can
	// prove claim bookkeeping without opening a socket.
	scheduledDownloadSubmit  ScheduledDownloadSubmitFunc
	scheduledDownloadActive  ScheduledDownloadActiveFunc
	pluginScheduleClaimToken func() (string, error)
}

func NewPluginScheduler(ctx *MahresourcesContext, interval time.Duration) *PluginScheduler {
	if interval <= 0 {
		interval = DefaultScheduleTick
	}
	return &PluginScheduler{
		ctx:          ctx,
		interval:     interval,
		dispatchWait: ScheduleDispatchWait,
		done:         make(chan struct{}),
	}
}

// Start begins ticking. It returns immediately.
func (s *PluginScheduler) Start() {
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.Tick(time.Now())
			case <-s.done:
				return
			}
		}
	}()
}

// Stop halts the ticker and waits, briefly, for runs already in flight.
func (s *PluginScheduler) Stop() {
	s.stopOnce.Do(func() { close(s.done) })

	drained := make(chan struct{})
	go func() {
		s.runs.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(scheduleDrainTimeout):
		log.Printf("warning: plugin scheduler still has runs in flight after %s; "+
			"their schedules stay claimed until the claim expires", scheduleDrainTimeout)
	}
}

// Tick claims and dispatches everything due. Exported so a test can drive the
// scheduler without waiting on a wall clock.
//
// Every row is re-checked against the live registry before it is claimed. That
// check is what makes a disabled plugin, a renamed schedule id and a downgraded
// plugin.lua all safe without a cleanup pass: the row is inert because nothing
// declares it, not because something deleted it.
func (s *PluginScheduler) Tick(now time.Time) {
	pm := s.ctx.PluginManager()
	if pm == nil {
		return
	}
	rows, err := s.ctx.DuePluginSchedules(now, maxSchedulesPerTick)
	if err != nil {
		log.Printf("warning: plugin scheduler could not list due schedules: %v", err)
	} else {
		for _, row := range rows {
			if !pm.ScheduleIsRegistered(row.PluginName, row.ScheduleID) {
				continue
			}
			token, err := s.newPluginScheduleClaimToken()
			if err != nil {
				log.Printf("warning: plugin scheduler could not mint a claim token: %v", err)
				break
			}
			claimed, err := s.ctx.ClaimPluginSchedule(row.ID, token, now)
			if err != nil {
				log.Printf("warning: plugin scheduler could not claim %s/%s: %v",
					row.PluginName, row.ScheduleID, err)
				continue
			}
			if !claimed {
				// Another process got there, or it stopped being due. Both ordinary.
				continue
			}

			s.runs.Add(1)
			go func(row models.PluginSchedule, token string) {
				defer s.runs.Done()
				s.dispatch(row, token)
			}(row, token)
		}
	}

	s.fireScheduledDownloads(now)
}

func (s *PluginScheduler) newPluginScheduleClaimToken() (string, error) {
	if s.pluginScheduleClaimToken != nil {
		return s.pluginScheduleClaimToken()
	}
	return newScheduleClaimToken()
}

func (s *PluginScheduler) fireScheduledDownloads(now time.Time) {
	if s == nil || s.ctx == nil {
		return
	}
	fired, err := s.ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:            now,
		Limit:          maxSchedulesPerTick,
		ActiveDownload: s.scheduledDownloadActiveFunc(),
		Submit:         s.scheduledDownloadSubmitFunc(),
	})
	if err != nil {
		log.Printf("warning: plugin scheduler could not fire scheduled downloads: %v", err)
		return
	}
	if fired > 0 {
		log.Printf("plugin scheduler submitted %d scheduled download(s)", fired)
	}
}

func (s *PluginScheduler) scheduledDownloadSubmitFunc() ScheduledDownloadSubmitFunc {
	if s.scheduledDownloadSubmit != nil {
		return s.scheduledDownloadSubmit
	}
	return func(creator *query_models.ResourceFromRemoteCreator, ownerUserID *uint, pluginName string) (string, error) {
		if s.ctx == nil || s.ctx.downloadManager == nil {
			return "", fmt.Errorf("the download queue is not available")
		}
		job, err := s.ctx.downloadManager.SubmitForPlugin(creator, ownerUserID, pluginName)
		if err != nil {
			return "", err
		}
		return job.Snapshot().ID, nil
	}
}

func (s *PluginScheduler) scheduledDownloadActiveFunc() ScheduledDownloadActiveFunc {
	if s.scheduledDownloadActive != nil {
		return s.scheduledDownloadActive
	}
	return func(url string) (string, bool) {
		if s.ctx == nil || s.ctx.downloadManager == nil {
			return "", false
		}
		return download_queue.ActiveDownloadForURL(s.ctx.downloadManager, url)
	}
}

// dispatch runs one claimed schedule to completion.
//
// It blocks for the whole run, which is the point: under "skip" the claim it
// holds is what stops the next tick starting a second copy, and nothing else
// could tell it when to let go.
func (s *PluginScheduler) dispatch(row models.PluginSchedule, token string) {
	pm := s.ctx.PluginManager()
	if pm == nil {
		_ = s.ctx.ReleasePluginScheduleClaim(row.ID, token)
		return
	}

	reg, found := s.scheduleRegistration(row)
	if !found {
		// Disabled between the tick's check and here.
		_ = s.ctx.ReleasePluginScheduleClaim(row.ID, token)
		return
	}

	actor := scheduleActor(row)

	overlapAllows := row.Overlap == models.PluginScheduleOverlapAllow
	if overlapAllows {
		// Advance and let go before running, so the following tick may start
		// another run rather than finding this one still holding the row.
		if err := s.ctx.AdvancePluginScheduleAtDispatch(row.ID, token, time.Now()); err != nil {
			log.Printf("warning: plugin scheduler could not advance %s/%s: %v",
				row.PluginName, row.ScheduleID, err)
		}
	}

	_, ran, runErr := pm.RunSchedule(reg, actor, s.dispatchWait, !overlapAllows)

	if !ran {
		// Nothing executed and nothing was recorded, so the honest outcome is
		// "not this tick": hand the claim back and leave the row due. Under
		// "skip" that is a full job budget or a VM that stayed busy for the
		// dispatch wait. Under "allow" it is only ever a full job budget — the
		// VM is waited for indefinitely there, because the schedule has already
		// been advanced and an interval dropped under "allow" is not deferred to
		// the next tick, it is gone. A full budget still drops one, which is
		// this dispatcher's own pre-existing behaviour rather than the VM wait's.
		if !overlapAllows {
			_ = s.ctx.ReleasePluginScheduleClaim(row.ID, token)
		}
		return
	}

	status, message := scheduleOutcome(runErr)

	now := time.Now()
	if overlapAllows {
		if err := s.ctx.RecordPluginScheduleOutcome(row.ID, status, message, now); err != nil {
			log.Printf("warning: plugin scheduler could not record %s/%s: %v",
				row.PluginName, row.ScheduleID, err)
		}
		return
	}
	if err := s.ctx.CompletePluginScheduleRun(row.ID, token, status, message, now); err != nil {
		log.Printf("warning: plugin scheduler could not complete %s/%s: %v",
			row.PluginName, row.ScheduleID, err)
	}
}

// RunNow executes one schedule immediately, on an operator's say-so, and returns
// as soon as the run has started rather than when it has finished.
//
// Returning early is not a shortcut. RunSchedule blocks for the whole run, which
// may be the full MaxAsyncJobDuration, and an HTTP request must not be held open
// for that. What the caller actually needs to know is whether the run *started*,
// and the claim answers that synchronously: past the claim the run is going to
// happen, and its progress and outcome are already reported the way every other
// plugin job's are, through the action_* events the jobs panel subscribes to.
//
// The run goes on the same WaitGroup a ticked run does, so Stop drains a manual
// run too. Without that, a run started from the page and abandoned at exit would
// hold its claim until the TTL expired with nothing waiting on it — the one
// outcome the drain exists to bound.
func (s *PluginScheduler) RunNow(pluginName, scheduleID string) error {
	pm := s.ctx.PluginManager()
	if pm == nil {
		return fmt.Errorf("%w: plugins are not enabled", ErrScheduleNotDeclared)
	}

	row, err := s.ctx.PluginScheduleByKey(pluginName, scheduleID)
	if err != nil {
		return err
	}
	// Checked before the claim because a row with no live registration must never
	// be claimed at all — that rule is what makes a disabled plugin, a renamed id
	// and a downgraded plugin.lua safe by construction rather than by a cleanup
	// pass, and a manual run is not an exception to it.
	if !pm.ScheduleIsRegistered(pluginName, scheduleID) {
		return fmt.Errorf("%w: %s/%s", ErrScheduleNotDeclared, pluginName, scheduleID)
	}
	// Reported separately from the claim's own refusal so an operator is told
	// which of the two stopped them. The claim still checks it: this read is a
	// label, not the decision.
	if row.CreatedByUserId == nil {
		return fmt.Errorf("%w: %s/%s", ErrScheduleUnowned, pluginName, scheduleID)
	}

	token, err := newScheduleClaimToken()
	if err != nil {
		return err
	}
	claimed, err := s.ctx.ClaimPluginScheduleNow(row.ID, token, time.Now())
	if err != nil {
		return err
	}
	if !claimed {
		// The compare-and-set is the decision, and it refuses for the reasons its
		// own predicates name. Ownership was reported above, so what is left is a
		// live claim: a tick is running this schedule, or another operator got
		// here first.
		return fmt.Errorf("%w: %s/%s", ErrScheduleBusy, pluginName, scheduleID)
	}

	s.runs.Add(1)
	go func(row models.PluginSchedule, token string) {
		defer s.runs.Done()
		s.dispatchManual(row, token)
	}(*row, token)

	return nil
}

// dispatchManual runs one claimed schedule that an operator asked for, and does
// the bookkeeping a manual run needs rather than the bookkeeping a tick needs.
//
// Two things differ from dispatch, and both follow from "an extra run is not a
// re-phasing":
//
//   - next_due_at is never touched, so CompletePluginScheduleRun — which advances
//     it — is exactly the call this must not make, whatever the overlap policy
//     says. AdvancePluginScheduleAtDispatch is out for the same reason.
//   - the claim is therefore held for the whole run under both policies, since
//     there is no advance for "allow" to release early. holdClaim is true, which
//     keeps the VM wait bounded and keeps ScheduleClaimTTL an honest bound on
//     this path as well as on the ticked one.
func (s *PluginScheduler) dispatchManual(row models.PluginSchedule, token string) {
	pm := s.ctx.PluginManager()
	if pm == nil {
		_ = s.ctx.ReleasePluginScheduleClaim(row.ID, token)
		return
	}
	reg, found := s.scheduleRegistration(row)
	if !found {
		// Disabled between RunNow's check and here. Logged for the same reason
		// the !ran branch below is: an operator has already been told this
		// started, and nothing will retry it.
		log.Printf("[plugin] schedule %s/%s was asked to run now but the plugin stopped "+
			"declaring it before the run began", row.PluginName, row.ScheduleID)
		_ = s.ctx.ReleasePluginScheduleClaim(row.ID, token)
		return
	}

	_, ran, runErr := pm.RunSchedule(reg, scheduleActor(row), s.dispatchWait, true)
	if !ran {
		// The handler was never entered, so there is no outcome to record — the
		// same "not this tick" a full job budget or a busy VM gives a ticked run.
		// Recording a failure here would blame the plugin for a run it did not
		// have, which is what errJobDidNotStart exists to prevent.
		//
		// Logged, unlike the ticked path, and the difference is who is waiting. A
		// tick that gives up is retried by the next tick a few seconds later, so
		// there is nothing to tell anyone. Here an operator has already been told
		// the run started, the job card appears and vanishes, the row gains no
		// history, and nothing will retry it — so this line is the only trace
		// that the request went nowhere.
		log.Printf("[plugin] schedule %s/%s was asked to run now but never started: "+
			"the job budget or the plugin's VM stayed busy for the whole %s dispatch wait",
			row.PluginName, row.ScheduleID, s.dispatchWait)
		_ = s.ctx.ReleasePluginScheduleClaim(row.ID, token)
		return
	}

	status, message := scheduleOutcome(runErr)
	// Record first, release second. While the claim is held nothing else can run
	// this schedule, so nothing else can write an outcome — releasing first would
	// open a window in which a tick starts, finishes, records its result, and then
	// has it overwritten by this older one. That is the stale-outcome shape the
	// download history carries an explicit ON CONFLICT guard for; here the claim
	// already excludes it, provided the writes are in this order.
	if err := s.ctx.RecordPluginScheduleOutcome(row.ID, status, message, time.Now()); err != nil {
		log.Printf("warning: plugin scheduler could not record manual run of %s/%s: %v",
			row.PluginName, row.ScheduleID, err)
	}
	if err := s.ctx.ReleasePluginScheduleClaim(row.ID, token); err != nil {
		log.Printf("warning: plugin scheduler could not release %s/%s after a manual run: %v",
			row.PluginName, row.ScheduleID, err)
	}
}

// scheduleRegistration finds the live registration a stored row names, if one
// still answers to it.
func (s *PluginScheduler) scheduleRegistration(row models.PluginSchedule) (plugin_system.ScheduleRegistration, bool) {
	pm := s.ctx.PluginManager()
	if pm == nil {
		return plugin_system.ScheduleRegistration{}, false
	}
	for _, candidate := range pm.DeclaredSchedules(row.PluginName) {
		if candidate.ScheduleID == row.ScheduleID {
			return candidate, true
		}
	}
	return plugin_system.ScheduleRegistration{}, false
}

// scheduleActor is the identity a run executes as: the operator who enabled the
// plugin, and zero when the row has none — which the claim already refuses, on
// both paths.
func scheduleActor(row models.PluginSchedule) uint {
	if row.CreatedByUserId == nil {
		return 0
	}
	return *row.CreatedByUserId
}

// scheduleOutcome turns a run's error into the pair stored on the row.
func scheduleOutcome(runErr error) (status, message string) {
	if runErr == nil {
		return models.PluginScheduleStatusCompleted, ""
	}
	return models.PluginScheduleStatusFailed, truncateScheduleError(runErr.Error())
}

// newScheduleClaimToken mints a token no other process can produce.
//
// Crypto-random rather than a timestamp: two processes claiming in the same
// instant is precisely the case this has to distinguish, and a clock does not.
func newScheduleClaimToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading random bytes for a schedule claim: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// truncateScheduleError keeps a stored error inside the column's width. The
// column is 2000; a Lua traceback is easily longer.
func truncateScheduleError(msg string) string {
	const max = 1900
	if len(msg) <= max {
		return msg
	}
	return msg[:max] + "... (truncated)"
}
