package application_context

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"mahresources/jobs"
	"mahresources/plugin_system"
)

// This file is how a plugin-action Job waits for its plugin.
//
// A plugin runs one piece of background work at a time, because its VM runs one
// Lua call at a time. So the Job is accepted `queued` and handed to its plugin's
// lane in plugin_system; it is claimed — moved to running, fenced, and counted
// against the deployment's `max-job-concurrency` — only when it reaches the head
// of that lane and holds a job slot. Until then it holds nothing another plugin or
// another Kind is waiting for, and it reads as what it is: queued.
//
// The dispatch loop does not claim this Kind (RuntimeClaimEnabled). A loop claims
// the oldest waiting Job whatever its plugin and would then wait for that plugin
// with the claim held, which turns one plugin's backlog into occupied slots of the
// whole deployment. The loop's cadence is still used, to hand this process's lanes
// the waiting Jobs nobody here holds (see AdoptWaiting).

// pluginActionAdmission is the durable half of one queued plugin execution.
//
// It is both the admission the head of the lane asks and the sink the execution
// reports through: before the claim nothing is reported (nothing has started),
// and after it every report goes to the claimed execution's own sink.
type pluginActionAdmission struct {
	ctx     *MahresourcesContext
	subtype string
	// accept, when set, is the acceptance of the Job this execution runs, made
	// at its first admission together with the claim, rather than before it
	// waited (see acceptAtAdmission).
	accept *jobs.Acceptance
	// decided, when set, is told once whether the execution started: true as
	// its handler is entered, false when it gave up without entering it, with
	// the reason its checks refused it before its Job was accepted, if they did.
	decided func(started bool, refusal string)
	// input is what the execution reports with when its Job stores none: a
	// closure's input is not replayable, so the claim opens nothing and the
	// accepting call's own description stands in for it.
	input *pluginActionJobInput
	// refusal re-checks, within bounded, that the execution may still run as the
	// principal it acts as. It answers the reason it may not, or an empty string
	// when it may; an error means it could not find out, which is not a refusal. A
	// nil refusal re-checks nothing.
	refusal pluginActionRefusalCheck

	mu sync.Mutex
	// jobID is the durable Job, or empty until an admission that accepts it at
	// admission has done so.
	jobID     string
	execution jobs.Execution
	sink      *pluginActionSink
	// returning is open while a claim this admission gave back is still being
	// handed back: the Job is running under the returned token until it closes.
	returning chan struct{}
	// ending is set once this admission has refused or failed the Job under its
	// own claim. The write that records it runs, and is retried, on its own.
	ending bool
	// refused is the reason the checks of an execution accepted at its admission
	// refused it before its Job existed; nothing was accepted then.
	refused string
	// givenBack counts the attempts in a row whose re-checks could not answer:
	// the claims given back for that, and the attempts after one that found the
	// checks still unanswered before claiming. Its deferral and its bound grow
	// with it (Deferral, attemptBound).
	givenBack int
}

// pluginActionRefusalCheck is an admission's re-check of the acting principal.
type pluginActionRefusalCheck func(bounded context.Context, execution jobs.Execution, input *pluginActionJobInput) (reason string, err error)

func (ctx *MahresourcesContext) newPluginActionAdmission(jobID string, input *pluginActionJobInput, refusal pluginActionRefusalCheck) *pluginActionAdmission {
	return &pluginActionAdmission{ctx: ctx, jobID: jobID, subtype: input.Subtype, input: input, refusal: refusal}
}

// acceptAtAdmission makes this admission accept its Job when it is first
// admitted, accepted and claimed in one transaction (jobs.Service.AcceptClaimed),
// and tells decided whether the execution started. It is for a scheduled
// occurrence: a tick whose plugin, job slot or deployment budget stays busy for
// the whole dispatch wait is "not this tick" and leaves the row due, and a Job
// that existed before that wait, or before its claim had a slot of the budget,
// would be a cancelled Job recorded for every such tick. The same holds for a
// tick its checks refuse (checksBeforeAcceptance).
func (a *pluginActionAdmission) acceptAtAdmission(acceptance jobs.Acceptance, decided func(started bool, refusal string)) {
	a.accept = &acceptance
	a.decided = decided
}

// decide tells the admission's caller, once, whether the execution started.
func (a *pluginActionAdmission) decide(started bool) {
	if a.decided != nil {
		a.decided(started, a.refusedBeforeAcceptance())
	}
}

// refusedBeforeAcceptance is the reason the checks refused this execution before
// its Job was accepted, or empty.
func (a *pluginActionAdmission) refusedBeforeAcceptance() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.refused
}

// waitsForAnyProcess reports whether this execution's Job is a scheduled run's
// Retry successor: it existed before this execution and holds no schedule row's
// claim, so it waits in the queue for whichever process can run it, as a queued
// action does, and is never withdrawn for not starting here. A fresh occurrence is
// accepted at its admission and is the tick of the row its scheduler claimed.
func (a *pluginActionAdmission) waitsForAnyProcess() bool {
	return a.subtype == pluginActionSubtypeScheduled && a.accept == nil
}

// JobID implements plugin_system.HostJobNamer: the durable Job, once there is one.
func (a *pluginActionAdmission) JobID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.jobID
}

func (a *pluginActionAdmission) setJobID(jobID string) {
	a.mu.Lock()
	a.jobID = jobID
	a.mu.Unlock()
}

// hostJobRef is the reference plugin_system is handed for this execution.
func (a *pluginActionAdmission) hostJobRef(handle, parentJobID string) *plugin_system.HostJobRef {
	return &plugin_system.HostJobRef{
		JobID: a.JobID(), Handle: handle, ParentJobID: parentJobID,
		Sink: a, Admission: a, Cancellable: a.input != nil && a.input.Cancellable,
	}
}

// acceptClaimed accepts this admission's Job and claims it in one transaction,
// within bounded, and keeps the claim alive as claimPluginActionJobNamed does. A
// full deployment budget rolls the whole transaction back: no Job is left behind
// for a tick that could not start.
func (a *pluginActionAdmission) acceptClaimed(bounded context.Context) (jobs.Execution, func(), error) {
	deps := a.ctx.jobDeps()
	if deps.DB != nil {
		deps.DB = deps.DB.WithContext(bounded)
	}
	execution, _, err := a.ctx.JobService().AcceptClaimed(context.Background(), deps, *a.accept, jobs.ClaimRequest{
		Kind:        JobKindPluginAction,
		KindVersion: jobPluginActionKindVersion,
		Claimant:    plugin_system.CurrentRuntimeIdentity().String(),
		Capacity:    a.ctx.hostClaimCapacityBudget(),
	})
	if execution.ExecutionToken == "" {
		if err == nil {
			err = errors.New("the acceptance answered no claim")
		}
		return jobs.Execution{}, func() {}, err
	}
	a.setJobID(execution.JobID)
	return execution, a.ctx.startPluginActionHeartbeat(execution), err
}

// Admit claims the Job for this execution.
//
// A full deployment budget is "later" and writes nothing. A Job that is no longer
// waiting — cancelled, blocked, or claimed by another runtime — is withdrawn from
// this process's lane without a word, because whoever moved it owns what happens
// to it now.
//
// It is asked with the plugin's VM held, so everything it does is bounded by one
// deadline: the caller's, or one attempt's bound, whichever comes first. That
// covers the claim, the read of the claimed input and the re-checks of the
// acting principal. A re-check that ran out, or failed, found nothing out: it is
// never recorded as a refusal, and the work never runs on it. The claim is given
// back and the admission is deferred, which takes the execution out of its lane
// for a while that doubles with each claim given back in a row (Deferral).
// Recording what ends a claimed Job, or gives its claim back, happens after the
// VM is released.
//
// A panic here is contained: before the claim the execution asks again later,
// and after it the Job is ended, since the claim would otherwise leave it running
// with nothing to run it.
func (a *pluginActionAdmission) Admit(deadline time.Time) (result plugin_system.AdmitResult) {
	if _, admitted := a.admitted(); admitted {
		return plugin_system.Admitted
	}
	if a.stillReturning() {
		// The Job is running under the token given back until the release lands,
		// and a claim asked for meanwhile would read it as another runtime's. The
		// execution stays out of its lane until then, rather than holding the lane
		// from the plugin's other work while it waits.
		return plugin_system.AdmitDeferred
	}
	if a.endedByItself() {
		return plugin_system.AdmitWithdrawn
	}
	if attempt := time.Now().Add(a.attemptBound()); deadline.IsZero() || attempt.Before(deadline) {
		deadline = attempt
	}
	bounded, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	jobID := a.JobID()
	if jobID == "" && a.accept == nil {
		return plugin_system.AdmitWithdrawn
	}
	if jobID == "" {
		if result, proceed := a.checksBeforeAcceptance(bounded); !proceed {
			return result
		}
	} else if !a.checksAnswerBeforeClaim(bounded, jobID) {
		return plugin_system.AdmitDeferred
	}

	var claimed *jobs.Execution
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		log.Printf("warning: admitting plugin job %s panicked: %v", a.JobID(), r)
		result = plugin_system.AdmitLater
		if claimed != nil {
			execution := *claimed
			a.endClaimed(execution, func() error {
				return a.ctx.failPluginActionJob(execution, "plugin-action-unavailable",
					"the plugin action could not be started")
			})
			result = plugin_system.AdmitWithdrawn
		}
	}()
	// The claim is known the moment it commits, so a panic in what follows it
	// is settled by the recovery above rather than leaving the Job running.
	var (
		execution     jobs.Execution
		stopHeartbeat func()
		err           error
	)
	acceptedNow := jobID == ""
	if acceptedNow {
		execution, stopHeartbeat, err = a.acceptClaimed(bounded)
		jobID = a.JobID()
	} else {
		execution, stopHeartbeat, err = a.ctx.claimPluginActionJobNamed(bounded, jobID, func(ref jobs.ExecutionRef) {
			claimed = &jobs.Execution{JobID: ref.JobID, ExecutionToken: ref.ExecutionToken}
		})
	}
	var unrunnable *jobs.UnrunnableClaimError
	switch {
	case err == nil:
	case errors.As(err, &unrunnable):
		// The Job cannot run, and the control plane could not record why within
		// the bound. It is recorded from here, under the claim.
		claimed = &execution
		reason := unrunnable.Reason
		a.endClaimed(execution, func() error { return a.ctx.blockPluginActionJob(execution, reason) })
		return plugin_system.AdmitWithdrawn
	case errors.Is(err, jobs.ErrExecutionNotLoaded):
		// The sealed input could not be read in time. The admission holds the
		// same input in memory, as it was sealed or opened, and runs with that.
		log.Printf("warning: could not read the input of plugin job %s after claiming it; running with the one it was queued with: %v", jobID, err)
	case errors.Is(err, jobs.ErrCapacityExhausted):
		return plugin_system.AdmitLater
	case errors.Is(err, jobs.ErrJobNotWaiting):
		return plugin_system.AdmitWithdrawn
	case errors.Is(err, context.DeadlineExceeded):
		return plugin_system.AdmitLater
	default:
		log.Printf("warning: could not claim plugin job %s: %v", jobID, err)
		return plugin_system.AdmitLater
	}
	claimed = &execution
	if !acceptedNow && a.cancelWon(bounded, execution.JobID) {
		sink := newPluginActionSink(a.ctx, execution, a.input)
		a.endClaimed(execution, sink.endCancelled)
		return plugin_system.AdmitWithdrawn
	}
	input, err := a.inputOf(execution)
	if err != nil {
		a.endClaimed(execution, func() error {
			return a.ctx.failPluginActionJob(execution, "plugin-action-unavailable",
				"the plugin action could not be started")
		})
		return plugin_system.AdmitWithdrawn
	}
	if a.refusal != nil {
		reason, err := a.refusal(bounded, execution, input)
		if err != nil {
			log.Printf("warning: could not re-check plugin job %s before it runs; giving its claim back: %v", jobID, err)
			a.giveBack(execution, input, stopHeartbeat)
			return plugin_system.AdmitDeferred
		}
		if reason != "" {
			a.endClaimed(execution, func() error { return a.ctx.blockPluginActionJob(execution, reason) })
			return plugin_system.AdmitWithdrawn
		}
	}
	sink := newPluginActionSink(a.ctx, execution, input)
	sink.waitsForAnyProcess = a.waitsForAnyProcess()
	a.mu.Lock()
	a.execution = execution
	a.sink = sink
	a.mu.Unlock()
	return plugin_system.Admitted
}

// checksAnswerBeforeClaim reports whether this admission may ask for its claim.
//
// Once a claim has been given back because the re-checks under it could not
// answer, the next attempt asks them first without a claim, and defers again
// while they still cannot answer. A claim taken for checks that still cannot
// answer is given straight back, and every such round trip adds a start and a
// return to the queue to the Job's timeline — host lifecycle events, which no
// timeline ceiling bounds — so a failure that lasted a day would write thousands.
// This way a streak of failures costs one round trip. The checks under the claim
// still decide: an answer here only lets the admission ask for the claim.
func (a *pluginActionAdmission) checksAnswerBeforeClaim(bounded context.Context, jobID string) bool {
	a.mu.Lock()
	givenBack := a.givenBack
	a.mu.Unlock()
	if a.refusal == nil || givenBack == 0 {
		return true
	}
	deps := a.ctx.jobDeps()
	if deps.DB != nil {
		deps.DB = deps.DB.WithContext(bounded)
	}
	snap, err := a.ctx.JobService().Get(deps, jobs.Access{Administrator: true}, jobID)
	if err == nil && snap.ControlIntent == jobs.ControlIntentCancel {
		// A cancellation has won the Job: it will not run, so nothing needs
		// checking before the claim that ends it.
		return true
	}
	if err == nil {
		access, known := waitingExecutionAccess(snap)
		if !known {
			// The account the Job acts as is gone; the claim blocks it.
			return true
		}
		if _, err = a.refusal(bounded, jobs.Execution{JobID: jobID, Access: access}, a.input); err == nil {
			return true
		}
	}
	log.Printf("warning: the checks for plugin job %s still cannot answer; waiting before claiming it: %v", jobID, err)
	a.mu.Lock()
	a.givenBack++
	a.mu.Unlock()
	return false
}

// checksBeforeAcceptance asks the checks of an execution accepted at its
// admission before its Job exists, and reports whether the admission may go on
// to accept it. A refusal accepts nothing and records why (refusedBeforeAcceptance);
// checks that cannot answer defer, as they would under a claim, with no Job to
// give back. A schedule whose operator may no longer run its plugin therefore
// records no Job at each tick. The checks under the claim still decide: this
// answer only lets the admission ask for the Job.
func (a *pluginActionAdmission) checksBeforeAcceptance(bounded context.Context) (plugin_system.AdmitResult, bool) {
	if a.refusal == nil {
		return plugin_system.Admitted, true
	}
	reason, err := a.refusal(bounded, jobs.Execution{Access: acceptanceAccess(*a.accept)}, a.input)
	switch {
	case err != nil:
		log.Printf("warning: the checks for a %s run of %s cannot answer; waiting before accepting it: %v",
			a.subtype, a.input.Plugin, err)
		a.mu.Lock()
		a.givenBack++
		a.mu.Unlock()
		return plugin_system.AdmitDeferred, false
	case reason != "":
		a.mu.Lock()
		a.refused = reason
		a.mu.Unlock()
		return plugin_system.AdmitWithdrawn, false
	}
	return plugin_system.Admitted, true
}

// acceptanceAccess is the principal a Job accepted with acceptance runs as, read
// the way a claim reads it: its actor, otherwise its owner, otherwise nobody.
func acceptanceAccess(acceptance jobs.Acceptance) jobs.Access {
	switch {
	case acceptance.ActorUserID != nil:
		return jobs.Access{UserID: *acceptance.ActorUserID}
	case acceptance.OwnerUserID != nil:
		return jobs.Access{UserID: *acceptance.OwnerUserID}
	default:
		return jobs.Access{}
	}
}

// cancelWon reports whether a person's cancellation has won the waiting Job this
// admission just claimed. Such a Job does not run and needs no re-check: it is
// ended cancelled under the claim. A cancellation reaches a waiting Job this way
// when it was recorded while an earlier claim of it was being returned to the
// queue; one recorded after this read reaches the execution through its
// heartbeat. A read that fails answers false, and the admission goes on as it
// would have.
func (a *pluginActionAdmission) cancelWon(bounded context.Context, jobID string) bool {
	deps := a.ctx.jobDeps()
	if deps.DB != nil {
		deps.DB = deps.DB.WithContext(bounded)
	}
	snap, err := a.ctx.JobService().Get(deps, jobs.Access{Administrator: true}, jobID)
	return err == nil && snap.ControlIntent == jobs.ControlIntentCancel
}

// waitingExecutionAccess is the principal a waiting Job would run as, read the way
// a claim reads it (jobs.executionAccess): nobody for host work, its owner or its
// actor otherwise. It reports false when that account is gone.
func waitingExecutionAccess(snap jobs.Snapshot) (jobs.Access, bool) {
	switch snap.ExecutionPrincipal {
	case jobs.PrincipalHost:
		return jobs.Access{}, true
	case jobs.PrincipalOwner:
		if snap.OwnerUserID == nil {
			return jobs.Access{}, false
		}
		return jobs.Access{UserID: *snap.OwnerUserID}, true
	default:
		if snap.ActorUserID == nil {
			return jobs.Access{}, false
		}
		return jobs.Access{UserID: *snap.ActorUserID}, true
	}
}

// attemptBound is how long this admission's database work may hold the plugin's
// VM: pluginActionAdmissionAttempt, doubled for each claim given back in a row, up
// to pluginActionAdmissionAttemptCap. A re-check that is slow but answers, such as
// a scope subtree large enough to take longer than the first bound, is given more
// time at each attempt until it fits, while no attempt holds the VM for longer
// than the cap.
func (a *pluginActionAdmission) attemptBound() time.Duration {
	a.mu.Lock()
	givenBack := a.givenBack
	a.mu.Unlock()
	bound := pluginActionAdmissionAttempt
	for ; givenBack > 0 && bound < pluginActionAdmissionAttemptCap; givenBack-- {
		bound *= 2
	}
	if bound > pluginActionAdmissionAttemptCap {
		bound = pluginActionAdmissionAttemptCap
	}
	return bound
}

// Deferral implements plugin_system.HostDeferral: how long the execution stays
// out of its lane after an attempt whose re-checks could not answer. It doubles
// with each such attempt in a row, from one second to pluginActionGiveBackCap.
// With checksAnswerBeforeClaim that is the whole cost of a re-check that keeps
// failing: one claim given back — a start and a return to the queue on the Job's
// timeline — for the streak, and then one attempt per interval that holds the
// plugin's VM for its bound and writes nothing.
func (a *pluginActionAdmission) Deferral() time.Duration {
	a.mu.Lock()
	givenBack := a.givenBack
	a.mu.Unlock()
	if givenBack < 1 {
		givenBack = 1
	}
	if givenBack > 6 {
		return pluginActionGiveBackCap
	}
	if doubled := time.Second << (givenBack - 1); doubled < pluginActionGiveBackCap {
		return doubled
	}
	return pluginActionGiveBackCap
}

// giveBack hands a claim whose re-checks could not answer back to the queue, the
// Job to `queued` under the claim's own token, and keeps trying until that lands
// or the Job has left running. The heartbeat keeps the claim alive until it does,
// and stops once it has. It runs after the VM is released. A Job a person's
// cancellation has won is ended cancelled instead (returnClaim), and this
// admission then leaves its lane.
func (a *pluginActionAdmission) giveBack(execution jobs.Execution, input *pluginActionJobInput, stopHeartbeat func()) {
	done := make(chan struct{})
	a.mu.Lock()
	a.returning = done
	a.givenBack++
	a.mu.Unlock()
	ref := jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken}
	sink := newPluginActionSink(a.ctx, execution, input)
	release := func() error {
		// Said on the Job while it still holds the claim, so the queued row says
		// why it is waiting. A progress snapshot is not an event, and the start
		// that ends the wait replaces it. It is best-effort and bounded: the
		// release after it is what frees the Job's slot of the budget.
		returned, err := sink.returnClaim("admission-unfinished", func() { a.ctx.noteWaitingForChecks(ref) })
		if err == nil && !returned {
			a.mu.Lock()
			a.ending = true
			a.mu.Unlock()
		}
		return err
	}
	go func() {
		defer close(done)
		defer stopHeartbeat()
		if err := release(); err != nil {
			log.Printf("warning: could not give back the claim on plugin job %s; retrying: %v", execution.JobID, err)
			a.ctx.retryPluginActionSettlement(execution.JobID, jobs.StateRunning, release)
		}
	}()
}

// stillReturning reports whether a claim this admission gave back is still on
// its way back.
func (a *pluginActionAdmission) stillReturning() bool {
	returning := a.returningClaim()
	if returning == nil {
		return false
	}
	select {
	case <-returning:
		return false
	default:
		return true
	}
}

func (a *pluginActionAdmission) returningClaim() chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.returning
}

// afterReturn runs then once any claim this admission gave back is in the queue
// again: at once when there is none, and otherwise in the background when it
// lands. A write that assumes the Job is waiting would find it still running
// under the returned token and give up.
func (a *pluginActionAdmission) afterReturn(then func()) {
	returning := a.returningClaim()
	if returning == nil {
		then()
		return
	}
	go func() {
		<-returning
		then()
	}()
}

// endClaimed records why a Job this admission claimed will not run. The write
// runs after the plugin's VM is released, and is retried until it lands.
func (a *pluginActionAdmission) endClaimed(execution jobs.Execution, record func() error) {
	a.mu.Lock()
	a.ending = true
	a.mu.Unlock()
	go a.ctx.settleRefusedPluginAction(execution, record)
}

// endedByItself reports whether this admission refused or failed the Job under
// its own claim.
func (a *pluginActionAdmission) endedByItself() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ending
}

// inputOf is the input the claimed execution runs with: the sealed input it
// opened, which is the durable truth, or the accepting call's own description
// when the Job stores none.
func (a *pluginActionAdmission) inputOf(execution jobs.Execution) (*pluginActionJobInput, error) {
	if len(execution.Input) == 0 && a.input != nil {
		return a.input, nil
	}
	return pluginActionInputOf(execution.Input)
}

// admitted answers the claimed execution, once there is one.
func (a *pluginActionAdmission) admitted() (jobs.Execution, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.execution, a.sink != nil
}

func (a *pluginActionAdmission) liveSink() *pluginActionSink {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sink
}

// Started implements plugin_system.HostJobSink. It is reported as the handler
// is entered, which is the one moment "it started" is true.
func (a *pluginActionAdmission) Started(message string) {
	a.decide(true)
	if sink := a.liveSink(); sink != nil {
		sink.Started(message)
	}
}

// Progress implements plugin_system.HostJobSink.
func (a *pluginActionAdmission) Progress(progress plugin_system.HostProgress) error {
	if sink := a.liveSink(); sink != nil {
		return sink.Progress(progress)
	}
	return nil
}

// Completed implements plugin_system.HostJobSink.
func (a *pluginActionAdmission) Completed(message string, result map[string]any) error {
	if sink := a.liveSink(); sink != nil {
		return sink.Completed(message, result)
	}
	return nil
}

// Failed implements plugin_system.HostJobSink.
func (a *pluginActionAdmission) Failed(failure plugin_system.HostFailure) error {
	if sink := a.liveSink(); sink != nil {
		return sink.Failed(failure)
	}
	return nil
}

// Stopped implements plugin_system.HostJobSink.
func (a *pluginActionAdmission) Stopped(reason string) error {
	if sink := a.liveSink(); sink != nil {
		return sink.Stopped(reason)
	}
	return nil
}

// NotStarted implements plugin_system.HostJobSink. It is only reported for an
// admitted execution, whose own sink decides.
func (a *pluginActionAdmission) NotStarted(reason string) {
	if sink := a.liveSink(); sink != nil {
		sink.NotStarted(reason)
	}
}

// CallbackLost implements plugin_system.HostJobSink.
//
// Once claimed, the execution's own sink decides, exactly as for work claimed at
// acceptance. Before the claim nothing has run, and what a lost callback means
// depends on whether anything else could run the Job: a registered action's input
// is replayable, so it stays queued for the next process to pick up; a closure's
// function and a scheduled occurrence's tick die with this process, so they are
// withdrawn as never started.
func (a *pluginActionAdmission) CallbackLost(reason string) {
	if sink := a.liveSink(); sink != nil {
		sink.CallbackLost(reason)
		return
	}
	if a.subtype == pluginActionSubtypeRegistered || a.waitsForAnyProcess() {
		return
	}
	jobID := a.JobID()
	if jobID == "" {
		// Accepted at admission, and never admitted: there is no Job.
		return
	}
	a.afterReturn(func() {
		a.ctx.settlePluginActionWhile(jobID, jobs.StateQueued, func() error {
			return a.ctx.withdrawPluginActionJob(jobs.Execution{JobID: jobID}, pluginActionNotStartedEvent,
				pluginActionNotStartedMessage(reason))
		})
	})
}

// settleRefusedPluginAction records why a Job this process has just claimed will
// not run, and keeps trying until the record lands.
//
// The execution has already been given up, so a refusal that was not recorded
// would leave the Job running, heartbeated and owned by nothing — holding a slot
// of the deployment's budget and hiding the work from the person waiting for it.
// It is retried the way a refused outcome is (retainUnsettled), and it stops once
// the Job has left running, whoever moved it.
func (ctx *MahresourcesContext) settleRefusedPluginAction(execution jobs.Execution, record func() error) {
	ctx.settlePluginActionWhile(execution.JobID, jobs.StateRunning, record)
}

// settlePluginActionWhile makes one write that ends a Job this process has given
// up, and retries it until it lands or the Job has left the state it was in —
// whoever moved it owns it then. Nothing else would ever end it: no execution in
// this process holds it any more, and while this process is alive no other one
// can prove its work is gone.
func (ctx *MahresourcesContext) settlePluginActionWhile(jobID string, state jobs.State, record func() error) {
	err := record()
	if err == nil {
		return
	}
	log.Printf("warning: could not end plugin job %s; retrying: %v", jobID, err)
	go ctx.retryPluginActionSettlement(jobID, state, record)
}

// retryPluginActionSettlement repeats a write that failed until it lands or the
// Job has left state, and returns then.
func (ctx *MahresourcesContext) retryPluginActionSettlement(jobID string, state jobs.State, record func() error) {
	ticker := time.NewTicker(pluginActionSettlementRetryInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := record(); err == nil {
			return
		}
		snap, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
		if err == nil && snap.State != state {
			return
		}
	}
}

// pluginActionIsSuccessor reports whether a Job was made by a Retry, a Continue
// or a Repeat of another, rather than by its own origin.
func (ctx *MahresourcesContext) pluginActionIsSuccessor(jobID string) (bool, error) {
	lineage, err := ctx.JobService().Lineage(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return false, err
	}
	return len(lineage.Ancestors) > 0, nil
}

// pluginDisabledEverywhere reports whether the deployment has a plugin disabled —
// the durable state every process loads from — rather than merely not loaded in
// this one.
func (ctx *MahresourcesContext) pluginDisabledEverywhere(pluginName string) bool {
	state, err := ctx.GetPluginState(pluginName)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// Never enabled in this deployment.
		return true
	case err != nil:
		// A read that failed proves nothing either way.
		return false
	default:
		return !state.Enabled
	}
}

// pluginActionGiveBackCap is the longest a deferred execution stays out of its
// lane between claims whose re-checks could not answer.
const pluginActionGiveBackCap = 30 * time.Second

// pluginActionAdmissionAttempt bounds one admission's database work: the claim,
// the read of the claimed input and the re-checks of the acting principal. It is
// asked for with the plugin's VM held, so a claim stuck on a lock or an exhausted
// pool, or a re-check stuck behind one, would otherwise hold that plugin's hooks
// and pages — and a shutdown closing its VM — for as long as the stall lasts. An
// admission that runs out is "later": a claim it had been granted goes back to the
// queue, and the VM is given back before it asks again.
var pluginActionAdmissionAttempt = 10 * time.Second

// pluginActionAdmissionAttemptCap is the longest attemptBound grows to.
var pluginActionAdmissionAttemptCap = time.Minute

// pluginActionWaitingForChecks is what a Job whose claim was given back says
// while it waits to be asked again.
const pluginActionWaitingForChecks = "Waiting for the account and scope checks"

// pluginActionWaitingNoticeBound bounds the write of that message, which comes
// before the release of the claim it is written under.
var pluginActionWaitingNoticeBound = time.Second

// noteWaitingForChecks writes pluginActionWaitingForChecks as the progress of a
// Job whose claim is about to be given back, under that claim's token, within
// pluginActionWaitingNoticeBound. A write that does not land in time is skipped.
func (ctx *MahresourcesContext) noteWaitingForChecks(ref jobs.ExecutionRef) {
	deps := ctx.jobDeps()
	if deps.DB != nil {
		bounded, cancel := context.WithTimeout(context.Background(), pluginActionWaitingNoticeBound)
		defer cancel()
		deps.DB = deps.DB.WithContext(bounded)
	}
	if _, err := ctx.JobService().UpdateProgress(deps, ref, jobs.Progress{Message: pluginActionWaitingForChecks}); err != nil {
		log.Printf("warning: could not say why plugin job %s is waiting: %v", ref.JobID, err)
	}
}

// claimPluginActionJobNamed claims one waiting plugin-action Job for this process
// against the deployment's budget, and keeps the claim alive until the returned
// stop is called or the Job leaves running. bounded bounds the claim's database
// work and what ClaimJob does before handing the execution over. An error that
// comes with a claimed execution (ErrExecutionNotLoaded, an UnrunnableClaimError)
// comes with its heartbeat too: the claim is the caller's to settle.
func (ctx *MahresourcesContext) claimPluginActionJobNamed(bounded context.Context, jobID string, claimed func(jobs.ExecutionRef)) (jobs.Execution, func(), error) {
	service := ctx.JobService()
	if service == nil {
		return jobs.Execution{}, func() {}, errors.New("this context has no job control plane installed")
	}
	// The bound rides on the handle the claim's own queries run on, and not on
	// the context ClaimJob is given: that one is what the execution publishes
	// through for the rest of its life, long after the bound has passed.
	deps := ctx.jobDeps()
	if deps.DB != nil {
		deps.DB = deps.DB.WithContext(bounded)
	}
	execution, err := service.ClaimJob(context.Background(), deps, jobs.ClaimRequest{
		Kind:        JobKindPluginAction,
		KindVersion: jobPluginActionKindVersion,
		JobID:       jobID,
		Claimant:    plugin_system.CurrentRuntimeIdentity().String(),
		// The deployment-wide budget, exactly as every other Kind's claim asks for
		// it: a plugin execution is one of the executions `max-job-concurrency`
		// counts.
		Capacity: ctx.hostClaimCapacityBudget(),
		Claimed:  claimed,
	})
	if err != nil && execution.ExecutionToken == "" {
		return jobs.Execution{}, func() {}, err
	}
	return execution, ctx.startPluginActionHeartbeat(execution), err
}

// pluginActionAdoptBatch bounds how many waiting Jobs one adoption pass reads. A
// longer queue is walked over several passes, from where the last one stopped.
const pluginActionAdoptBatch = 100

// pluginActionAdoptDepth bounds how deep adoption fills one plugin's lane in this
// process. A waiting Job needs a lane only when it is about to run, and every
// process of the deployment adopts from the same queue: filling lanes with the
// whole backlog would hold a goroutine and an in-memory entry per waiting Job in
// every process while none of them can run. The rest stay durable and waiting,
// and later passes hand them over as the lane drains.
const pluginActionAdoptDepth = 8

// adoptWaitingPluginActions hands this process's plugin lanes the waiting
// plugin-action Jobs no execution here holds, and answers where the next pass
// continues.
//
// These are the Jobs nothing else would run: a Retry or a Continue the command
// plane accepted, work a stopped process left queued, and work another process of
// the deployment accepted and has not reached yet. Adopting one that another
// process also holds costs a refused claim and nothing more, because the claim is
// what decides who runs it.
//
// A closure-backed Job is never adopted: its callback lives only in the process
// that started it. A scheduled occurrence is adopted only when it is a Retry's
// successor: a fresh one belongs to the scheduler that holds its row's claim and
// records its outcome, and a run elsewhere would leave that row to fire the same
// interval again. Either kind whose process is provably gone can never start, and
// is withdrawn as never started.
//
// Work for a plugin this process does not have loaded is left for a process
// that does, unless the deployment has the plugin disabled: not having it loaded
// here proves nothing about the others. With the plugin loaded here, this
// process's registration is the one it answers from.
func (ctx *MahresourcesContext) adoptWaitingPluginActions(runCtx context.Context, after jobs.Cursor) jobs.Cursor {
	service := ctx.JobService()
	pm := ctx.PluginManager()
	if service == nil || pm == nil {
		return jobs.Cursor{}
	}
	deps := ctx.jobDeps()
	if runCtx != nil && deps.DB != nil {
		deps.DB = deps.DB.WithContext(runCtx)
	}
	waiting, err := service.WaitingJobs(deps, JobKindPluginAction, jobPluginActionKindVersion, after, pluginActionAdoptBatch)
	if err != nil {
		if runCtx == nil || runCtx.Err() == nil {
			log.Printf("job runtime: listing waiting plugin jobs failed: %v", err)
		}
		return after
	}
	next := jobs.Cursor{}
	if len(waiting) == pluginActionAdoptBatch {
		last := waiting[len(waiting)-1]
		next = jobs.Cursor{AcceptedAt: last.AcceptedAt, ID: last.ID}
	}
	// A lane's depth is read once per plugin per pass: an adopted occurrence
	// joins its lane from its own goroutine, so a depth read after handing it
	// over would not count it yet.
	depth := map[string]int{}
	for _, job := range waiting {
		if pm.HostJobHeld(job.ID) {
			continue
		}
		plugin := ""
		if summary, ok := pluginActionSummaryDecoded(job.Summary); ok {
			plugin = summary.Plugin
		}
		if _, read := depth[plugin]; !read {
			depth[plugin] = pm.LaneDepth(plugin)
		}
		if depth[plugin] >= pluginActionAdoptDepth {
			continue
		}
		if ctx.adoptWaitingPluginAction(pm, job) {
			depth[plugin]++
		}
	}
	return next
}

// adoptWaitingPluginAction hands one waiting Job to its plugin's lane here, or
// resolves it when it can never run, and reports whether it now waits in a lane
// of this process.
func (ctx *MahresourcesContext) adoptWaitingPluginAction(pm *plugin_system.PluginManager, job jobs.Snapshot) bool {
	summary, ok := pluginActionSummaryDecoded(job.Summary)
	if !ok {
		return false
	}
	ownedByItsProcess := summary.Subtype == pluginActionSubtypeClosure
	if summary.Subtype == pluginActionSubtypeScheduled {
		successor, err := ctx.pluginActionIsSuccessor(job.ID)
		if err != nil {
			log.Printf("warning: could not read the lineage of plugin job %s: %v", job.ID, err)
			return false
		}
		ownedByItsProcess = !successor
	}
	if ownedByItsProcess {
		identity, ok := plugin_system.ParseRuntimeIdentity(summary.Runtime)
		if ok && identity.Liveness() == plugin_system.RuntimeGone {
			if err := ctx.withdrawPluginActionJob(jobs.Execution{JobID: job.ID}, "not-started",
				"the process that started this job stopped before it ran"); err != nil {
				log.Printf("warning: could not withdraw plugin job %s: %v", job.ID, err)
			}
		}
		return false
	}
	if !pm.IsEnabled(summary.Plugin) && !ctx.pluginDisabledEverywhere(summary.Plugin) {
		// Another process may be the one with this plugin loaded.
		return false
	}

	opened, err := ctx.JobService().OpenReplay(ctx.jobDeps(), jobs.Access{Administrator: true}, job.ID)
	if err != nil {
		// An input that cannot be opened here blocks the Job with the reason a
		// claim would record; anything else is a read that may work next pass.
		if jobs.ReplayBlocked(job.State, err) {
			if blockErr := ctx.blockPluginActionJob(jobs.Execution{JobID: job.ID}, "input-unavailable"); blockErr != nil {
				log.Printf("warning: could not block plugin job %s: %v", job.ID, blockErr)
			}
			return false
		}
		log.Printf("warning: could not open waiting plugin job %s: %v", job.ID, err)
		return false
	}
	input, err := pluginActionInputOf(opened.Input)
	if err != nil {
		if failErr := ctx.failPluginActionJob(jobs.Execution{JobID: job.ID}, "plugin-action-unavailable",
			"the plugin action could not be started"); failErr != nil {
			log.Printf("warning: could not fail plugin job %s: %v", job.ID, failErr)
		}
		return false
	}

	switch input.Subtype {
	case pluginActionSubtypeRegistered:
		if err := ctx.queueRegisteredPluginAction(pm, job.ID, ctx.pluginActionHandleFor(job.ID), job.ActorUserID, input); err != nil {
			log.Printf("warning: could not queue plugin job %s: %v", job.ID, err)
			return false
		}
		return pm.HostJobHeld(job.ID)
	case pluginActionSubtypeScheduled:
		reg, found := ctx.pluginScheduleRegistration(pm, input.Plugin, input.ScheduleID)
		if !found {
			// A row with no live registration is never run, which is what makes a
			// disabled plugin and a renamed schedule id safe without a cleanup
			// pass. The occurrence exists, so it is recorded as unable to run
			// rather than left waiting for a registration that is gone.
			if err := ctx.failPluginActionJob(jobs.Execution{JobID: job.ID}, "schedule-not-declared",
				"the schedule is no longer declared by its plugin"); err != nil {
				log.Printf("warning: could not fail plugin job %s: %v", job.ID, err)
			}
			return false
		}
		var actor uint
		if job.ActorUserID != nil {
			actor = *job.ActorUserID
		}
		go func() {
			if _, err := ctx.runQueuedScheduledOccurrence(pm, job.ID, reg, actor, input); err != nil {
				log.Printf("warning: adopted plugin job %s: %v", job.ID, err)
			}
		}()
		return true
	}
	return false
}
