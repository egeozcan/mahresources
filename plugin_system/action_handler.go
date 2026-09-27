package plugin_system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// This file is the part of an async execution that runs its handler: the context
// the Lua call runs under, how the host ends that call early, and who reports the
// outcome once it is over.
//
// The host ends a handler for three reasons, and all three go through one
// mechanism: the call's context is cancelled with a stop cause. gopher-lua checks
// its context between instructions and raises once it has ended, and the calls
// into the host that wait (mah.sleep, mah.http) wait on the same context, so a
// handler stops at the next instruction after a Go call returns. The timeout that
// every async handler has always had works exactly this way, so a handler that
// cannot survive being stopped between two instructions was never safe to run.
//
//   - a person cancels the Job, where the registration declares that its handler
//     may be stopped (cancel = true);
//   - the plugin is disabled or reloaded while the handler runs;
//   - the server is shutting down, after a grace period for the handler to finish
//     by itself.

// asyncHandlerTimeout bounds one async handler's Lua call. It is
// MaxAsyncJobDuration, kept as a variable so a test can observe a timeout
// without waiting five minutes.
var asyncHandlerTimeout = asyncActionTimeout

// errHandlerTimeout is the cause a handler's context ends with when the handler
// ran for longer than asyncHandlerTimeout.
var errHandlerTimeout = errors.New("the handler ran longer than it may")

// stopCause is the cause a handler's context ends with when the host stopped it:
// one of the Stop reasons.
type stopCause struct{ reason string }

func (c stopCause) Error() string { return "the handler was stopped: " + c.reason }

// handlerRun is one entry into an execution's handler: the plugin's VM it holds,
// the context its Lua call runs under, and the stop that ends that call early.
//
// It is also how the VM is given back. Unlock records, before the VM is
// released, that the handler is over and that this execution's own goroutine is
// settling its outcome. Whoever takes the VM next — a shutdown about to close it
// and report every unfinished callback as lost — therefore knows this outcome is
// on its way and must not be reported a second way.
type handlerRun struct {
	pm   *PluginManager
	job  *ActionJob
	vm   *vmMutex
	live func() bool
	// cancellable is whether a person's Cancel may stop this handler once it
	// has entered: the Job records that its handler may be stopped partway.
	cancellable bool

	stopCtx context.Context
	stop    context.CancelCauseFunc
	luaCtx  context.Context
	once    sync.Once
}

// enterHandler records that job's handler is about to run on the VM it holds.
func (pm *PluginManager) enterHandler(job *ActionJob, vm *vmMutex, live func() bool) *handlerRun {
	stopCtx, stop := context.WithCancelCause(context.Background())
	h := &handlerRun{pm: pm, job: job, vm: vm, live: live, stopCtx: stopCtx, stop: stop}
	if ref := job.hostJobRef(); ref != nil {
		h.cancellable = ref.Cancellable
	}
	job.mu.Lock()
	job.handler = h
	job.mu.Unlock()
	return h
}

// cancelledBeforeEntry reports whether a person cancelled the Job while its
// execution was on its way to the handler: then the handler is not entered.
func (job *ActionJob) cancelledBeforeEntry() bool {
	job.mu.RLock()
	defer job.mu.RUnlock()
	return job.pendingStop == StopCancelled
}

// errNotEntered is what a work function returns when its handler was not entered
// after all (enter), with the reason.
type errNotEntered struct{ reason string }

func (e errNotEntered) Error() string { return "the handler was not entered: " + e.reason }

// enter is the handler's entry: the work function calls it with everything its
// Lua call needs in place, immediately before making the call. It answers why the
// handler must not be entered after all, or an empty string when it may, and in
// that case tells the host the handler has started (HostEntryObserver). The
// reasons are the ones checked before the execution reported that it was
// starting, asked again because that report is a write and a person's Cancel, a
// disable or a shutdown can land while it is made.
func (h *handlerRun) enter() string {
	if reason := h.refusal(); reason != "" {
		return reason
	}
	if ref := h.job.hostJobRef(); ref != nil {
		if observer, ok := ref.Sink.(HostEntryObserver); ok {
			observer.Entered()
		}
	}
	return ""
}

// refusal answers why the handler must not be entered, or an empty string when
// it may: a person cancelled the Job, the host stopped the handler, the server is
// shutting down, or the VM it holds is no longer its plugin's.
func (h *handlerRun) refusal() string {
	var stopped stopCause
	switch {
	case h.job.cancelledBeforeEntry():
		return StopCancelled
	case errors.As(context.Cause(h.stopCtx), &stopped):
		return stopped.reason
	}
	return h.pm.cannotEnter(asyncWork{live: h.live})
}

// Context is the context the handler's Lua call runs under: withValues' values
// on a context that the host's stop and asyncHandlerTimeout both end. The stop is
// also what the host's own waiting calls see as the caller going away
// (vmRequestContext), so a handler waiting in mah.http is stopped too rather than
// only once the request returns.
func (h *handlerRun) Context(withValues func(context.Context) context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeoutCause(withValues(vmParentContext(h.stopCtx)), asyncHandlerTimeout, errHandlerTimeout)
	h.luaCtx = ctx
	return ctx, cancel
}

// Unlock gives the plugin's VM back once the handler's Lua call is over. It may be
// called more than once; the first call is the one that counts.
func (h *handlerRun) Unlock() {
	h.once.Do(func() {
		h.job.mu.Lock()
		if h.job.handler == h {
			h.job.handler = nil
		}
		if !h.job.lost {
			h.job.settlesItself = true
		}
		h.job.mu.Unlock()
		h.vm.Unlock()
	})
}

// requestStop ends the handler's Lua call with reason as its cause, unless the
// call is already over.
func (h *handlerRun) requestStop(reason string) {
	h.stop(stopCause{reason: reason})
}

// ended says what ended the Lua call before it returned by itself: the reason
// the host stopped it, or that it ran out of time. Both are empty for a call that
// was not ended early.
//
// It is read from the call's own context, whose cause is fixed by whichever
// ended it first: a stop that lands after the timeout already ended the call (a
// handler still inside a Go call when its time ran out) did not end it, and a
// stop after the call returned finds the context already cancelled by its caller.
func (h *handlerRun) ended() (stopReason string, timedOut bool) {
	if h == nil || h.luaCtx == nil {
		return "", false
	}
	cause := context.Cause(h.luaCtx)
	var stopped stopCause
	switch {
	case errors.As(cause, &stopped):
		return stopped.reason, false
	case errors.Is(cause, errHandlerTimeout):
		return "", true
	}
	return "", false
}

// An execution is in flight from its submission until its goroutine has
// settled it, and exactly one party reports its outcome: the execution itself,
// once its handler is over or it was given up before its handler
// (settlesItself, recorded under the job's lock before the VM is released), or a
// shutdown's lost report for one that has not (lost, under the same lock). Every
// walk that stops, waits for or reports executions goes through trackExecution's
// registry, never through the panel's list, whose entries a person can clear
// while their handler still runs.

// trackExecution records that an async execution is in flight.
func (pm *PluginManager) trackExecution(job *ActionJob) {
	pm.executionsMu.Lock()
	if pm.executions == nil {
		pm.executions = make(map[*ActionJob]struct{})
	}
	pm.executions[job] = struct{}{}
	pm.executionsMu.Unlock()
}

// untrackExecution records that an execution's goroutine is done with it.
func (pm *PluginManager) untrackExecution(job *ActionJob) {
	pm.executionsMu.Lock()
	delete(pm.executions, job)
	pm.executionsMu.Unlock()
}

// inFlight answers the executions in flight now.
func (pm *PluginManager) inFlight() []*ActionJob {
	pm.executionsMu.Lock()
	defer pm.executionsMu.Unlock()
	jobs := make([]*ActionJob, 0, len(pm.executions))
	for job := range pm.executions {
		jobs = append(jobs, job)
	}
	return jobs
}

// stopHandlers ends the Lua call of every running handler match selects, with
// reason as its cause, and answers how many it asked.
func (pm *PluginManager) stopHandlers(reason string, match func(job *ActionJob, h *handlerRun) bool) int {
	var running []*handlerRun
	for _, job := range pm.inFlight() {
		job.mu.RLock()
		h := job.handler
		job.mu.RUnlock()
		if h != nil && match(job, h) {
			running = append(running, h)
		}
	}
	for _, h := range running {
		h.requestStop(reason)
	}
	return len(running)
}

// runningHandlers counts the handlers whose Lua call is in progress.
func (pm *PluginManager) runningHandlers() int {
	running := 0
	for _, job := range pm.inFlight() {
		job.mu.RLock()
		if job.handler != nil {
			running++
		}
		job.mu.RUnlock()
	}
	return running
}

// unsettledOutcomes counts the executions whose handler is over and whose own
// goroutine has not finished reporting the outcome yet.
func (pm *PluginManager) unsettledOutcomes() int {
	pending := 0
	for _, job := range pm.inFlight() {
		job.mu.RLock()
		if job.settlesItself && !job.settleFinished {
			pending++
		}
		job.mu.RUnlock()
	}
	return pending
}

// pollUntil polls done until it answers true or the deadline passes, and reports
// whether it answered true.
func pollUntil(deadline time.Time, done func() bool) bool {
	for {
		if done() {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(handlerDrainPoll)
	}
}

// handlerDrainPoll is how often a teardown looks again at the handlers it is
// waiting for.
const handlerDrainPoll = 25 * time.Millisecond

// StopHostJob stops the handler of the durable Job named, because a person
// cancelled it, and reports whether this process holds an execution of it. A
// running handler ends at its next instruction and reports that it was
// cancelled; one this process holds and has not entered yet is stopped the moment
// it is, so a cancel that lands between the claim and the handler is not lost.
func (pm *PluginManager) StopHostJob(jobID string) bool {
	if jobID == "" {
		return false
	}
	var held []*ActionJob
	for _, job := range pm.inFlight() {
		if ref := job.hostJobRef(); ref != nil && ref.JobID == jobID {
			held = append(held, job)
		}
	}
	for _, job := range held {
		job.mu.Lock()
		h := job.handler
		if h == nil && !job.settlesItself {
			job.pendingStop = StopCancelled
		}
		job.mu.Unlock()
		// A handler that has entered is stopped only where the Job records that
		// it may be: the Job's own declaration (HostJobRef.Cancellable) is what
		// every process advertised Cancel from, so a process whose registration
		// says otherwise cannot authorize stopping this one.
		if h != nil && h.cancellable {
			h.requestStop(StopCancelled)
		}
	}
	return len(held) > 0
}

// stopRevokedHandlers stops the running handlers of one plugin whose VM is no
// longer the plugin's: it was disabled or reloaded under them.
func (pm *PluginManager) stopRevokedHandlers(pluginName string) int {
	return pm.stopHandlers(StopPluginDisabled, func(job *ActionJob, h *handlerRun) bool {
		return job.PluginName == pluginName && !h.live()
	})
}

// handlerFailure is why one execution's handler ended unsuccessfully, from the
// error its Lua call returned.
func (pm *PluginManager) handlerFailure(pluginName string, workErr error, timedOut bool) HostFailure {
	if timedOut {
		return HostFailure{Cause: FailureTimeout,
			Message: fmt.Sprintf("the handler ran for longer than %s and was stopped", readableDuration(asyncHandlerTimeout))}
	}
	if isAbort, reason := parseAbortError(workErr); isAbort {
		return HostFailure{Cause: FailureDeclared, Message: reason}
	}
	return HostFailure{Cause: FailureError, Message: pm.handlerErrorText(pluginName, workErr)}
}

// handlerErrorText is a handler's error as a reader of its Job sees it: the first
// line of a Lua error's own message, without the stack traceback gopher-lua
// appends to it or any further lines the message carried, and with the plugin's
// directory taken off the file it names, so it reads "plugin.lua:20: ..." rather
// than naming where the server keeps its plugins.
func (pm *PluginManager) handlerErrorText(pluginName string, err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	var apiErr *lua.ApiError
	if errors.As(err, &apiErr) && apiErr.Object != nil {
		text = apiErr.Object.String()
	}
	if line, _, found := strings.Cut(text, "\n"); found {
		text = strings.TrimSuffix(line, "\r")
	}
	if plugin := pm.GetDiscoveredPlugin(pluginName); plugin != nil && plugin.Dir != "" {
		text = strings.ReplaceAll(text, plugin.Dir+string(os.PathSeparator), "")
	}
	return text
}

// readableDuration writes a whole number of minutes as minutes, and anything
// else as Go does.
func readableDuration(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 {
		minutes := int(d / time.Minute)
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	return d.String()
}

// SetShutdownBoundsForTest shortens Close's waits for plugin handlers and
// returns a function that restores them.
//
// Exported for application_context's tests, which observe what a shutdown
// records for the plugin work it interrupts and cannot reach this package's
// unexported bounds.
func SetShutdownBoundsForTest(grace, stopWait, settleWait time.Duration) func() {
	oldGrace, oldStop, oldSettle := shutdownHandlerGrace, shutdownHandlerStopWait, shutdownSettleWait
	shutdownHandlerGrace, shutdownHandlerStopWait, shutdownSettleWait = grace, stopWait, settleWait
	return func() {
		shutdownHandlerGrace, shutdownHandlerStopWait, shutdownSettleWait = oldGrace, oldStop, oldSettle
	}
}
