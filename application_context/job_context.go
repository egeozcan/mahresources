package application_context

import (
	"context"
	"fmt"
	"time"

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

// GetJobOutputs returns one visible Job's typed outputs.
func (ctx *MahresourcesContext) GetJobOutputs(jobID string) ([]jobs.Output, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil, err
	}
	return service.Outputs(ctx.jobDeps(), ctx.jobAccess(), jobID)
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
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.SweepResult{}, err
	}
	return service.Sweep(ctx.jobDeps(), ctx.jobRetentionPolicy(), cursor, limit)
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
