package application_context

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"mahresources/jobs"
	"mahresources/plugin_system"
)

// This file is the application-owned half of durable dispatch: the loop that
// decides what this process should be running right now, and that stops cleanly
// without inventing an outcome for work it could not finish.
//
// It owns no policy. Which Kinds exist, what they declare, how much concurrency
// they may draw and how a Job's outcome is decided all belong to the registered
// adapters and to the Job control plane; what is left here is cadence (a poll —
// local wake channels would be an optimization, not a correctness mechanism),
// heartbeats, and the shutdown contract.

const (
	// defaultJobRuntimeInterval is how often the loop looks for work. A tick is
	// a bounded pair of queries, and the reconciliation scan is indexed, so a
	// short interval costs little; a slow one is nothing but latency.
	defaultJobRuntimeInterval = 2 * time.Second
	// defaultJobRuntimeQuiesceTimeout bounds how long Stop waits for running
	// executions to return after their context is cancelled. An executor that
	// ignores its context must not hold a deployment open, and the work it
	// abandons stays durable for the next process.
	defaultJobRuntimeQuiesceTimeout = 5 * time.Second
	// jobRuntimeHeartbeatDivisor is how many heartbeats fit in one lease.
	jobRuntimeHeartbeatDivisor = 3
	// minJobRuntimeHeartbeatInterval keeps a Kind that declares a very short
	// lease from turning heartbeats into a hot loop.
	minJobRuntimeHeartbeatInterval = 250 * time.Millisecond

	// jobRuntimeDispatchFailedCode is recorded when a Kind's Dispatch returned an
	// error. The failure is bounded and classed rather than copied from the
	// adapter: only the adapter knows what in its own error text is safe, and a
	// Job's failure record is a taxonomy a reader groups on.
	jobRuntimeDispatchFailedCode = "dispatch-failed"
	// jobRuntimeUnfinishedCode is recorded when an adapter returned while its Job
	// was still running. At-least-once dispatch cannot simply retry: a Job left
	// running with nobody owning it would never be resolved by anything, so the
	// execution's runtime ends it.
	jobRuntimeUnfinishedCode = "execution-unfinished"
)

// JobRuntimeConfig configures the dispatch loop. Every field has a default, so
// the zero value is a working runtime.
type JobRuntimeConfig struct {
	// Claimant identifies this runtime to the control plane. Empty selects
	// host:pid, which is what makes a claim's owner identifiable after the fact.
	Claimant string
	// Interval is how often the loop looks for work.
	Interval time.Duration
	// GlobalCapacity is the deployment-wide concurrency budget, shared by every
	// Kind. 0 selects the deployment's configured budget (the context's
	// Config.MaxJobConcurrency), which is the same number the host-side claim
	// paths take: one deployment budget, one source for it.
	GlobalCapacity int
	// QuiesceTimeout bounds how long Stop waits for running executions to
	// acknowledge cancellation.
	QuiesceTimeout time.Duration
}

// JobRuntime claims and dispatches this process's share of the durable work.
//
// It is built once and started by main, so the place that owns the process
// lifetime is the place that stops it. A runtime with no registered Kind does
// nothing but reconcile: reconciliation is not optional, because work claimed
// when a Kind was registered in a previous process still has to be resolved when
// it is gone.
type JobRuntime struct {
	ctx     *MahresourcesContext
	service *jobs.Service

	claimant       string
	interval       time.Duration
	globalCapacity int
	quiesceTimeout time.Duration

	lifeCtx    context.Context
	cancelLife context.CancelFunc

	stop      chan struct{}
	stopOnce  sync.Once
	startOnce sync.Once
	loopWG    sync.WaitGroup
	execWG    sync.WaitGroup
}

// NewJobRuntime builds the runtime. The lifecycle context exists from
// construction rather than from Start, so a caller that drives single passes —
// a test, an operator's manual tick — gets executions that are cancellable the
// same way a started runtime's are.
func NewJobRuntime(ctx *MahresourcesContext, service *jobs.Service, config JobRuntimeConfig) *JobRuntime {
	if config.Claimant == "" {
		config.Claimant = defaultJobRuntimeClaimant()
	}
	if config.Interval <= 0 {
		config.Interval = defaultJobRuntimeInterval
	}
	// Every claim takes the deployment's concurrency budget, and this is the one
	// place the number comes from: the runtime and the host-side claim paths (a
	// plugin action's own process) share it, so "the deployment is running as much
	// as it may" is one fact rather than two counts that can disagree.
	if config.GlobalCapacity <= 0 && ctx != nil {
		config.GlobalCapacity = ctx.Config.MaxJobConcurrency
	}
	if config.QuiesceTimeout <= 0 {
		config.QuiesceTimeout = defaultJobRuntimeQuiesceTimeout
	}
	lifeCtx, cancelLife := context.WithCancel(context.Background())
	return &JobRuntime{
		ctx:            ctx,
		service:        service,
		claimant:       config.Claimant,
		interval:       config.Interval,
		globalCapacity: config.GlobalCapacity,
		quiesceTimeout: config.QuiesceTimeout,
		lifeCtx:        lifeCtx,
		cancelLife:     cancelLife,
		stop:           make(chan struct{}),
	}
}

// RegisterAdapter teaches this runtime's control plane to run one Kind version.
func (r *JobRuntime) RegisterAdapter(adapter jobs.Adapter) error {
	if r == nil || r.service == nil {
		return fmt.Errorf("jobs: no job runtime is configured")
	}
	return r.service.RegisterAdapter(adapter)
}

// Start begins the loop. It is safe to call on a runtime that was never given a
// service, and safe to call twice.
func (r *JobRuntime) Start() {
	if r == nil {
		return
	}
	r.startOnce.Do(func() {
		r.loopWG.Add(1)
		go r.run()
	})
}

// Stop ends the loop, asks every running execution to quiesce, and leaves
// whatever did not finish exactly where it is.
//
// Cancellation comes first, because it is the *request* to stop: an adapter that
// waits on its context — which is what the interface tells it to do — is holding
// the loop inside a reconciliation, and waiting for that loop to return before
// cancelling it is a wait that can never end. Only after the request has been
// delivered is anything waited for, and both waits are bounded: an executor that
// ignores its context must not hold a deployment open.
//
// Nothing is written on the way out. A Job that was still running keeps its
// state, its claim and its lease, so the next process reconciles it with the
// evidence intact — and a blocked or quarantined Job keeps its capacity, because
// shutting down proves nothing about external work either.
func (r *JobRuntime) Stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		close(r.stop)
		// Cancelling the lifecycle context is the request to quiesce: it reaches
		// the loop's own database work, every execution's context and every
		// publish bound to it.
		r.cancelLife()
		waitForWaitGroup(&r.loopWG, r.quiesceTimeout)
		waitForWaitGroup(&r.execWG, r.quiesceTimeout)
	})
}

// run is the loop. It ticks once immediately — work waiting at boot should not
// wait for an interval — and then on the configured cadence.
func (r *JobRuntime) run() {
	defer r.loopWG.Done()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.tick(r.lifeCtx)
	for {
		select {
		case <-r.stop:
			return
		case <-r.lifeCtx.Done():
			return
		case <-ticker.C:
			r.tick(r.lifeCtx)
		}
	}
}

// tick is one pass: reconcile what nobody owns, then claim and dispatch what
// this process can run.
//
// Reconciliation comes first because the capacity an expired claim holds is
// capacity the dispatch behind it may need, and because a Job whose claim is
// still live must not be handed out a second time. Pending work of a Kind this
// process has no adapter for is blocked in between, for the same reason: such a
// Job is not owned by anybody, so no reconciliation batch would ever reach it and
// nothing here could ever run it.
func (r *JobRuntime) tick(ctx context.Context) {
	if r == nil || r.service == nil || r.ctx == nil {
		return
	}

	report, err := r.service.ReconcileExpired(ctx, r.depsFor(ctx), r.claimant, jobs.DefaultReconcileBatch)
	if err != nil {
		log.Printf("job runtime: reconciliation failed: %v", err)
	}
	for _, execution := range report.Resume {
		adapter, ok := r.service.AdapterFor(execution.Kind, execution.KindVersion)
		if !ok {
			log.Printf("job runtime: resumed job %s names a kind this process cannot run", execution.JobID)
			continue
		}
		r.startExecution(adapter, execution, adapter.Definition().EffectiveLease())
	}

	// A Kind this process cannot run at all leaves its pending work in a state
	// nobody is asked about, so this pass blocks it: nonterminal, visible, and
	// owned by the person who has to decide what happens to it.
	if _, err := r.service.ReconcileUnrunnable(r.depsFor(ctx), jobs.DefaultReconcileBatch); err != nil {
		log.Printf("job runtime: blocking work no adapter can run failed: %v", err)
	}

	for _, registration := range r.service.Registrations() {
		for claimed := 0; claimed < jobs.DefaultClaimBatch; claimed++ {
			execution, ok, err := r.service.Claim(ctx, r.depsFor(ctx), jobs.ClaimRequest{
				Kind:        registration.Definition.Kind,
				KindVersion: registration.Definition.KindVersion,
				Claimant:    r.claimant,
				Capacity:    r.capacityBudget(),
			})
			if err != nil {
				log.Printf("job runtime: claiming %s v%d work failed: %v",
					registration.Definition.Kind, registration.Definition.KindVersion, err)
				break
			}
			if !ok {
				// Nothing waiting, or every budget full: either way this pass has
				// nothing more to do for this Kind.
				break
			}
			r.startExecution(registration.Adapter, execution, registration.Definition.EffectiveLease())
		}
	}
}

// capacityBudget is the deployment-wide budget this runtime asks every claim to
// occupy, on top of the Kind's own.
func (r *JobRuntime) capacityBudget() []jobs.CapacityRef {
	return deploymentCapacityBudget(r.globalCapacity)
}

// deploymentCapacityBudget is the deployment-wide concurrency budget, expressed
// as the claim admission that occupies it. A limit of zero is "not configured",
// never "no budget for this claim": an unenforced budget would admit the very
// concurrency the setting exists to bound.
func deploymentCapacityBudget(limit int) []jobs.CapacityRef {
	if limit <= 0 {
		return nil
	}
	return []jobs.CapacityRef{{Group: jobs.CapacityGroupGlobal, Limit: limit}}
}

// hostClaimCapacityBudget is the budget a claim taken outside the dispatch loop
// occupies — a plugin action, a scheduled occurrence or a closure-backed
// start_job, all of which are admitted by the process that accepted them. It is
// the deployment's own budget, so a host-side execution and a polling runtime's
// execution are admitted against one count rather than two.
func (ctx *MahresourcesContext) hostClaimCapacityBudget() []jobs.CapacityRef {
	if ctx == nil {
		return nil
	}
	return deploymentCapacityBudget(ctx.Config.MaxJobConcurrency)
}

// startExecution runs one claimed execution in its own goroutine, heartbeating
// it while it runs.
func (r *JobRuntime) startExecution(adapter jobs.Adapter, execution jobs.Execution, lease time.Duration) {
	ctx, cancel := context.WithCancel(r.lifeCtx)

	r.execWG.Add(1)
	go func() {
		defer r.execWG.Done()
		defer cancel()

		done := make(chan struct{})
		go r.heartbeatLoop(ctx, execution, lease, done, cancel)
		dispatchErr := adapter.Dispatch(ctx, execution)
		close(done)

		r.finishExecution(execution, dispatchErr)
	}()
}

// heartbeatLoop keeps one execution's claim alive while it runs.
//
// A heartbeat refused because the token no longer owns the Job is not a failure
// to log: it is the fence telling this runtime that its execution was released,
// replaced by a reconciliation or taken over, so the work is cancelled rather
// than left running and publishing into a Job somebody else owns.
func (r *JobRuntime) heartbeatLoop(ctx context.Context, execution jobs.Execution, lease time.Duration, done <-chan struct{}, cancel context.CancelFunc) {
	if lease <= 0 {
		lease = jobs.DefaultClaimLease
	}
	interval := lease / jobRuntimeHeartbeatDivisor
	if interval < minJobRuntimeHeartbeatInterval {
		interval = minJobRuntimeHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ref := jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken}
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := r.service.Heartbeat(r.deps(), ref, lease)
			if err == nil {
				continue
			}
			if errors.Is(err, jobs.ErrStaleExecution) {
				cancel()
				return
			}
			log.Printf("job runtime: heartbeat for job %s failed: %v", execution.JobID, err)
		}
	}
}

// finishExecution reconciles what an execution left behind after its adapter
// returned.
//
// An adapter that ended its Job — success, failure, a return to the queue, a
// pause, a block — is done, and this only hands the claim back. An adapter that
// returned while its Job was still running has left the Job owned by nobody, and
// a Job in that state would never be resolved by anything at all: it is not
// queued, no claim of it expires into a reconciliation, and no executor owns it.
// So the runtime ends it, bounded and classed, and the claim and its capacity go
// with it.
//
// The adapter's own error text is deliberately not recorded or logged. Only the
// adapter knows what in it is safe — a URL, a header, a plugin value — and the
// durable record is a bounded taxonomy instead. An adapter that wants to explain
// itself appends a bounded event before it returns.
func (r *JobRuntime) finishExecution(execution jobs.Execution, dispatchErr error) {
	if r.lifeCtx.Err() != nil {
		// The runtime is stopping: the Job keeps its state, its claim and its
		// lease, and the next process decides what happens to it.
		return
	}

	deps := r.deps()
	ref := jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken}
	snap, err := r.service.Get(deps, jobs.Access{Administrator: true}, execution.JobID)
	if err != nil {
		log.Printf("job runtime: reading job %s after its execution ended failed: %v", execution.JobID, err)
		return
	}

	if snap.State != jobs.StateRunning {
		// The adapter ended its own Job, so the state is its decision and all that is
		// left is to hand the claim back.
		if _, err := r.service.ReleaseClaim(deps, jobs.ReleaseRequest{
			ExecutionRef: ref, Reason: jobs.ReleaseReasonExecutionEnded,
		}); err != nil {
			log.Printf("job runtime: releasing job %s failed: %v", execution.JobID, err)
		}
		return
	}

	code := jobRuntimeUnfinishedCode
	if dispatchErr != nil {
		code = jobRuntimeDispatchFailedCode
	}
	_, err = r.service.Finish(deps, jobs.FinishRequest{
		ExecutionRef:    ref,
		ExpectedVersion: snap.Version,
		Outcome:         jobs.StateFailed,
		Failure:         &jobs.Failure{Code: code, Class: jobs.FailureClassInternal},
	})
	switch {
	case err == nil:
		log.Printf("job runtime: job %s was ended as %s by its runtime", execution.JobID, code)
	case errors.Is(err, jobs.ErrStaleExecution), errors.Is(err, jobs.ErrVersionConflict):
		// A reconciliation or another runtime owns the Job now, which is exactly
		// what the fence is for: nothing to do.
	default:
		log.Printf("job runtime: ending job %s failed: %v", execution.JobID, err)
	}
}

// deps is the per-call handle the control plane runs on. It is rebuilt for every
// call, so a request-scoped or transactional handle is never cached here.
func (r *JobRuntime) deps() jobs.Deps {
	if r.ctx == nil {
		return jobs.Deps{}
	}
	return r.ctx.jobDeps()
}

// depsFor binds the per-call handle to the context whose work it is doing, so
// the database work of a cancelled loop stops with the loop rather than
// continuing behind a request nobody is waiting for any more. It is what makes
// the shutdown request reach the queries as well as the adapters.
func (r *JobRuntime) depsFor(ctx context.Context) jobs.Deps {
	deps := r.deps()
	if ctx != nil && deps.DB != nil {
		deps.DB = deps.DB.WithContext(ctx)
	}
	return deps
}

// defaultJobRuntimeClaimant names this runtime: the host and process that holds
// a claim, which is what an operator needs to identify an abandoned execution —
// and, for a Kind whose work cannot be restored, what an adapter proves liveness
// from.
//
// It is the same process identity a non-restorable Kind records for its own
// host-side executions, boot session included, rather than a second spelling of
// "this process". An identity without a boot session cannot answer the only
// question a plugin-action Job asks when its claim expires — may that process
// still be running a *lua.LFunction? — and a claim that cannot answer it leaves
// the Job blocked for a person instead of resolved. One identity, readable by
// every reconciler in the deployment.
func defaultJobRuntimeClaimant() string {
	identity := plugin_system.CurrentRuntimeIdentity()
	if identity.Host != "" && identity.BootSession != "" {
		return identity.String()
	}
	host := identity.Host
	if host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}

// waitForWaitGroup waits for a wait group, bounded. A false return means the
// bound was reached and something is still running.
func waitForWaitGroup(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
