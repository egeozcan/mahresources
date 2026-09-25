package application_context

import (
	"context"
	"fmt"
	"log"
	"time"

	"mahresources/contracts"
	"mahresources/jobs"
)

// This file is the application seam of the Job Center: the authorization-aware
// facade the HTTP and template layers call, and the accessors that read a
// deployment's Job configuration.
//
// It holds no policy of its own. Who may see which Job is the control plane's
// shared predicate, expressed as jobs.Access; what a Job may do is its Kind
// adapter's answer. What is left here is the two things only the application
// knows: which principal this context is asking as, and what the deployment
// configured — both read per call, because a request-scoped context is a shallow
// copy and an operator's setting change must apply to the next call rather than
// to the next restart.

// SetJobService installs the Job control plane this process's facades and
// runtime use.
//
// It is set from main.go rather than built here for one reason: the runtime that
// claims and dispatches work registers its Kind adapters on a *jobs.Service, and
// a facade that held a second instance would read a control plane with no
// adapters registered — so registration would silently do nothing for every read
// path. One process, one control plane.
func (ctx *MahresourcesContext) SetJobService(service *jobs.Service) {
	if ctx == nil {
		return
	}
	ctx.jobService = service
	if service == nil {
		return
	}
	// The Kind adapters this context's own executors publish into are registered
	// with the control plane it was handed, so "one process, one control plane"
	// includes one registry: a facade reading a service with no adapters registered
	// would answer "this process cannot run that work" for work it is running.
	if err := ctx.registerDownloadJobKinds(service); err != nil {
		log.Printf("warning: could not register the download job kinds: %v", err)
	}
	// The queue-backed Kinds are registered here for the same reason: this is the
	// one place a process's control plane is installed, so it is the one place the
	// executors this context owns can be declared to it. A second registration of a
	// Kind already present is skipped rather than refused, so a context handed a
	// populated service — a test, a re-wire — does not fail on wiring order.
	if err := ctx.registerWorkflowJobKinds(service); err != nil {
		log.Printf("warning: could not register the workflow job kinds: %v", err)
	}
	// The plugin-action Kind is registered here for the same reason, and its host
	// half is installed on the plugin manager in the same breath: a manager
	// without it would keep plugin work in memory while the control plane had no
	// executor for the Jobs the HTTP layer had already accepted.
	if err := ctx.registerPluginActionJobKind(service); err != nil {
		log.Printf("warning: could not register the plugin-action job kind: %v", err)
	}
	if err := ctx.registerPluginCommandJobKinds(service); err != nil {
		log.Printf("warning: could not register plugin command job kinds: %v", err)
	}
}

// registerPluginActionJobKind teaches one control plane to run plugin background
// work, and installs the host half of that seam on the plugin manager.
//
// The two halves are one wiring step because they are one feature: the adapter is
// the executor for Jobs accepted from the HTTP layer, and the manager's HostJobs
// is what makes a closure-backed mah.start_job durable. Installing one without
// the other leaves plugin work half-migrated in a way nothing reports.
//
// Idempotent, like the other registrations here: a context may be handed a service
// another caller already populated.
func (ctx *MahresourcesContext) registerPluginActionJobKind(service *jobs.Service) error {
	if ctx == nil || service == nil {
		return nil
	}
	adapter := &pluginActionAdapter{ctx: ctx}
	if !jobs.HasReplayCodec(service, JobKindPluginAction, jobPluginActionKindVersion) {
		if err := service.RegisterReplayCodec(JobKindPluginAction, jobPluginActionKindVersion, pluginActionJobCodec()); err != nil {
			return err
		}
	}
	if _, registered := service.AdapterFor(JobKindPluginAction, jobPluginActionKindVersion); !registered {
		if err := service.RegisterAdapter(adapter); err != nil {
			return err
		}
	}
	if pm := ctx.PluginManager(); pm != nil {
		pm.SetHostJobs(&pluginActionHostJobs{ctx: ctx})
	}
	return nil
}

// JobService returns the installed control plane, or nil when this context was
// built without one — the CLI's read paths, a programmatic embed, and the
// handlers' bare mounts all look like that, and every facade method below says
// so rather than dereferencing nothing.
func (ctx *MahresourcesContext) JobService() *jobs.Service {
	if ctx == nil {
		return nil
	}
	return ctx.jobService
}

// jobAccess is the asking principal as the visibility predicate needs it.
//
// A context with no principal is the implicit administrator, which is the
// no-auth rule every other surface follows: authentication is opt-in, and with it
// off every request runs as an unscoped administrator. It is not a fallback for
// background work — that work passes jobs.Access{Administrator: true} explicitly
// where it opens a Job it owns.
func (ctx *MahresourcesContext) jobAccess() jobs.Access {
	principal := ctx.Principal()
	return jobs.Access{UserID: principal.UserID, Administrator: principal.IsAdmin()}
}

// ListJobs returns one bounded page of the Jobs this context's principal may see.
func (ctx *MahresourcesContext) ListJobs(filter jobs.Filter, cursor jobs.Cursor, limit int) (jobs.Page, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.Page{}, err
	}
	return service.List(ctx.jobDeps(), ctx.jobAccess(), filter, cursor, limit)
}

// ListJobsBefore returns the page immediately newer than a cursor: the page a
// reader goes back to with Previous.
func (ctx *MahresourcesContext) ListJobsBefore(filter jobs.Filter, before jobs.Cursor, limit int) (jobs.Page, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.Page{}, err
	}
	return service.ListBefore(ctx.jobDeps(), ctx.jobAccess(), filter, before, limit)
}

// CountJobsByState counts the visible Jobs matching a filter, by state, with no
// summary window.
func (ctx *MahresourcesContext) CountJobsByState(filter jobs.Filter) (map[string]int64, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil, err
	}
	return service.CountByState(ctx.jobDeps(), ctx.jobAccess(), filter)
}

// VisibleJobKinds names the registered Kinds whose Jobs this principal can see,
// sorted: every Kind for an administrator, and only owner-visible Kinds for
// everybody else, since an admin-class Job is never listed to them.
func (ctx *MahresourcesContext) VisibleJobKinds() []string {
	service := ctx.JobService()
	if service == nil {
		return nil
	}
	admin := ctx.jobAccess().Administrator
	seen := map[string]bool{}
	var kinds []string
	for _, registration := range service.Registrations() {
		definition := registration.Definition
		if seen[definition.Kind] || (!admin && definition.Visibility == jobs.VisibilityAdmin) {
			continue
		}
		seen[definition.Kind] = true
		kinds = append(kinds, definition.Kind)
	}
	return kinds
}

// GetJob returns one Job this context's principal may see, or ErrNotFound — the
// same answer for a Job that does not exist and one they may not see.
func (ctx *MahresourcesContext) GetJob(jobID string) (jobs.Snapshot, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.Snapshot{}, err
	}
	return service.Get(ctx.jobDeps(), ctx.jobAccess(), jobID)
}

// GetJobTimeline returns one visible Job's events from a sequence onwards.
func (ctx *MahresourcesContext) GetJobTimeline(jobID string, afterSequence uint64, limit int) ([]jobs.Event, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil, err
	}
	return service.Timeline(ctx.jobDeps(), ctx.jobAccess(), jobID, afterSequence, limit)
}

// GetPublishedJobEvents returns the resumable stream's next visible events.
func (ctx *MahresourcesContext) GetPublishedJobEvents(afterDelivery uint64, limit int) ([]jobs.Event, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil, err
	}
	return service.PublishedEvents(ctx.jobDeps(), ctx.jobAccess(), afterDelivery, limit)
}

// GetLiveJobProgress returns the visible Jobs whose progress changed after
// since: the live feed beside the durable event stream.
func (ctx *MahresourcesContext) GetLiveJobProgress(since time.Time, sinceID string, limit int) ([]jobs.Snapshot, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil, err
	}
	return service.LiveProgress(ctx.jobDeps(), ctx.jobAccess(), since, sinceID, limit)
}

// GetJobOutputs returns one visible Job's typed outputs.
func (ctx *MahresourcesContext) GetJobOutputs(jobID string) ([]jobs.Output, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil, err
	}
	return service.Outputs(ctx.jobDeps(), ctx.jobAccess(), jobID)
}

// OpenJobOutput resolves and opens one currently available typed output. The
// application rechecks Job visibility, principal capability, and output
// availability for every request before resolving any stored reference.
func (ctx *MahresourcesContext) OpenJobOutput(requestCtx context.Context, jobID, key string) (contracts.JobOutputContent, error) {
	return ctx.openJobOutput(requestCtx, jobID, key)
}

// GetJobLineage returns one visible Job's relatives, as far as the principal may
// see them.
func (ctx *MahresourcesContext) GetJobLineage(jobID string) (jobs.Lineage, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.Lineage{}, err
	}
	return service.Lineage(ctx.jobDeps(), ctx.jobAccess(), jobID)
}

// GetJobSummary returns the bounded aggregate over one filter and window.
func (ctx *MahresourcesContext) GetJobSummary(filter jobs.Filter, window time.Duration) (jobs.Summary, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.Summary{}, err
	}
	return service.Summary(ctx.jobDeps(), ctx.jobAccess(), filter, window)
}

// SetJobPreference records this principal's own view of one Job: dismissal from
// their default list, a pin, or both.
func (ctx *MahresourcesContext) SetJobPreference(request jobs.PreferenceRequest) error {
	service, err := ctx.requireJobService()
	if err != nil {
		return err
	}
	return service.SetPreference(ctx.jobDeps(), ctx.jobAccess(), request)
}

// SweepJobHistory runs one bounded retention pass.
//
// It takes no access: retention is the deployment's own rule about its own disk,
// not a view of somebody's Jobs. Its caller is whatever owns a process lifetime —
// main's cleanup loop, or an operator's command — and the three things that must
// never be swept are enforced inside the sweep rather than by who calls it.
//
// The cursor is a position inside one cycle of the walk, and a caller that keeps
// it must keep the cycle with it: the returned result says where to continue, or
// that the cycle is over and the next pass starts a new one.
func (ctx *MahresourcesContext) SweepJobHistory(cursor jobs.SweepCursor, limit int) (jobs.SweepResult, error) {
	return ctx.SweepJobHistoryContext(context.Background(), cursor, limit)
}

// SweepJobHistoryContext is the cancellable form of SweepJobHistory used by
// the managed retention lifecycle. The database handle and artifact cleanup
// both observe the lifecycle context, so shutdown does not leave a sweep
// running after its owner has stopped.
func (ctx *MahresourcesContext) SweepJobHistoryContext(requestCtx context.Context, cursor jobs.SweepCursor, limit int) (jobs.SweepResult, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.SweepResult{}, err
	}
	if requestCtx == nil {
		requestCtx = context.Background()
	}
	deps := ctx.jobDeps()
	if deps.DB != nil {
		deps.DB = deps.DB.WithContext(requestCtx)
	}
	return service.SweepContext(requestCtx, deps, ctx.jobRetentionPolicy(), cursor, limit)
}

// AdvertisedJobCommands returns the controls one visible Job offers this
// context's principal right now: the Job's Kind answers for its own work and the
// control plane answers for the bookkeeping it owns, so a page renders what the
// server will actually accept rather than what a client guessed from the state.
func (ctx *MahresourcesContext) AdvertisedJobCommands(requestCtx context.Context, jobID string) ([]jobs.Command, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil, err
	}
	return service.AdvertisedCommands(requestCtx, ctx.jobDeps(), ctx.jobAccess(), jobID)
}

// ExecuteJobCommand runs one command as this context's principal.
//
// The actor is replaced with the principal this context is bound to rather than
// taken from the request: every visibility and policy answer a command makes is
// made for the person asking, and a caller that could name its own actor would be
// asking as somebody else. The request's own fields — which Job, which command,
// which idempotency key, which version the client decided from — are the caller's.
func (ctx *MahresourcesContext) ExecuteJobCommand(requestCtx context.Context, request jobs.CommandRequest) (jobs.CommandResult, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.CommandResult{}, err
	}
	request.Actor = ctx.jobAccess()
	return service.ExecuteCommand(requestCtx, ctx.jobDeps(), request)
}

// ReplayJobCommand returns an already-recorded command outcome without applying a
// fresh effect. The legacy Retry route uses it before checking queue state so a
// keyed repeat can receive its original outcome after the handle has moved.
func (ctx *MahresourcesContext) ReplayJobCommand(requestCtx context.Context, request jobs.CommandRequest) (jobs.CommandResult, bool, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.CommandResult{}, false, err
	}
	request.Actor = ctx.jobAccess()
	return service.ReplayCommand(requestCtx, ctx.jobDeps(), request)
}

// ExecuteBulkJobCommand runs one command across a selection as this context's
// principal, with one outcome per Job. A context with no control plane answers
// nothing at all rather than reporting a refusal per Job it never looked at.
func (ctx *MahresourcesContext) ExecuteBulkJobCommand(requestCtx context.Context, request jobs.BulkCommandRequest) []jobs.CommandResult {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil
	}
	request.Actor = ctx.jobAccess()
	return service.ExecuteBulkCommand(requestCtx, ctx.jobDeps(), request)
}

// requireJobService refuses a facade call on a context with no control plane,
// rather than dereferencing nil: an install that forgot SetJobService is a
// wiring mistake, and it should say so once rather than panic in a request.
func (ctx *MahresourcesContext) requireJobService() (*jobs.Service, error) {
	service := ctx.JobService()
	if service == nil {
		return nil, fmt.Errorf("jobs: this context has no job control plane installed")
	}
	return service, nil
}

// jobRetentionPolicy is the deployment's ordinary history retention, read live.
func (ctx *MahresourcesContext) jobRetentionPolicy() jobs.RetentionPolicy {
	return jobs.RetentionPolicy{
		History:   ctx.JobHistoryRetention(),
		Attention: ctx.JobAttentionRetention(),
	}
}

// JobHistoryRetention is how long succeeded and cancelled Jobs stay after they
// finish.
//
// Read through the live settings with the zero-guard the download retentions
// document: a context built from a raw MahresourcesConfig carries no settings
// service, and a published 0 must mean "not configured" rather than "expire on
// write".
func (ctx *MahresourcesContext) JobHistoryRetention() time.Duration {
	if ctx == nil {
		return jobs.DefaultHistoryRetention
	}
	if s := ctx.settings; s != nil {
		if d := s.JobHistoryRetention(); d > 0 {
			return d
		}
	}
	return jobs.DefaultHistoryRetention
}

// JobAttentionRetention is how long failed and interrupted Jobs stay after they
// finish. It is deliberately longer than the ordinary window: work that did not
// succeed is what someone comes back to look at.
func (ctx *MahresourcesContext) JobAttentionRetention() time.Duration {
	if ctx == nil {
		return jobs.DefaultAttentionRetention
	}
	if s := ctx.settings; s != nil {
		if d := s.JobAttentionRetention(); d > 0 {
			return d
		}
	}
	return jobs.DefaultAttentionRetention
}

// JobPinLimit is how many Jobs one viewer may pin, read live with the same
// zero-guard: an unset value selects the default rather than meaning "unlimited",
// because a limit that a missing value removed would be no limit at all.
func (ctx *MahresourcesContext) JobPinLimit() int {
	if ctx == nil {
		return jobs.DefaultPinLimit
	}
	if s := ctx.settings; s != nil {
		if n := s.JobPinLimit(); n > 0 {
			return n
		}
	}
	return jobs.DefaultPinLimit
}
