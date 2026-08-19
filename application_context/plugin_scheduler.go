package application_context

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"mahresources/models"
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
		return
	}
	for _, row := range rows {
		if !pm.ScheduleIsRegistered(row.PluginName, row.ScheduleID) {
			continue
		}
		token, err := newScheduleClaimToken()
		if err != nil {
			log.Printf("warning: plugin scheduler could not mint a claim token: %v", err)
			return
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

	var reg plugin_system.ScheduleRegistration
	found := false
	for _, candidate := range pm.DeclaredSchedules(row.PluginName) {
		if candidate.ScheduleID == row.ScheduleID {
			reg, found = candidate, true
			break
		}
	}
	if !found {
		// Disabled between the tick's check and here.
		_ = s.ctx.ReleasePluginScheduleClaim(row.ID, token)
		return
	}

	var actor uint
	if row.CreatedByUserId != nil {
		actor = *row.CreatedByUserId
	}

	overlapAllows := row.Overlap == models.PluginScheduleOverlapAllow
	if overlapAllows {
		// Advance and let go before running, so the following tick may start
		// another run rather than finding this one still holding the row.
		if err := s.ctx.AdvancePluginScheduleAtDispatch(row.ID, token, time.Now()); err != nil {
			log.Printf("warning: plugin scheduler could not advance %s/%s: %v",
				row.PluginName, row.ScheduleID, err)
		}
	}

	_, ran, runErr := pm.RunSchedule(reg, actor, s.dispatchWait)

	if !ran {
		// The job budget was full. Nothing executed and nothing was recorded, so
		// the honest outcome is "not this tick": hand the claim back and leave the
		// row due. Under "allow" the schedule has already been advanced, so that
		// interval is simply missed — a tick that could not get a slot, which the
		// next one will.
		if !overlapAllows {
			_ = s.ctx.ReleasePluginScheduleClaim(row.ID, token)
		}
		return
	}

	status := models.PluginScheduleStatusCompleted
	message := ""
	if runErr != nil {
		status = models.PluginScheduleStatusFailed
		message = truncateScheduleError(runErr.Error())
	}

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
