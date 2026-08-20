package plugin_system

import (
	"context"
	"fmt"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// MinScheduleInterval is the shortest interval a plugin may declare.
//
// It is a floor on what the host will honour, not on what the ticker can see: a
// deployment whose tick is slower than this simply runs schedules at the tick's
// resolution. The floor exists so a plugin cannot ask for a poll every second
// and quietly get one every thirty.
const MinScheduleInterval = 30 * time.Second

// MaxScheduleInterval is the longest, and it is a guard against arithmetic
// rather than against ambition: a duration far past the epoch produces a due
// time the database cannot store.
const MaxScheduleInterval = 365 * 24 * time.Hour

// Overlap policies. Mirrored in models.PluginSchedule, which cannot import this
// package.
const (
	ScheduleOverlapSkip  = "skip"
	ScheduleOverlapAllow = "allow"
)

// ScheduleRegistration is one mah.schedule call: a plugin's own name for a piece
// of recurring work, plus the handler to run.
//
// The handler is the half that cannot be persisted. A *lua.LFunction belongs to
// the *lua.LState that created it and is meaningless once that state is closed,
// so the durable record of a schedule is a database row and this is what the row
// is matched against by name. A row with no registration here is a schedule
// whose plugin is disabled, whose id was renamed, or whose plugin.lua no longer
// declares it — and it is never run.
type ScheduleRegistration struct {
	PluginName string        `json:"pluginName"`
	ScheduleID string        `json:"scheduleId"`
	Every      time.Duration `json:"-"`
	Overlap    string        `json:"overlap"`

	// EverySeconds is the wire form of Every, for the manage page.
	EverySeconds int64 `json:"everySeconds"`

	fn    *lua.LFunction
	state *lua.LState
}

// validScheduleID is the same grammar a page path uses, minus the slashes: the
// id is half of a database unique key and is printed on the manage page, so it
// is kept to something that cannot be confused with anything else.
func validScheduleID(id string) bool {
	if id == "" || len(id) > 100 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

// parseScheduleRegistration reads the table mah.schedule was given.
//
// Every wrong shape is rejected by name rather than defaulted. That is the rule
// the low-hanging-fruit round arrived at the hard way: a validator that guards
// the good shape and lets everything else fall through to a zero value makes a
// typo mean the opposite of what its author wrote — here, a mistyped `every`
// would read as zero and become "as often as the host will allow".
func parseScheduleRegistration(L *lua.LState, tbl *lua.LTable, pluginName string) (ScheduleRegistration, error) {
	var reg ScheduleRegistration
	reg.PluginName = pluginName

	idVal := tbl.RawGetString("id")
	id, ok := idVal.(lua.LString)
	if !ok {
		return reg, fmt.Errorf("id must be a string naming this schedule, got %s", idVal.Type())
	}
	if !validScheduleID(string(id)) {
		return reg, fmt.Errorf("id %q must be 1-100 characters of letters, digits, underscore or hyphen", string(id))
	}
	reg.ScheduleID = string(id)

	everyVal := tbl.RawGetString("every")
	everyStr, ok := everyVal.(lua.LString)
	if !ok {
		return reg, fmt.Errorf("every must be a duration string such as \"15m\", got %s", everyVal.Type())
	}
	every, err := time.ParseDuration(string(everyStr))
	if err != nil {
		return reg, fmt.Errorf("every %q is not a duration: %v", string(everyStr), err)
	}
	if every < MinScheduleInterval {
		return reg, fmt.Errorf("every %q is below the %s minimum; a schedule that cannot be honoured "+
			"is refused rather than silently slowed", string(everyStr), MinScheduleInterval)
	}
	if every > MaxScheduleInterval {
		return reg, fmt.Errorf("every %q is above the %s maximum", string(everyStr), MaxScheduleInterval)
	}
	reg.Every = every
	reg.EverySeconds = int64(every / time.Second)

	reg.Overlap = ScheduleOverlapSkip
	switch overlapVal := tbl.RawGetString("overlap"); v := overlapVal.(type) {
	case *lua.LNilType:
	case lua.LString:
		switch string(v) {
		case ScheduleOverlapSkip, ScheduleOverlapAllow:
			reg.Overlap = string(v)
		default:
			return reg, fmt.Errorf("overlap %q must be %q or %q", string(v), ScheduleOverlapSkip, ScheduleOverlapAllow)
		}
	default:
		return reg, fmt.Errorf("overlap must be a string, got %s", overlapVal.Type())
	}

	handlerVal := tbl.RawGetString("handler")
	handler, ok := handlerVal.(*lua.LFunction)
	if !ok {
		return reg, fmt.Errorf("handler must be a function, got %s", handlerVal.Type())
	}
	reg.fn = handler

	return reg, nil
}

// DeclaredSchedules returns what a plugin currently has registered.
//
// The bridge in application_context reads this after a load to sync rows. It is
// a copy: the caller must not be handed a slice the teardown sweep can mutate
// underneath it.
func (pm *PluginManager) DeclaredSchedules(pluginName string) []ScheduleRegistration {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]ScheduleRegistration, len(pm.schedules[pluginName]))
	copy(out, pm.schedules[pluginName])
	return out
}

// AllDeclaredSchedules returns every registered schedule, for the sweep that
// decides which rows still have a plugin behind them.
func (pm *PluginManager) AllDeclaredSchedules() []ScheduleRegistration {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var out []ScheduleRegistration
	for _, regs := range pm.schedules {
		out = append(out, regs...)
	}
	return out
}

// ScheduleIsRegistered reports whether a stored row still has a live handler.
//
// This is the join between the durable half and the in-memory half, and it is
// the reason none of the awkward lifecycle cases need a cleanup pass: a disabled
// plugin, a renamed schedule id and a downgraded plugin.lua all answer false
// here, and a row that answers false is never run.
func (pm *PluginManager) ScheduleIsRegistered(pluginName, scheduleID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, reg := range pm.schedules[pluginName] {
		if reg.ScheduleID == scheduleID {
			return true
		}
	}
	return false
}

// errScheduleVMBusy is internal: it wraps errJobDidNotStart, so the job runner
// returns ran=false without recording a failure, and RunSchedule's existing
// !ran path removes the job entry. It never reaches a caller, because a schedule
// whose VM stayed busy did not run at all, and the dispatcher must treat that as
// "not this tick" rather than as a failed run to record.
//
// It can only be returned when the caller holds a claim; see acquireScheduleVM.
var errScheduleVMBusy = fmt.Errorf(
	"the plugin's VM stayed busy for the whole dispatch budget: %w", errJobDidNotStart)

// RunSchedule executes one due schedule and blocks until it has finished.
//
// Blocking is the point. The caller is holding a database claim on the schedule
// row for the whole run — under the "skip" policy that is what stops the next
// tick starting a second copy — so it needs to know when the run is over, and
// nothing else can tell it: an ActionJob lives in this process's memory only.
//
// The wait bounds everything before the handler is entered — the job slot and,
// when holdClaim is set, the plugin's VM lock, sharing one deadline — and
// running out of it is reported as ran=false with no error. That is not a
// failure of the schedule; it is a tick that could not get started, and the
// caller's correct response is to release the claim and leave the row due. The
// bound is not a nicety: the caller's claim expires after ScheduleClaimTTL,
// which is derived as this wait plus the run, so an unbounded wait would let the
// claim lapse mid-dispatch and the next tick would fire the same schedule again.
//
// holdClaim is that condition rather than an option. A caller that has already
// released the row must not have its VM wait bounded — see acquireScheduleVM.
// acquireScheduleVM takes the plugin's VM lock for a run that is about to start.
//
// Whether that wait is bounded is decided by the single thing a bound protects:
// a database claim held across it. Under "skip" the dispatcher holds the row for
// the whole run, and ScheduleClaimTTL is derived as the dispatch wait plus the
// run — so an unbounded wait here is a term that formula does not have, the
// claim lapses mid-dispatch, and the next tick of this same process fires a
// second copy of a schedule that has not begun its first.
//
// Under "allow" the dispatcher has already advanced next_due_at and released the
// claim before this is reached, so there is nothing left to lose — and bounding
// the wait there costs the policy its meaning. "allow" buys queueing: a run that
// finds the VM held by the previous one is supposed to wait for it, and CLAUDE.md
// says so ("an overrunning run does not cause the next to be skipped"). Giving up
// instead does not defer that interval, it drops it, because the row has moved on
// and no later tick will find it due.
//
// The busy report is therefore only ever true for a bounded wait. An unbounded
// one that comes back empty-handed means the plugin is gone, which is the same
// answer LockVM has always given and every caller already handles.
func (pm *PluginManager) acquireScheduleVM(state *lua.LState, holdClaim bool, deadline time.Time) (*vmMutex, bool) {
	if holdClaim {
		return pm.TryLockVMWithin(context.Background(), state, time.Until(deadline))
	}
	mu, _ := pm.LockVMWithContext(context.Background(), state)
	return mu, false
}

func (pm *PluginManager) RunSchedule(reg ScheduleRegistration, actorUserID uint, wait time.Duration, holdClaim bool) (jobID string, ran bool, err error) {
	pm.mu.RLock()
	live := pm.schedules[reg.PluginName]
	var current *ScheduleRegistration
	for i := range live {
		if live[i].ScheduleID == reg.ScheduleID {
			current = &live[i]
			break
		}
	}
	pm.mu.RUnlock()
	if current == nil {
		return "", false, fmt.Errorf("plugin %q no longer declares schedule %q", reg.PluginName, reg.ScheduleID)
	}
	if pm.closed.Load() {
		return "", false, fmt.Errorf("plugin manager is shutting down")
	}

	state, fn := current.state, current.fn

	jobID = generateActionJobID()
	var owner *uint
	if actorUserID != 0 {
		id := actorUserID
		owner = &id
	}
	job := &ActionJob{
		ID:         jobID,
		Source:     "plugin",
		PluginName: reg.PluginName,
		ActionID:   "schedule:" + reg.ScheduleID,
		Label:      fmt.Sprintf("%s: %s", reg.PluginName, reg.ScheduleID),
		EntityType: "custom",
		Status:     "pending",
		Progress:   0,
		Message:    "Waiting to start...",
		CreatedAt:  time.Now(),
		// A scheduled run has no interactive submitter, so the owner comes from
		// the row rather than from an ambient invocation the way start_job's
		// does. Without one, jobVisibleToPrincipal hides the job from every
		// non-admin — including the operator whose schedule it is.
		ownerUserID: owner,
	}

	pm.actionJobsMu.Lock()
	pm.actionJobs[jobID] = job
	pm.actionJobsMu.Unlock()
	pm.notifyActionJobSubscribers("added", job)

	wg := pm.actionWaitGroup(reg.PluginName)
	wg.Add(1)
	defer wg.Done()

	// One budget covers everything before the handler runs, rather than one wait
	// per queue. ScheduleClaimTTL is derived as this wait plus the run, so any
	// unbounded wait in between is a term the formula does not have — and LockVM
	// is exactly that ("Background never ends, so this is the same unbounded wait
	// it always was"). Whatever else holds this plugin's VM is what would be
	// waited on: a hook inside mah.http, another async job, a remote fetch. Past
	// the TTL the row reads as unclaimed again and the next tick starts a second
	// run of a schedule that has not begun its first.
	deadline := time.Now().Add(wait)

	ran = pm.executeAsyncJobWithin(job, fmt.Sprintf("schedule %q/%q", reg.PluginName, reg.ScheduleID), wait, func() error {
		mu, busy := pm.acquireScheduleVM(state, holdClaim, deadline)
		if mu == nil {
			if !busy {
				return fmt.Errorf("plugin %q is no longer available", reg.PluginName)
			}
			// Busy, and the budget is spent. The handler was never called, so
			// this is "not this tick" rather than a failed run — the same answer
			// a full job budget gives, and the dispatcher already knows how to
			// hand the claim back for it. errJobDidNotStart is what keeps the
			// job runner from recording it as a failure on the way out.
			return errScheduleVMBusy
		}
		defer mu.Unlock()

		timeoutCtx, cancel := context.WithTimeout(
			withInvocation(context.Background(), NewInvocation(actorUserID)), asyncActionTimeout)
		state.SetContext(timeoutCtx)
		defer func() {
			state.RemoveContext()
			cancel()
		}()

		return state.CallByParam(lua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		}, lua.LString(jobID))
	})

	if !ran {
		// Nothing was executed, so the job entry would sit in the panel claiming
		// to be pending or, for a VM that stayed busy, blaming the plugin for a
		// run it never started.
		pm.actionJobsMu.Lock()
		delete(pm.actionJobs, jobID)
		pm.actionJobsMu.Unlock()
		pm.notifyActionJobSubscribers("removed", job)
		return "", false, nil
	}

	job.mu.RLock()
	failed := job.Status == "failed"
	message := job.Message
	job.mu.RUnlock()
	if failed {
		return jobID, true, fmt.Errorf("%s", message)
	}
	return jobID, true, nil
}
