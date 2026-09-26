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
	jobID   string
	subtype string
	// input is what the execution reports with when its Job stores none: a
	// closure's input is not replayable, so the claim opens nothing and the
	// accepting call's own description stands in for it.
	input *pluginActionJobInput
	// refusal re-checks, under the fresh claim, that the execution may still run
	// as the principal it acts as. It answers nil when it may, and otherwise the
	// write that records what stops it (a block or a failure); a nil refusal
	// re-checks nothing.
	refusal func(execution jobs.Execution, input *pluginActionJobInput) func() error

	mu        sync.Mutex
	execution jobs.Execution
	sink      *pluginActionSink
}

func (ctx *MahresourcesContext) newPluginActionAdmission(jobID string, input *pluginActionJobInput, refusal func(jobs.Execution, *pluginActionJobInput) func() error) *pluginActionAdmission {
	return &pluginActionAdmission{ctx: ctx, jobID: jobID, subtype: input.Subtype, input: input, refusal: refusal}
}

// hostJobRef is the reference plugin_system is handed for this execution.
func (a *pluginActionAdmission) hostJobRef(handle, parentJobID string) *plugin_system.HostJobRef {
	return &plugin_system.HostJobRef{
		JobID: a.jobID, Handle: handle, ParentJobID: parentJobID,
		Sink: a, Admission: a,
	}
}

// Admit claims the Job for this execution.
//
// A full deployment budget is "later" and writes nothing. A Job that is no longer
// waiting — cancelled, blocked, or claimed by another runtime — is withdrawn from
// this process's lane without a word, because whoever moved it owns what happens
// to it now. A deadline bounds the claim's own database work, so a claim waiting
// on a lock cannot carry a bounded caller past its budget.
func (a *pluginActionAdmission) Admit(deadline time.Time) plugin_system.AdmitResult {
	if _, admitted := a.admitted(); admitted {
		return plugin_system.Admitted
	}
	execution, err := a.ctx.claimPluginActionJobNamed(a.jobID, deadline)
	switch {
	case err == nil:
	case errors.Is(err, jobs.ErrCapacityExhausted):
		return plugin_system.AdmitLater
	case errors.Is(err, jobs.ErrJobNotWaiting):
		return plugin_system.AdmitWithdrawn
	case errors.Is(err, context.DeadlineExceeded):
		return plugin_system.AdmitLater
	default:
		// A claim that failed for any other reason may have blocked the Job
		// itself (an input that cannot be opened); the next question finds out.
		log.Printf("warning: could not claim plugin job %s: %v", a.jobID, err)
		return plugin_system.AdmitLater
	}
	input, err := a.inputOf(execution)
	if err != nil {
		a.ctx.settleRefusedPluginAction(execution, func() error {
			return a.ctx.failPluginActionJob(execution, "plugin-action-unavailable",
				"the plugin action could not be started")
		})
		return plugin_system.AdmitWithdrawn
	}
	if a.refusal != nil {
		if record := a.refusal(execution, input); record != nil {
			a.ctx.settleRefusedPluginAction(execution, record)
			return plugin_system.AdmitWithdrawn
		}
	}
	a.mu.Lock()
	a.execution = execution
	a.sink = newPluginActionSink(a.ctx, execution, input)
	a.mu.Unlock()
	return plugin_system.Admitted
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

// Started implements plugin_system.HostJobSink.
func (a *pluginActionAdmission) Started(message string) {
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
func (a *pluginActionAdmission) Failed(message string) error {
	if sink := a.liveSink(); sink != nil {
		return sink.Failed(message)
	}
	return nil
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
	if a.subtype == pluginActionSubtypeRegistered {
		return
	}
	a.ctx.settlePluginActionWhile(a.jobID, jobs.StateQueued, func() error {
		return a.ctx.withdrawPluginActionJob(jobs.Execution{JobID: a.jobID}, "not-started",
			"the process that was going to run this job stopped first")
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
	go func() {
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
	}()
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

// claimPluginActionJobNamed claims one waiting plugin-action Job for this process
// against the deployment's budget, and keeps the claim alive. A non-zero
// deadline bounds the claim's database work.
func (ctx *MahresourcesContext) claimPluginActionJobNamed(jobID string, deadline time.Time) (jobs.Execution, error) {
	service := ctx.JobService()
	if service == nil {
		return jobs.Execution{}, errors.New("this context has no job control plane installed")
	}
	// The deadline rides on the handle the claim's own queries run on, and not on
	// the context ClaimJob is given: that one is what the execution is loaded
	// with after the commit and publishes through for the rest of its life, long
	// after this deadline has passed.
	deps := ctx.jobDeps()
	if !deadline.IsZero() && deps.DB != nil {
		claimCtx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()
		deps.DB = deps.DB.WithContext(claimCtx)
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
	})
	if err != nil {
		return jobs.Execution{}, err
	}
	ctx.startPluginActionHeartbeat(execution)
	return execution, nil
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
// A registered action this process cannot run is left for a process that can,
// unless the deployment has its plugin disabled: not having the plugin loaded
// here proves nothing about the others.
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
	if summary.Subtype == pluginActionSubtypeRegistered {
		if _, _, err := pm.FindAction(summary.Plugin, summary.Action); err != nil && !ctx.pluginDisabledEverywhere(summary.Plugin) {
			return false
		}
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
			if _, err := ctx.runQueuedScheduledOccurrence(pm, job.ID, reg, actor, input, ScheduleDispatchWait); err != nil {
				log.Printf("warning: adopted plugin job %s: %v", job.ID, err)
			}
		}()
		return true
	}
	return false
}
