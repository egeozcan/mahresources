package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mahresources/auth"
	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/plugin_system"
)

// This file is the plugin-action Kind adapter: the durable Job every
// user-visible piece of plugin background work becomes.
//
// One Kind covers three subtypes, because they share everything that matters to
// the control plane — one executor (a Lua callback in this process), one
// ownership rule, one reason they can never be redispatched — and differ only in
// what started them and what a reader is shown. The subtype is a sanitized field
// of the summary, deliberately not a second Kind: three Kinds would be three
// registrations, three visibility rules and three reconciliations for one
// executor, and a reviewer reading any of them would have to read all three to
// know what happens to a plugin job.
//
//	 registered-action   an async mah.action a person ran, through the API or the page
//	 scheduled-occurrence  one occurrence of a mah.schedule, materialized when its row was claimed
//	 closure-start-job   a closure-backed mah.start_job, whose Lua function died with its process
//
// Two properties separate this Kind from the queue-backed ones:
//
//   - **Its work is not restorable, and it says so.** A closure's *lua.LFunction
//     belongs to one *lua.LState in one process, and a registered action is
//     arbitrary Lua the host cannot claim is idempotent. Nothing here may be
//     redispatched after its runtime is gone: the Kind declares itself
//     non-restorable, and its reconciliation interrupts a Job only when the
//     process that owned the callback is *proved* gone. An expired lease proves
//     nothing, so work whose runtime cannot be inspected stays blocked — visible
//     and resolved by a person — rather than being run a second time by a
//     process that cannot know what the first one did.
//
//   - **Its lifecycle is owned here, not by plugin_system.** The plugin manager
//     keeps its VM semaphore, its VM lock and its in-memory ActionJob projection
//     for the compatibility window; identity, state, progress, outcomes and
//     fencing are the control plane's, and the manager reports through the
//     HostJobSink seam in plugin_system/host_jobs.go.
//
// The commands are deliberately narrow. A Retry of an unsuccessful registered
// action is offered because the design's matrix offers it and because the input
// is replayable — the plugin, the action, the entity and the params are all
// re-validated against *current* policy before a successor Job exists, which is
// what makes a Retry an authorization decision rather than a replay of one. No
// Cancel is offered for anything: nothing in a registration declares that its
// handler can be stopped, and a plugin's VM lock is held for the duration of one
// Lua call, so "cancel" would mean either killing a VM mid-write or a button
// whose only outcome is a Job that keeps running. A closure-backed Job offers
// neither: its input cannot be replayed at all.

const (
	// JobKindPluginAction is every plugin background execution a person can see:
	// a registered async action, one scheduled occurrence, and a closure-backed
	// mah.start_job.
	JobKindPluginAction = "plugin-action"
	// jobPluginActionKindVersion is the version of this Kind's input semantics.
	jobPluginActionKindVersion = 1

	// PluginActionHandleNamespace is the legacy id space these Jobs answer in:
	// the short id the jobs panel renders and GET /v1/jobs/action/job resolves.
	// One namespace for all three subtypes, because it is one id space — the ids
	// are all minted by this Kind and a handle from it must never resolve as one
	// of another Kind's.
	PluginActionHandleNamespace = "plugin-action"
)

// Subtypes. They are stored in the input and copied into the sanitized summary,
// and they are what tells the adapter which executor to reach for.
const (
	pluginActionSubtypeRegistered = "registered-action"
	pluginActionSubtypeScheduled  = "scheduled-occurrence"
	pluginActionSubtypeClosure    = "closure-start-job"
)

// Bounds on what a plugin's own reports may write. Progress and failure text
// arrive from Lua, so they are truncated here rather than refused: refusing would
// turn a chatty plugin into a failing Job, and the control plane's own ceilings
// are what the truncation respects.
const (
	// maxPluginActionResultBytes bounds the result table published as an output.
	// A plugin that returns a document is offering it to whoever opens the Job;
	// one that returns something larger than this is not, and the Job still
	// succeeds — the output is optional by construction, so a result nobody can
	// store must not fail the work that produced it.
	maxPluginActionResultBytes = 6 << 10
	// pluginActionFailureCode and pluginActionFailureMessage are the bounded,
	// host-owned failure one plugin execution records. The code is what a reader
	// groups on and the message is what the Job Center shows: neither is derived
	// from what the plugin said, because a script's own words are arbitrary text
	// and a Job's failure is durable history the sealed input cannot protect.
	pluginActionFailureCode    = "plugin-action-failed"
	pluginActionFailureMessage = "the plugin's handler failed"
	// pluginActionRedactionMarker replaces any value of the Job's own parameters
	// that a plugin puts into a message the host persists.
	pluginActionRedactionMarker = "[redacted]"
	// pluginActionWaitPollInterval is how often a dispatched execution looks at its
	// Job while waiting for the plugin manager's report.
	pluginActionWaitPollInterval = 25 * time.Millisecond
	// minRedactedParamBytes is the shortest parameter value worth replacing. Below
	// it a value is a fragment of ordinary prose — "1", "on" — and replacing it
	// would mangle every message without hiding anything a person would call a
	// secret.
	minRedactedParamBytes = 3
)

// pluginActionJobInput is what a plugin-action Job is accepted with.
//
// It is the encrypted envelope's payload, so params travel in it — they are the
// plugin's own values, they may be sensitive, and the design is explicit that
// replay input is never returned by an ordinary Job API. The sanitized *summary*
// carries none of them.
type pluginActionJobInput struct {
	Subtype    string         `json:"subtype"`
	Plugin     string         `json:"plugin"`
	Action     string         `json:"action,omitempty"`
	ScheduleID string         `json:"scheduleId,omitempty"`
	Label      string         `json:"label,omitempty"`
	EntityID   uint           `json:"entityId,omitempty"`
	EntityType string         `json:"entityType,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
	// Fingerprint is the registration fingerprint the submitter validated
	// against. Dispatch re-derives it and refuses when it differs, which is what
	// stops a disable/edit/re-enable between the request and the run from
	// executing a handler whose filters the caller's entities were never checked
	// against.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Overlap is the schedule row's overlap policy for a scheduled occurrence.
	// It rides in the input because it is a property of *this dispatch*: "skip"
	// means the occurrence holds its row for the whole run, "allow" means the row
	// was advanced before it started.
	Overlap string `json:"overlap,omitempty"`
	// Runtime names the process that owns the callback, when the subtype is one
	// whose work cannot be restored. It is what reconciliation reads instead of
	// guessing from a lease.
	Runtime string `json:"runtime,omitempty"`
	// NoTerminalHook marks a Job whose outcome is not announced to the plugin hook
	// feed, because the work itself came from that feed: an after_job_* handler
	// that starts work must not be handed the completion of the work it started,
	// or every completion would start another Job. It is recorded durably rather
	// than held in memory because the Job outlives the call chain that decided it,
	// and it is inherited down a lineage so the suppression covers descendants
	// too.
	NoTerminalHook bool `json:"noTerminalHook,omitempty"`
}

// pluginActionSummary is the bounded, sanitized, *searchable* view. It carries
// where the work came from and what it acts on — never a param, never a value the
// plugin supplied.
type pluginActionSummary struct {
	Subtype    string `json:"subtype"`
	Plugin     string `json:"plugin"`
	Action     string `json:"action,omitempty"`
	ScheduleID string `json:"scheduleId,omitempty"`
	EntityID   uint   `json:"entityId,omitempty"`
	EntityType string `json:"entityType,omitempty"`
	Runtime    string `json:"runtime,omitempty"`
	// NoTerminalHook is the causal-suppression flag, carried in the readable
	// summary so a child Job can inherit it from its parent without anybody having
	// to open a sealed envelope to find out.
	NoTerminalHook bool `json:"noTerminalHook,omitempty"`
}

// pluginActionJobCodec is this Kind's replay codec: it validates the shape on the
// way in, sanitizes the summary, and refuses a version it does not understand.
func pluginActionJobCodec() jobs.ReplayCodec {
	return jobs.ReplayCodec{
		Sanitize: func(input json.RawMessage) (json.RawMessage, error) {
			decoded, err := pluginActionInputOf(input)
			if err != nil {
				return nil, err
			}
			return json.Marshal(pluginActionSummary{
				Subtype:        decoded.Subtype,
				Plugin:         decoded.Plugin,
				Action:         decoded.Action,
				ScheduleID:     decoded.ScheduleID,
				EntityID:       decoded.EntityID,
				EntityType:     decoded.EntityType,
				Runtime:        decoded.Runtime,
				NoTerminalHook: decoded.NoTerminalHook,
			})
		},
		Encode: func(input json.RawMessage) (json.RawMessage, error) {
			if _, err := pluginActionInputOf(input); err != nil {
				return nil, err
			}
			return input, nil
		},
		Decode: func(payload json.RawMessage, version uint) (json.RawMessage, error) {
			if version != jobPluginActionKindVersion {
				return nil, fmt.Errorf("%w: plugin-action v%d input", jobs.ErrReplayCodecUnregistered, version)
			}
			if _, err := pluginActionInputOf(payload); err != nil {
				return nil, err
			}
			return payload, nil
		},
		Migrate: func(payload json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error) {
			if fromVersion != toVersion {
				return nil, fmt.Errorf("jobs: no plugin-action input migration from v%d to v%d", fromVersion, toVersion)
			}
			return payload, nil
		},
	}
}

// pluginActionInputOf reads and validates one input. Every wrong shape is refused
// by name: an input this release cannot run is a Job nobody may dispatch, and
// letting it through to a zero value would run the wrong executor.
func pluginActionInputOf(input json.RawMessage) (*pluginActionJobInput, error) {
	if len(input) == 0 || !json.Valid(input) {
		return nil, errors.New("a plugin-action Job's input is not valid JSON")
	}
	var decoded pluginActionJobInput
	if err := json.Unmarshal(input, &decoded); err != nil {
		return nil, fmt.Errorf("a plugin-action Job's input is not readable: %w", err)
	}
	switch decoded.Subtype {
	case pluginActionSubtypeRegistered:
		if decoded.Plugin == "" || decoded.Action == "" {
			return nil, errors.New("a registered plugin action names no plugin or action")
		}
	case pluginActionSubtypeScheduled:
		if decoded.Plugin == "" || decoded.ScheduleID == "" {
			return nil, errors.New("a scheduled occurrence names no plugin or schedule")
		}
		if decoded.Overlap != plugin_system.ScheduleOverlapSkip && decoded.Overlap != plugin_system.ScheduleOverlapAllow {
			return nil, fmt.Errorf("a scheduled occurrence carries no overlap policy this release knows: %q", decoded.Overlap)
		}
	case pluginActionSubtypeClosure:
		if decoded.Plugin == "" {
			return nil, errors.New("a closure-backed job names no plugin")
		}
		if _, ok := plugin_system.ParseRuntimeIdentity(decoded.Runtime); !ok {
			return nil, errors.New("a closure-backed job records no originating runtime")
		}
	default:
		return nil, fmt.Errorf("this release runs no plugin-action subtype named %q", decoded.Subtype)
	}
	return &decoded, nil
}

// pluginActionAdapter runs one plugin-action Job.
type pluginActionAdapter struct {
	ctx *MahresourcesContext
}

// Definition declares the Kind non-restorable, and it is the *Kind* that decides
// that rather than the caller: a plugin's Lua callback dies with the process that
// held it, whether it was an action a person ran or a closure a plugin started,
// and no lease expiry can prove otherwise.
func (a *pluginActionAdapter) Definition() jobs.Definition {
	return jobs.Definition{
		Kind:        JobKindPluginAction,
		KindVersion: jobPluginActionKindVersion,
		Restorable:  false,
		Visibility:  jobs.VisibilityOwner,
	}
}

// Dispatch runs one claimed plugin-action execution.
//
// It does not own the Job's lifecycle: it re-validates the execution against
// current policy and then hands it to the executor, whose reports go through the
// execution's own sink. A refusal here blocks the Job — visible, with a bounded
// reason, on the timeline — rather than failing it, because a disabled plugin or
// a target that has left the caller's subtree is a decision for a person, not an
// error the work produced.
func (a *pluginActionAdapter) Dispatch(ctx context.Context, execution jobs.Execution) error {
	if a.ctx == nil {
		return errors.New("the application context is not available")
	}
	input, err := pluginActionInputOf(execution.Input)
	if err != nil {
		return err
	}
	run, err := a.ctx.runPluginActionExecution(ctx, execution, input)
	if err != nil {
		return err
	}
	if !run.Started {
		// Nothing was entered, so there is nothing to wait for: the Job was
		// blocked, failed or withdrawn by the executor and owns no running state.
		return nil
	}
	if _, err := a.ctx.awaitPluginActionRun(ctx, execution); err != nil {
		return err
	}
	return nil
}

// RunPluginActionAsync is the one door an async plugin action is submitted
// through: the HTTP layer hands it the plugin, the action, the entity and the
// params, and it answers the ids the client and the Job Center use.
//
// The order is the design's and it is not negotiable: the durable Job is accepted
// *and claimed* before the handler's goroutine exists, so the work is durable
// before anything runs and one execution owns exactly one Job under one fencing
// token. Re-validation happens on the execution path rather than here, so a
// request and a Retry are checked the same way.
//
// It returns as soon as the work has started — an async action exists precisely so
// a person is not held to the VM's schedule — and answers (legacy id, canonical
// job id, error): the legacy id is what the panel and GET /v1/jobs/action/job
// resolve, the canonical id is what the Job Center lists.
func (ctx *MahresourcesContext) RunPluginActionAsync(owner *uint, pluginName, actionID string, entityID uint, params map[string]any, expectFilters string) (string, string, error) {
	pm := ctx.PluginManager()
	if pm == nil {
		return "", "", errors.New("the plugin system is not available")
	}
	service := ctx.JobService()
	if service == nil {
		// No control plane: the in-memory record is the whole lifecycle, exactly
		// as it was before this Kind existed.
		jobID, err := pm.RunActionAsyncForOwner(owner, pluginName, actionID, entityID, params, expectFilters)
		return jobID, "", err
	}

	action, _, err := pm.FindAction(pluginName, actionID)
	if err != nil {
		return "", "", err
	}

	handle := download_queue.NewJobID()
	input, err := json.Marshal(pluginActionJobInput{
		Subtype:     pluginActionSubtypeRegistered,
		Plugin:      pluginName,
		Action:      actionID,
		Label:       truncateTo(action.Label, jobs.MaxTitleBytes),
		EntityID:    entityID,
		EntityType:  action.Entity,
		Params:      params,
		Fingerprint: expectFilters,
		Runtime:     plugin_system.CurrentRuntimeIdentity().String(),
	})
	if err != nil {
		return "", "", err
	}

	title := action.Label
	if title == "" {
		title = pluginName + ": " + actionID
	}
	accepted, err := service.Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind:        JobKindPluginAction,
		KindVersion: jobPluginActionKindVersion,
		State:       jobs.StateQueued,
		OwnerUserID: owner,
		ActorUserID: owner,
		Origin:      "plugin",
		Title:       truncateTo(title, jobs.MaxTitleBytes),
		Replay:      jobs.ReplayInput{Input: input},
		LegacyRefs:  []jobs.LegacyRef{{Namespace: PluginActionHandleNamespace, Handle: handle}},
	})
	if err != nil {
		return "", "", err
	}

	claim, err := ctx.claimPluginActionJobForHost(accepted.ID)
	if err != nil {
		return "", "", err
	}
	if !claim.Owned {
		// This process does not own the Job, and nothing is started here.
		//
		// Two facts look alike from here: another runtime of the deployment claimed
		// the Job in the window between acceptance and this call, and the claim was
		// refused because the deployment's concurrency budget is full. Both answer
		// the same way, and neither is a failure: the Job is one the dispatch loop
		// owns — queued, visible, and a registered action is exactly what a loop
		// with the plugin loaded can run — so the client gets the ids it polls and
		// the work runs when a slot is free. §3 asks each runtime to run its share
		// of durable work; it does not require the process that touched the request
		// to be the one that runs it.
		//
		// Withdrawing it here instead — which is what this did before the budget was
		// asked for — would report a full deployment as a broken plugin and destroy
		// work acceptance had already promised.
		return handle, accepted.ID, nil
	}

	decoded, err := pluginActionInputOf(input)
	if err != nil {
		return "", "", err
	}
	if _, err := ctx.runPluginActionExecution(context.Background(), claim.execution, decoded); err != nil {
		return "", "", err
	}
	return handle, accepted.ID, nil
}

// pluginActionRun is what running one execution produced, for callers that need
// the outcome rather than only the Job's state — the plugin scheduler, which
// records the schedule row's own history from it.
type pluginActionRun struct {
	// JobID is the durable Job this execution published into.
	JobID string
	// Started reports whether the handler was entered at all. False means the
	// execution gave up before entering it (a busy VM, a full job budget), which
	// is "not this time" rather than a failed run.
	Started bool
	// Failed reports that the handler was entered and ended unsuccessfully.
	Failed bool
	// Message is the bounded message the run produced.
	Message string
}

// runPluginActionExecution is the one executor path for all three subtypes: it
// re-validates, hands the work to the plugin manager, and reports back what the
// manager did with it.
func (ctx *MahresourcesContext) runPluginActionExecution(runCtx context.Context, execution jobs.Execution, input *pluginActionJobInput) (pluginActionRun, error) {
	pm := ctx.PluginManager()
	if pm == nil {
		return pluginActionRun{JobID: execution.JobID}, ctx.blockPluginActionJob(execution, "plugins-unavailable")
	}

	switch input.Subtype {
	case pluginActionSubtypeRegistered:
		return ctx.runRegisteredPluginAction(pm, execution, input)
	case pluginActionSubtypeScheduled:
		return ctx.runScheduledPluginOccurrence(pm, execution, input, ScheduleDispatchWait)
	default:
		// A closure-backed Job reaching dispatch is usually one whose callback is
		// gone: start_job claims its Job before it runs, so a claimed-but-
		// undispatched closure has no Lua function behind it any more. What to do
		// about that is the reconciliation question, and answering it here — by
		// running the work — is exactly the replacement dispatch §3 forbids.
		//
		// One case is not loss: the callback *is* running here, in this process,
		// reporting through the sink start_job handed it. That happens when the
		// dispatch loop claimed the Job in the window between its acceptance and
		// the host's own claim — the work is in flight, so this waits for it
		// instead of blocking a Job that is about to succeed.
		if handle := ctx.pluginActionHandleFor(execution.JobID); handle != "" {
			if running := pm.GetActionJob(handle); running != nil && !actionJobTerminal(running.Status) {
				return ctx.awaitPluginActionRun(runCtx, execution)
			}
		}
		if err := ctx.blockPluginActionJob(execution, "closure-callback-gone"); err != nil {
			return pluginActionRun{JobID: execution.JobID}, err
		}
		return pluginActionRun{JobID: execution.JobID}, nil
	}
}

// runRegisteredPluginAction runs one async action for one entity.
//
// The re-validation is the whole point of this function. Between the request that
// accepted the Job and the moment it runs, a plugin may have been disabled or
// edited, an account's scope may have narrowed, and a role may have been
// revoked — so plugin availability, the registration fingerprint, the params, the
// acting principal's write role, its access to the plugin and its reach over the
// target entity are all asked again, now, for the principal the Job acts as.
func (ctx *MahresourcesContext) runRegisteredPluginAction(pm *plugin_system.PluginManager, execution jobs.Execution, input *pluginActionJobInput) (pluginActionRun, error) {
	refusal := ctx.pluginActionRefusal(pm, execution, input)
	if refusal != "" {
		if err := ctx.blockPluginActionJob(execution, refusal); err != nil {
			return pluginActionRun{JobID: execution.JobID}, err
		}
		return pluginActionRun{JobID: execution.JobID}, nil
	}

	ref := &plugin_system.HostJobRef{
		JobID:  execution.JobID,
		Handle: ctx.pluginActionHandleFor(execution.JobID),
		Sink:   newPluginActionSink(ctx, execution, input),
	}
	if _, err := pm.RunActionAsyncForHost(ref, ctx.pluginActionOwner(execution), input.Plugin, input.Action, input.EntityID, input.Params, input.Fingerprint); err != nil {
		// The manager refused after the Job was already claimed: the registration
		// changed under us, or the plugin stopped. The Job is failed rather than
		// blocked because the executor's own refusal is an initiation failure —
		// nothing ran — and a Retry re-validates.
		return pluginActionRun{JobID: execution.JobID}, ctx.failPluginActionJob(execution, "plugin-action-unavailable",
			"the plugin action could not be started")
	}
	// The handler's goroutine is running and reports through the sink; this does
	// not wait for it. A request must not be held to the VM's schedule, and the
	// runtime that dispatched this execution waits for the outcome itself —
	// see Dispatch.
	return pluginActionRun{JobID: execution.JobID, Started: true}, nil
}

// runScheduledPluginOccurrence runs one materialized occurrence of a schedule.
//
// holdClaim mirrors the row's overlap policy, exactly as the scheduler's own
// inline run did: under "skip" the occurrence holds its row for the whole run, so
// the VM wait must stay bounded by the dispatch budget or the claim lapses
// mid-dispatch; under "allow" the row was advanced before the run and the VM wait
// is what queues the next occurrence behind this one.
func (ctx *MahresourcesContext) runScheduledPluginOccurrence(pm *plugin_system.PluginManager, execution jobs.Execution, input *pluginActionJobInput, wait time.Duration) (pluginActionRun, error) {
	reg, found := ctx.pluginScheduleRegistration(pm, input.Plugin, input.ScheduleID)
	if !found {
		// A row with no live registration is never run, which is what makes a
		// disabled plugin and a renamed schedule id safe without a cleanup pass.
		// The occurrence was already materialized when the row was claimed, so
		// the honest answer is to record that it cannot run rather than to
		// pretend the tick never happened.
		if err := ctx.failPluginActionJob(execution, "schedule-not-declared",
			"the schedule is no longer declared by its plugin"); err != nil {
			return pluginActionRun{JobID: execution.JobID}, err
		}
		return pluginActionRun{JobID: execution.JobID, Failed: true}, nil
	}

	// The operator's authority is rechecked immediately before the handler runs, for
	// the same reason a registered action's is: the row was claimed by a scheduler
	// that carries no request, and the operator who enabled the plugin may have been
	// demoted or may have lost access to it since. A materialized occurrence that may
	// no longer run is blocked rather than failed — a person has to decide about it,
	// and a broken schedule would be the wrong thing to report.
	if refusal := ctx.commandActorRefusal(ctx.jobDeps(), execution.Access, input.Plugin); refusal != "" {
		if err := ctx.blockPluginActionJob(execution, refusal); err != nil {
			return pluginActionRun{JobID: execution.JobID}, err
		}
		return pluginActionRun{JobID: execution.JobID}, nil
	}

	ref := &plugin_system.HostJobRef{
		JobID:  execution.JobID,
		Handle: ctx.pluginActionHandleFor(execution.JobID),
		Sink:   newPluginActionSink(ctx, execution, input),
	}
	actor := accessUserID(execution.Access)
	_, ran, _ := pm.RunScheduleForHost(reg, actor, wait,
		input.Overlap == plugin_system.ScheduleOverlapSkip, ref)
	if !ran {
		// The handler was never entered. The Job is withdrawn rather than failed:
		// errJobDidNotStart's doctrine is that a full budget or a busy VM is not a
		// plugin's failure, and a Job that ended `failed` here would put a failure
		// on the timeline of work that never began. It ends `cancelled` with a
		// bounded not-started event, which is what tells the scheduler that its
		// row gets its claim back and no outcome is recorded.
		if err := ctx.withdrawPluginActionJob(execution, "not-started", "the plugin's execution budget or VM stayed busy"); err != nil {
			return pluginActionRun{JobID: execution.JobID}, err
		}
		return pluginActionRun{JobID: execution.JobID}, nil
	}
	// RunScheduleForHost blocks until the handler has finished, so the Job's own
	// outcome is already recorded by the time this returns; the wait here is the
	// confirmation of that, and the scheduler holds its own row claim for it.
	return ctx.awaitPluginActionRun(context.Background(), execution)
}

// awaitPluginActionRun waits for the Job's own terminal state and reports it.
//
// There is deliberately no wall-clock bound, because there is no honest one. The
// executor does not report on the dispatcher's schedule: the plugin manager
// accepts the work and runs it when a VM and a job slot are free, which can be
// behind another plugin's five-minute Lua call (plugin_system's own job-slot wait
// is deliberate and unbounded for exactly that reason). A bound here would
// therefore expire while the callback was still queued or running, and the
// runtime would record `dispatch-failed` on a Job whose work was alive — a Job the
// Job Center then offers to Retry while the original handler can still run.
//
// So the wait has exactly two ends: the Job reaching an outcome, and the runtime
// that owns the execution being told to stop. While it waits the claim is held and
// heartbeated, so the work is not handed to anybody else; if the runtime stops,
// the Job keeps its state, its claim and its lease, and the next process
// reconciles it with the proof §3 requires rather than with a guess from a clock.
// It is a poll because the plugin manager's completion is a sink call rather than
// a channel this layer can select on.
func (ctx *MahresourcesContext) awaitPluginActionRun(runCtx context.Context, execution jobs.Execution) (pluginActionRun, error) {
	service := ctx.JobService()
	if service == nil {
		return pluginActionRun{JobID: execution.JobID, Started: true}, nil
	}
	if runCtx == nil {
		runCtx = context.Background()
	}
	for {
		snap, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, execution.JobID)
		if err != nil {
			// A read that failed is not a Job that failed: the execution is still
			// alive, so the honest thing is to keep waiting for its report rather
			// than to end work somebody is running.
			log.Printf("warning: could not read plugin job %s while waiting for it: %v", execution.JobID, err)
		} else if snap.State.Terminal() {
			run := pluginActionRun{
				JobID:   execution.JobID,
				Started: true,
				Failed:  snap.State == jobs.StateFailed || snap.State == jobs.StateInterrupted,
			}
			if snap.Failure != nil {
				run.Message = snap.Failure.Message
			}
			return run, nil
		}
		select {
		case <-runCtx.Done():
			// The runtime is stopping (or the fence released this execution): the Job
			// is left exactly where it is, owned by nobody here, for reconciliation.
			return pluginActionRun{JobID: execution.JobID, Started: true}, nil
		case <-time.After(pluginActionWaitPollInterval):
		}
	}
}

// pluginActionRefusal answers why this execution may not run as the principal it
// acts as, or an empty string.
//
// Every check is asked of the *acting* principal, resolved from the actor id the
// Job recorded: a Retry runs as whoever asked for the retry, and a scheduled
// occurrence runs as the operator who enabled the plugin. An actor whose account
// cannot be read — deleted or disabled — resolves to this tree's deny-all
// identity rather than to an unscoped one, which is what makes the scope question
// below answer "no" instead of "everywhere".
func (ctx *MahresourcesContext) pluginActionRefusal(pm *plugin_system.PluginManager, execution jobs.Execution, input *pluginActionJobInput) string {
	action, _, err := pm.FindAction(input.Plugin, input.Action)
	if err != nil {
		// FindAction refuses for both "no such plugin" and "no such action", and
		// the two are one answer here: neither can be run.
		return "action-unavailable"
	}
	if input.Fingerprint != "" && plugin_system.ActionFiltersFingerprint(action.Filters) != input.Fingerprint {
		return "registration-changed"
	}
	if validationErrs := plugin_system.ValidateActionParams(action, input.Params); len(validationErrs) > 0 {
		return "params-changed"
	}

	actorID := accessUserID(execution.Access)
	principal := ctx.principalForPluginActor(actorID)
	if principal != nil {
		scoped := ctx.WithPrincipal(principal)
		if err := scoped.requireWriteRole("run a plugin action"); err != nil {
			return "role-refused"
		}
		if !auth.PluginActionAccessFor(auth.WithPrincipal(context.Background(), principal),
			ctx.PluginAllowsScopedPrincipals)(input.Plugin) {
			return "plugin-refused"
		}
		// The target's scope is checked as the acting principal, not as whoever
		// submitted the Job: an entity that has since moved out of a narrowed
		// subtree must not be acted on by a Job that was accepted while it was
		// inside.
		switch action.Entity {
		case "resource":
			if !scoped.ResourceVisible(input.EntityID) {
				return "target-out-of-scope"
			}
		case "note":
			if !scoped.NoteVisible(input.EntityID) {
				return "target-out-of-scope"
			}
		case "group":
			if !scoped.GroupVisible(input.EntityID) {
				return "target-out-of-scope"
			}
		}
	}
	return ""
}

// Reconcile answers what should happen to one plugin-action Job whose claim
// expired.
//
// The only question it can answer honestly is whether the process that owned the
// callback is provably gone, and what it asks is the *execution* identity: the
// claimant the claim recorded, which is the runtime that took the Job when it was
// dispatched. Not the submission provenance in the sealed input, which names
// whoever accepted the Job and is immutable — a Retry submitted here and claimed
// there would otherwise be judged by a process that has nothing to do with the
// work, and a live execution would be interrupted because a dead one once held
// the same input.
//
//   - proved gone (this host has rebooted since, or the pid no longer exists):
//     interrupt. The *lua.LFunction cannot run again, and §3's rule for
//     non-restorable work is that a proven loss ends the execution rather than
//     restarting it. The Job keeps its history and offers no Retry when its input
//     cannot be replayed.
//   - anything else (alive, another host, an identity no reconciler can read):
//     blocked-external-work-unproven. The Job keeps its claim and its capacity,
//     no replacement is dispatched, and a person decides.
func (a *pluginActionAdapter) Reconcile(_ context.Context, request jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
	identity, ok := plugin_system.ParseRuntimeIdentity(request.Claimant)
	if !ok {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	if identity.Liveness() == plugin_system.RuntimeGone {
		return jobs.ReconcileInterrupt, nil
	}
	return jobs.ReconcileExternalWorkUnproven, nil
}

// CleanupArtifacts accounts for one plugin-action Job's outputs: this Kind
// publishes a result summary at most, and a summary has no bytes of its own to
// establish the absence of.
func (a *pluginActionAdapter) CleanupArtifacts(_ context.Context, _ jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
	return jobs.ArtifactCleanupResult{}, nil
}

// Commands reports what one plugin-action Job offers.
//
// A Retry only, and only where a replay would mean something: an unsuccessful
// registered action or scheduled occurrence whose registration *declares* that
// re-running the handler with the same input is safe. Nothing here happens by
// state alone. The host cannot know whether arbitrary Lua is idempotent — the
// adapter's own contract says it is not — so Retry is an opt-in an author
// writes (ActionRegistration.Retryable / ScheduleRegistration.Retryable) and the
// host enforces by advertising nothing without it. A closure-backed Job offers
// nothing either way: its Lua function died with its process and no input can
// bring it back. No Cancel, for the reason the file comment gives.
//
// Reading the registration is also what makes this a *current* answer rather
// than a recorded one: a disabled plugin, a replaced generation and an action
// deleted from a newer plugin.lua all take the option away the moment they
// happen, and the command plane re-asks this question inside the transaction
// that would create the successor.
//
// The control plane decides the rest: a terminal Job, a Job whose successor is
// already running, and a Job whose lineage is hidden from this viewer are all
// answered there rather than here.
func (a *pluginActionAdapter) Commands(_ context.Context, commandContext jobs.CommandContext) ([]jobs.Command, error) {
	switch commandContext.Snapshot.State {
	case jobs.StateFailed, jobs.StateCancelled, jobs.StateInterrupted:
	default:
		return nil, nil
	}
	// The subtype comes from the sanitized summary rather than from the sealed
	// input: an advertisement is a read, and a read path that opened every Job's
	// replay envelope to answer "does this offer Retry?" would decrypt history on
	// every list render.
	summary, ok := pluginActionSummaryDecoded(commandContext.Snapshot.Summary)
	if !ok || a.ctx == nil {
		return nil, nil
	}
	// §8: ownership grants visibility, not permanent control. A principal demoted below
	// "may write" — or one whose access to the plugin has since been revoked — keeps
	// the sanitized history and loses the Retry that would run the plugin's Lua again on
	// its behalf.
	if a.ctx.commandActorRefusal(commandContext.Deps, commandContext.Access, summary.Plugin) != "" {
		return nil, nil
	}
	pm := a.ctx.PluginManager()
	if pm == nil {
		return nil, nil
	}
	switch summary.Subtype {
	case pluginActionSubtypeRegistered:
		action, _, err := pm.FindAction(summary.Plugin, summary.Action)
		if err != nil || !action.Retryable {
			return nil, nil
		}
	case pluginActionSubtypeScheduled:
		reg, found := a.ctx.pluginScheduleRegistration(pm, summary.Plugin, summary.ScheduleID)
		if !found || !reg.Retryable {
			return nil, nil
		}
	default:
		return nil, nil
	}
	return []jobs.Command{{Key: jobs.CommandRetry, Label: "Retry"}}, nil
}

// ExecuteCommand runs one control the host decided this Kind owns. Nothing here
// does: Retry is the control plane's own lineage work, and the common
// dismiss/pin/forget handlers are the Service's. The refusal names the command
// rather than staying silent, so a caller reaching here learns what happened.
func (a *pluginActionAdapter) ExecuteCommand(_ context.Context, execution jobs.CommandExecution) (jobs.CommandOutcome, error) {
	return jobs.CommandOutcome{}, fmt.Errorf("%w: %s", jobs.ErrCommandNotAdvertised, execution.Key)
}

// pluginActionSink publishes one plugin execution's reports into its Job.
//
// It is the adapter's only writer: plugin_system calls these five methods from the
// goroutine running the Lua, and every one of them turns a fact into a control
// plane call. Nothing here decides whether a Job may finish — a stale execution's
// publish is refused by the token fence, and the refusal is swallowed because a
// fence doing its job is not an error to report to a plugin.
type pluginActionSink struct {
	ctx       *MahresourcesContext
	execution jobs.Execution
	input     *pluginActionJobInput
	// pending is the terminal outcome this execution reached and the durable plane
	// has not accepted yet, or nil. It is immutable once stored: a retry publishes
	// the same outcome, never a recomputed one.
	pending atomic.Pointer[pluginActionOutcome]
	// retryOnce starts the one retry goroutine this sink may have.
	retryOnce sync.Once
}

func newPluginActionSink(ctx *MahresourcesContext, execution jobs.Execution, input *pluginActionJobInput) *pluginActionSink {
	return &pluginActionSink{ctx: ctx, execution: execution, input: input}
}

// pluginActionSettlementRetryInterval is how long a retained terminal outcome waits
// before it is offered to the durable plane again. Short, because what it is waiting
// for is a transient write refusal and the Job stays visibly running until it lands.
const pluginActionSettlementRetryInterval = 500 * time.Millisecond

func (s *pluginActionSink) ref() jobs.ExecutionRef {
	return jobs.ExecutionRef{JobID: s.execution.JobID, ExecutionToken: s.execution.ExecutionToken}
}

// Started records that the handler is about to be entered.
func (s *pluginActionSink) Started(message string) {
	s.progress(jobs.Progress{Phase: "running", Message: s.safeText(message, jobs.MaxProgressMessageBytes)})
}

// Progress replaces the bounded progress snapshot. It is not an event: the plugin
// manager throttles these calls, and a Job's timeline is not the place to record
// that a loop was at 43 percent.
//
// The plugin's own text is redacted first, because a progress snapshot is durable
// and searchable in exactly the way a Job's sealed input is not.
func (s *pluginActionSink) Progress(percent int, message string) {
	completed := int64(percent)
	total := int64(100)
	s.progress(jobs.Progress{
		Completed: &completed,
		Total:     &total,
		Unit:      "percent",
		Message:   s.safeText(message, jobs.MaxProgressMessageBytes),
	})
}

// safeText is the one redaction policy for text a plugin supplies: every value of
// this Job's own parameters is replaced before the text is persisted.
//
// It is deliberately about values the host *knows* rather than about guessing at
// what looks secret. The parameters are the plugin's own inputs — the Job's
// summary refuses to carry them, and its envelope is the only durable copy — so a
// string built out of one of them is the concrete leak a redaction rule can
// actually close: `error(ctx.params.token)` reaching the timeline, the progress
// snapshot or a hook payload. Values too short to be worth replacing are left
// alone, and a Job with no parameters has nothing to redact.
func (s *pluginActionSink) safeText(message string, limit int) string {
	return truncateTo(redactPluginParamValues(message, s.input), limit)
}

// sanitizedResult is the result table with every parameter value replaced, at every
// depth.
//
// A result is arbitrary Lua: a table of strings, of tables, of lists — whichever shape
// the handler chose — and the value that matters can sit in any of them, including the
// message the completion carries. Redaction therefore walks the structure rather than
// the top-level strings, and it redacts *keys* as well: a handler can use a value as a
// key (`result[ctx.params.token] = true`), and a key is as durable and as searchable as
// a value.
func (s *pluginActionSink) sanitizedResult(result map[string]any) map[string]any {
	sanitized := make(map[string]any, len(result))
	for key, value := range result {
		sanitized[s.safeText(key, jobs.MaxTitleBytes)] = s.sanitizedValue(value)
	}
	return sanitized
}

// sanitizedValue is sanitizedResult for one value of any shape.
func (s *pluginActionSink) sanitizedValue(value any) any {
	switch typed := value.(type) {
	case string:
		return s.safeText(typed, pluginsMaxSummaryTextBytes)
	case map[string]any:
		return s.sanitizedResult(typed)
	case []any:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, s.sanitizedValue(item))
		}
		return values
	default:
		return value
	}
}

// pluginsMaxSummaryTextBytes bounds one string inside a published result. It is the
// control plane's own text ceiling, so a free-text field cannot make an otherwise
// storable result exceed what an output may hold.
const pluginsMaxSummaryTextBytes = jobs.MaxProgressMessageBytes

// redactPluginParamValues replaces every occurrence of a parameter value in a
// plugin-supplied string. Longest first, so a value that contains another value is
// replaced whole rather than in pieces.
func redactPluginParamValues(message string, input *pluginActionJobInput) string {
	if message == "" || input == nil || len(input.Params) == 0 {
		return message
	}
	values := make([]string, 0, len(input.Params))
	for _, value := range input.Params {
		text, ok := pluginParamText(value)
		if !ok || len(text) < minRedactedParamBytes {
			continue
		}
		values = append(values, text)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, value := range values {
		message = strings.ReplaceAll(message, value, pluginActionRedactionMarker)
	}
	return message
}

// pluginParamText renders one parameter value the way a plugin's own text would
// carry it: a string as itself, and everything else through its JSON form, which
// is how a Lua table comes back.
func pluginParamText(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case nil:
		return "", false
	case bool, float64, int, int64:
		// A scalar that short is caught by the length rule and is not a secret in
		// practice; rendering it would only mangle prose.
		return "", false
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func (s *pluginActionSink) progress(progress jobs.Progress) {
	service := s.ctx.JobService()
	if service == nil {
		return
	}
	if _, err := service.UpdateProgress(s.ctx.jobDeps(), s.ref(), progress); err != nil && !mirrorRefusalIsSilent(err) {
		log.Printf("warning: could not record progress for job %s: %v", s.execution.JobID, err)
	}
}

// Completed records the execution's own success, publishing the plugin's result
// table as a bounded summary output when it is one this Kind can store.
//
// The table is redacted before it is published, exactly as the message is. It is a
// durable, searchable surface: `mah.job_complete(id, {message = ctx.params.token})`
// stores the parameter value in it, and the parameter values are the one secret this
// Kind is holding — the Job's summary refuses to carry them, and the sealed envelope is
// their only other copy. Redacting only the *message* left the same value sitting in the
// structured output beside it.
func (s *pluginActionSink) Completed(message string, result map[string]any) error {
	if s.settled() {
		return nil
	}
	var publication error
	if len(result) > 0 {
		sanitized := s.sanitizedResult(result)
		if reference, err := json.Marshal(sanitized); err == nil && len(reference) <= maxPluginActionResultBytes {
			if _, err := s.execution.Output(jobs.OutputInput{
				Key:       "result",
				Type:      jobs.OutputTypeSummary,
				Label:     "Result",
				Reference: reference,
			}); err != nil && !mirrorRefusalIsSilent(err) {
				log.Printf("warning: could not publish the result of job %s: %v", s.execution.JobID, err)
				publication = err
			}
		} else {
			// Too large to be an output, which is not a failure: the result is an
			// optional document, and its absence must not turn the work that
			// produced it into a failed Job.
			s.warn("result-too-large", "the action's result is too large to store")
		}
	}
	outcome := pluginActionOutcome{succeeded: true, message: s.safeText(message, jobs.MaxProgressMessageBytes)}
	if _, err := s.publishOutcome(outcome); err != nil {
		return s.retainUnsettled(outcome, err)
	}
	s.announceTerminal("completed", "")
	return publication
}

// Failed records an unsuccessful execution.
//
// The failure is a *bounded host-owned classification*, not the plugin's text.
// A handler's message is arbitrary Lua — `error(ctx.params.secret)`, a signed URL
// out of an HTTP error — and a Job's failure is durable, searchable history that
// outlives the sealed envelope the secret was supposed to live in. Every other
// Kind in this tree already does this ("the download did not complete"); this is
// the same rule applied to the one executor whose text comes from a script.
//
// The executor's own words are not lost: they go to the process's own log, which
// is not the Job Center and is not a searchable surface, with the Job id that
// ties them back here.
func (s *pluginActionSink) Failed(message string) error {
	if s.settled() {
		return nil
	}
	if raw := s.safeText(message, jobs.MaxFailureMessageBytes); raw != "" {
		log.Printf("plugin job %s failed: %s", s.execution.JobID, raw)
	}
	outcome := pluginActionOutcome{}
	if _, err := s.publishOutcome(outcome); err != nil {
		return s.retainUnsettled(outcome, err)
	}
	s.announceTerminal("failed", pluginActionFailureMessage)
	return nil
}

// announceTerminal tells the deployment's job-event observer that one plugin Job
// ended, so the three after_job_* hooks fire for plugin background work exactly
// as they do for every other Job.
//
// The record is the download queue's own type rather than a second one: one
// observer, one mapping from an outcome to a hook name, and one payload shape —
// the alternative is a second place the catalogue could drift out of.
//
// An interrupted Job is deliberately not announced. The catalogued events are
// completed, failed and cancelled, and "its runtime vanished" is none of them;
// inventing a fourth name here is exactly the drift the catalogue's own drift
// test exists to prevent. The fact is not lost — it is on the Job's timeline,
// where everything else about it is.
func (s *pluginActionSink) announceTerminal(status, failureMessage string) {
	if s.input != nil && s.input.NoTerminalHook {
		// The work came from the hook feed, so its outcome does not go back into
		// it: announcing here is what would let an after_job_* handler that calls
		// mah.start_job notify itself, forever.
		return
	}
	var owner *uint
	if s.execution.Access.UserID != 0 {
		id := s.execution.Access.UserID
		owner = &id
	}
	s.ctx.announcePluginJobTerminal(s.execution.JobID, status, pluginActionJobName(s.input),
		truncateTo(failureMessage, jobs.MaxFailureMessageBytes), owner)
}

// pluginActionJobName is the human name of one plugin execution, for a hook
// payload and only ever bounded text: the plugin, and which of its things ran.
func pluginActionJobName(input *pluginActionJobInput) string {
	if input == nil {
		return "plugin"
	}
	switch input.Subtype {
	case pluginActionSubtypeScheduled:
		return input.Plugin + ": " + input.ScheduleID
	case pluginActionSubtypeClosure:
		return input.Plugin + ": " + input.Label
	default:
		return input.Plugin + ": " + input.Action
	}
}

// CallbackLost records that the callback this execution was reporting for can
// never run or finish again, which is what a graceful shutdown proves.
//
// It is not the same statement as an expired lease and it is not inferred from
// one: stopping the VM is a fact this process establishes about its own
// *lua.LFunction. The Job ends `interrupted` — unexpected end, continuation
// requires a new Job — rather than failed, because nothing about the work failed;
// it lost its runtime.
func (s *pluginActionSink) CallbackLost(reason string) {
	if s.finished() {
		return
	}
	detail, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return
	}
	service := s.ctx.JobService()
	if service == nil {
		return
	}
	current, err := s.current()
	if err != nil || current.State.Terminal() {
		return
	}
	if _, err := service.Finish(s.ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    s.ref(),
		ExpectedVersion: current.Version,
		Outcome:         jobs.StateInterrupted,
		Event:           jobs.EventInput{Type: jobs.EventInterrupted, Detail: detail},
	}); err != nil && !mirrorRefusalIsSilent(err) {
		log.Printf("warning: could not interrupt job %s: %v", s.execution.JobID, err)
	}
}

// settled reports whether this Job already reached an end state, so a late or
// repeated report writes nothing. The plugin manager's own reporters can fire
// after a handler completed itself, and a second terminal write would either be
// refused or — worse — be a second outcome.
//
// A Job whose row cannot be *read* is deliberately not settled. Reading a failure
// as "somebody already ended it" discarded the outcome the executor had just
// produced: the Job stayed running with its claim, its capacity and no writer left,
// and reconciliation could not classify it. An unknown state is "try to publish",
// and the publish's own read is what decides.
func (s *pluginActionSink) settled() bool {
	snap, err := s.current()
	if err != nil {
		return false
	}
	return snap.State.Terminal()
}

// current reads this execution's Job, with the injected read failure a test uses to
// reach the branch above.
func (s *pluginActionSink) current() (jobs.Snapshot, error) {
	service := s.ctx.JobService()
	if service == nil {
		return jobs.Snapshot{}, errors.New("this context has no job control plane installed")
	}
	if err := s.ctx.jobFaults.settlementRead(); err != nil {
		return jobs.Snapshot{}, err
	}
	return service.Get(s.ctx.jobDeps(), jobs.Access{Administrator: true}, s.execution.JobID)
}

// retainUnsettled keeps one refused terminal outcome and retries it until the durable
// plane has it.
//
// §3's quiescence rule is why the outcome may not simply be dropped: the callback has
// returned, the work is over, and the only thing standing between the Job and its
// classification is a write that can be refused transiently. Dropping it left a Job
// running and heartbeated for ever — its capacity held, invisible to reconciliation,
// and the person waiting told nothing. A refusal that is the *fence* working (the Job
// moved on, somebody else settled it, there is no such Job) is not retained: there is
// nothing left to publish, and retrying would be a second outcome.
func (s *pluginActionSink) retainUnsettled(outcome pluginActionOutcome, err error) error {
	if settleRefused(err) {
		return nil
	}
	log.Printf("warning: could not settle job %s; retaining its outcome to report again: %v",
		s.execution.JobID, err)
	s.pending.Store(&outcome)
	s.retrySettlement()
	return err
}

// retrySettlement republishes one retained outcome until the durable plane has it.
//
// One goroutine per execution, started on the first refusal and ended by the first
// answer that is either durable or final. It is deliberately not driven from the
// heartbeat: a heartbeat that could not reach the database is the same outage that
// refused this write, and an execution whose outcome is waiting must not have its
// retry tied to a loop that ends the moment the Job leaves running.
func (s *pluginActionSink) retrySettlement() {
	s.retryOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(pluginActionSettlementRetryInterval)
			defer ticker.Stop()
			for range ticker.C {
				pending := s.pending.Load()
				if pending == nil {
					return
				}
				if _, err := s.publishOutcome(*pending); settleRefused(err) {
					s.pending.Store(nil)
					return
				}
			}
		}()
	})
}

// pluginActionOutcome is one execution's terminal outcome in its publishable form.
// The success message is carried already redacted: an outcome that is retried is
// published later, and a value that was safe to store once must not depend on the
// retry path redacting it a second time.
type pluginActionOutcome struct {
	succeeded bool
	message   string
}

// publishOutcome records one terminal outcome through the sink's own finish path,
// reporting the refusal when there is one.
func (s *pluginActionSink) publishOutcome(outcome pluginActionOutcome) (jobs.Snapshot, error) {
	if outcome.succeeded {
		return s.finish(jobs.StateSucceeded, nil, outcome.message)
	}
	return s.finish(jobs.StateFailed, &jobs.Failure{
		Code:    pluginActionFailureCode,
		Class:   jobs.FailureClassInternal,
		Message: pluginActionFailureMessage,
	}, pluginActionFailureMessage)
}

// settleRefused reports whether an answer ends this execution's attempts to publish,
// and it is deliberately narrower than "an error": a refusal by the execution fence,
// by the state machine, or by the Job's absence is final — the outcome cannot be
// recorded and retrying it would be a second outcome — while everything else (a
// locked database, a pool that is briefly exhausted, a version that moved under a
// concurrent command) is the write being refused rather than the outcome being wrong.
func settleRefused(err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, jobs.ErrStaleExecution),
		errors.Is(err, jobs.ErrIllegalTransition),
		errors.Is(err, jobs.ErrNotFound):
		return true
	default:
		return false
	}
}

// finished reports whether this Job already reached an end state, so a late or
// repeated report writes nothing. The plugin manager's own reporters can fire
// after a handler completed itself, and a second terminal write would either be
// refused or — worse — be a second outcome.
func (s *pluginActionSink) finished() bool {
	return s.settled()
}

func (s *pluginActionSink) finish(outcome jobs.State, failure *jobs.Failure, message string) (jobs.Snapshot, error) {
	service := s.ctx.JobService()
	if service == nil {
		return jobs.Snapshot{}, nil
	}
	current, err := s.current()
	if err != nil {
		return jobs.Snapshot{}, err
	}
	if err := s.ctx.jobFaults.settlementWrite(); err != nil {
		return jobs.Snapshot{}, err
	}
	if current.State.Terminal() {
		return current, nil
	}
	// The event's *type* stays the derived terminal one — the timeline says
	// "succeeded", which is what a reader filters on — and the plugin's own
	// message rides in its bounded detail.
	event := jobs.EventInput{}
	if outcome == jobs.StateSucceeded && message != "" && message != "Completed" {
		if detail, err := json.Marshal(map[string]string{"message": truncateTo(message, 1500)}); err == nil {
			event = jobs.EventInput{Detail: detail}
		}
	}
	return service.Finish(s.ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    s.ref(),
		ExpectedVersion: current.Version,
		Outcome:         outcome,
		Event:           event,
		Failure:         failure,
	})
}

// warn records a bounded, optional warning on the Job's timeline. It never
// changes the outcome, which is why it is safe for chatter: the control plane
// reserves capacity for lifecycle and terminal events, so an adapter's warnings
// cannot decide whether a Job may finish.
func (s *pluginActionSink) warn(code, message string) {
	detail, err := json.Marshal(map[string]string{"reason": code, "message": truncateTo(message, 500)})
	if err != nil {
		return
	}
	if err := s.execution.Event(jobs.EventInput{Type: jobs.EventWarning, Detail: detail}); err != nil && !mirrorRefusalIsSilent(err) {
		log.Printf("warning: could not record a warning for job %s: %v", s.execution.JobID, err)
	}
}

// pluginActionHostJobs is the host half of plugin_system's HostJobs seam: it is
// what makes a closure-backed mah.start_job durable.
type pluginActionHostJobs struct {
	ctx *MahresourcesContext
}

// StartClosureJob accepts and claims the Job one closure-backed mah.start_job
// stands for.
//
// The Job is accepted *and claimed* in one transaction, before the Lua goroutine
// exists: the work is durable before anything runs — the design's acceptance
// boundary — and it is never visible as waiting work. That second half is what
// makes it correct rather than merely tidy. The callback is a *lua.LFunction in
// this process's VM, so nothing but this process can run the Job, and a Job left
// queued for it is a Job the control plane's dispatch loop is entitled to claim —
// which it does, in the window between acceptance and the host's own claim, only
// to find no callback to run. Accept-and-claim removes that window instead of
// arbitrating it: the Job is born running under this process's token, and another
// runtime has no instant at which it can see the work at all.
//
// The Job is non-replayable: its input is a label and a provenance, not something
// that could be replayed, and recording it as replayable would advertise a Retry
// whose execution could not exist. It records the originating runtime identity, so
// a Job this process leaves running can be reconciled by the next one with proof
// rather than with a guess.
func (h *pluginActionHostJobs) StartClosureJob(request plugin_system.ClosureJobRequest) (*plugin_system.HostJobRef, error) {
	service := h.ctx.JobService()
	if service == nil {
		// No control plane: the plugin manager keeps its in-memory record, which
		// is the behaviour every bare manager has always had.
		return nil, nil
	}
	label := request.Label
	if strings.TrimSpace(label) == "" {
		label = "Plugin background work"
	}

	// Whether this Job announces its own terminal event to the plugin hook feed.
	// A start_job from an after_job_* delivery does not, and neither does one whose
	// parent Job does not: the suppression has to hold down the whole lineage, or
	// a hook that starts work is fed by the completion of the work that work
	// started instead of by its direct child.
	noTerminalHook := request.JobEventDispatch || h.ctx.pluginActionJobIsQuiet(request.ParentJobID)

	handle := download_queue.NewJobID()
	input, err := json.Marshal(pluginActionJobInput{
		Subtype:        pluginActionSubtypeClosure,
		Plugin:         request.PluginName,
		Label:          truncateTo(label, jobs.MaxTitleBytes),
		Runtime:        plugin_system.CurrentRuntimeIdentity().String(),
		NoTerminalHook: noTerminalHook,
	})
	if err != nil {
		return nil, err
	}

	var owner *uint
	if request.ActorUserID != 0 {
		id := request.ActorUserID
		owner = &id
	}
	execution, _, err := service.AcceptClaimed(context.Background(), h.ctx.jobDeps(), jobs.Acceptance{
		Kind:        JobKindPluginAction,
		KindVersion: jobPluginActionKindVersion,
		State:       jobs.StateQueued,
		OwnerUserID: owner,
		ActorUserID: owner,
		Origin:      "plugin",
		Title:       truncateTo(label, jobs.MaxTitleBytes),
		Replay:      jobs.ReplayInput{NonReplayable: true},
		LegacyRefs:  []jobs.LegacyRef{{Namespace: PluginActionHandleNamespace, Handle: handle}},
		// Non-replayable work stores no envelope, so there is no input for the
		// Kind's sanitizer to run on at acceptance: the summary is derived here,
		// through the same codec, so it is still one description of one Job and
		// still carries no value the plugin supplied.
		Summary: pluginActionSummaryOf(input),
	}, jobs.ClaimRequest{
		Kind:        JobKindPluginAction,
		KindVersion: jobPluginActionKindVersion,
		Claimant:    plugin_system.CurrentRuntimeIdentity().String(),
		Capacity:    h.ctx.hostClaimCapacityBudget(),
	})
	if err != nil {
		return nil, fmt.Errorf("accept the plugin job: %w", err)
	}
	h.ctx.startPluginActionHeartbeat(execution)

	if request.ParentJobID != "" {
		// A child link is how the Job Center shows what started what. It is a
		// best-effort relation: a parent that cannot be linked — it was
		// reconciled away, or it is not visible to this handle — must not stop
		// the work the plugin asked for.
		if err := service.Link(h.ctx.jobDeps(), jobs.LinkRequest{
			Type: jobs.LinkParentChild, FromJobID: request.ParentJobID, ToJobID: execution.JobID,
		}); err != nil {
			log.Printf("warning: could not link plugin job %s to its parent %s: %v",
				execution.JobID, request.ParentJobID, err)
		}
	}

	return &plugin_system.HostJobRef{
		JobID:       execution.JobID,
		Handle:      handle,
		ParentJobID: request.ParentJobID,
		Sink: newPluginActionSink(h.ctx, execution, &pluginActionJobInput{
			Subtype:        pluginActionSubtypeClosure,
			Plugin:         request.PluginName,
			NoTerminalHook: noTerminalHook,
		}),
	}, nil
}

// runScheduledOccurrenceJob materializes one occurrence of a schedule as a
// durable Job and runs it here, in the caller's goroutine.
//
// It is the scheduler's door, and it is deliberately synchronous: the scheduler
// holds the schedule row's claim for the whole run under the "skip" policy, and
// what that claim is protecting is that one occurrence is in flight at a time.
// Handing the occurrence to the control plane's dispatch loop instead would make
// the row's claim and the execution's lifetime two different things — the claim
// would be released while the work it protects was still queued — so the Job is
// claimed by the process that is about to run it, and the heartbeat keeps it.
//
// Started=false means the handler was never entered: the Job was withdrawn, the
// row keeps its claim (the caller releases it), and no outcome is recorded. That
// is the same answer the inline path gives for a full budget or a busy VM, and it
// is what errJobDidNotStart exists to distinguish from a failed run.
func (ctx *MahresourcesContext) runScheduledOccurrenceJob(reg plugin_system.ScheduleRegistration, actorUserID uint, overlap string, wait time.Duration) (pluginActionRun, error) {
	service := ctx.JobService()
	if service == nil {
		return pluginActionRun{}, errors.New("this context has no job control plane installed")
	}

	label := fmt.Sprintf("%s: %s", reg.PluginName, reg.ScheduleID)
	input, err := json.Marshal(pluginActionJobInput{
		Subtype:    pluginActionSubtypeScheduled,
		Plugin:     reg.PluginName,
		ScheduleID: reg.ScheduleID,
		Label:      truncateTo(label, jobs.MaxTitleBytes),
		Overlap:    overlap,
		Runtime:    plugin_system.CurrentRuntimeIdentity().String(),
	})
	if err != nil {
		return pluginActionRun{}, err
	}

	var owner *uint
	if actorUserID != 0 {
		id := actorUserID
		owner = &id
	}
	accepted, err := service.Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind:        JobKindPluginAction,
		KindVersion: jobPluginActionKindVersion,
		State:       jobs.StateQueued,
		OwnerUserID: owner,
		ActorUserID: owner,
		// The origin is the Schedule, not a person: §1's provenance list has an
		// entry for it precisely because "a plugin asked on a timer" is not the
		// same fact as "somebody clicked a button".
		Origin: "schedule",
		Title:  truncateTo(label, jobs.MaxTitleBytes),
		Replay: jobs.ReplayInput{Input: input},
	})
	if err != nil {
		return pluginActionRun{}, err
	}

	claim, err := ctx.claimPluginActionJobForHost(accepted.ID)
	if err != nil {
		return pluginActionRun{JobID: accepted.ID}, err
	}
	if claim.Elsewhere {
		// The dispatch loop claimed this occurrence in the window between its
		// acceptance and this call. It will run — with the same input, through
		// the same adapter — so this waits for the outcome it produces rather
		// than running a second copy of one tick.
		return ctx.awaitPluginActionRun(context.Background(), jobs.Execution{JobID: accepted.ID})
	}
	if !claim.Owned {
		if err := ctx.withdrawPluginActionJob(jobs.Execution{JobID: accepted.ID},
			"not-started", "the occurrence could not be claimed"); err != nil {
			log.Printf("warning: could not withdraw the unclaimed occurrence %s: %v", accepted.ID, err)
		}
		return pluginActionRun{JobID: accepted.ID}, nil
	}

	pm := ctx.PluginManager()
	if pm == nil {
		if err := ctx.blockPluginActionJob(claim.execution, "plugins-unavailable"); err != nil {
			return pluginActionRun{JobID: accepted.ID}, err
		}
		return pluginActionRun{JobID: accepted.ID}, nil
	}
	inputDecoded, err := pluginActionInputOf(input)
	if err != nil {
		return pluginActionRun{JobID: accepted.ID}, err
	}
	return ctx.runScheduledPluginOccurrence(pm, claim.execution, inputDecoded, wait)
}

// claimPluginActionJob claims one already-accepted Job for this process and
// starts the heartbeat that keeps its claim alive.
//
// Claiming by id is what makes a host-side execution a real one: a Job owned by
// nobody has no lease to reconcile, no token to fence a stale publish with, and no
// release anybody frees. The heartbeat is what keeps a five-minute Lua call from
// outliving its own claim.
func (ctx *MahresourcesContext) claimPluginActionJob(jobID string) (jobs.Execution, bool, error) {
	service := ctx.JobService()
	if service == nil {
		return jobs.Execution{}, false, errors.New("this context has no job control plane installed")
	}
	execution, claimed, err := service.Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind:        JobKindPluginAction,
		KindVersion: jobPluginActionKindVersion,
		JobID:       jobID,
		Claimant:    plugin_system.CurrentRuntimeIdentity().String(),
		// The deployment-wide budget, exactly as the dispatch loop asks for it: a
		// plugin execution is one of the executions `max-job-concurrency` counts,
		// and a claim that skipped it would let a submitting process run work the
		// deployment's own budget had refused.
		Capacity: ctx.hostClaimCapacityBudget(),
	})
	if err != nil || !claimed {
		return execution, claimed, err
	}
	ctx.startPluginActionHeartbeat(execution)
	return execution, true, nil
}

// pluginActionClaim is the answer to "who is going to run this Job?" for a host
// that has just accepted it.
type pluginActionClaim struct {
	// Execution is the claim this process took, valid only when Owned.
	execution jobs.Execution
	// Owned reports that this process claimed the Job and must run it now.
	Owned bool
	// Elsewhere reports that another runtime of this deployment claimed it in the
	// window between acceptance and this claim. The work will run — the dispatch
	// loop is looking at the same queue — so the caller waits for it rather than
	// withdrawing work somebody else owns.
	Elsewhere bool
}

// claimPluginActionJobForHost claims one freshly accepted Job for a host-side
// execution and says who owns it.
//
// The window this exists for is real rather than theoretical: the control plane's
// dispatch loop claims waiting Jobs of every registered Kind, so a Job accepted a
// microsecond ago can be claimed by it before the host that accepted it asks.
// That is not a failure — one of the two wins and the other must not run the work
// a second time — so the loser is told which of the three situations it is in.
func (ctx *MahresourcesContext) claimPluginActionJobForHost(jobID string) (pluginActionClaim, error) {
	service := ctx.JobService()
	if service == nil {
		return pluginActionClaim{}, errors.New("this context has no job control plane installed")
	}
	execution, claimed, err := ctx.claimPluginActionJob(jobID)
	if err != nil {
		return pluginActionClaim{}, err
	}
	if claimed {
		return pluginActionClaim{execution: execution, Owned: true}, nil
	}
	snap, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return pluginActionClaim{}, err
	}
	if snap.State == jobs.StateRunning || snap.State.Terminal() {
		return pluginActionClaim{Elsewhere: true}, nil
	}
	return pluginActionClaim{}, nil
}

// startPluginActionHeartbeat keeps one host-side execution's claim alive for as
// long as it runs, and is what the control plane's own runtime does for the
// executions it dispatches.
//
// It stops when the Job leaves running — the heartbeat is refused by the token
// fence once the Job is finished or released — so nothing has to tell it to stop.
func (ctx *MahresourcesContext) startPluginActionHeartbeat(execution jobs.Execution) {
	service := ctx.JobService()
	if service == nil || execution.ExecutionToken == "" {
		return
	}
	lease := ctx.pluginActionLease()
	interval := lease / jobRuntimeHeartbeatDivisor
	if interval < minJobRuntimeHeartbeatInterval {
		interval = minJobRuntimeHeartbeatInterval
	}
	ref := jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			err := service.Heartbeat(ctx.jobDeps(), ref, lease)
			if err == nil {
				continue
			}
			if errors.Is(err, jobs.ErrStaleExecution) {
				// The Job is no longer this execution's: it finished, or it was
				// reconciled away. Either way there is nothing left to renew.
				return
			}
			log.Printf("warning: heartbeat for plugin job %s failed: %v", execution.JobID, err)
		}
	}()
}

// pluginActionLease is the definition's own lease, so a heartbeat renews exactly
// what the claim took.
func (ctx *MahresourcesContext) pluginActionLease() time.Duration {
	adapter := pluginActionAdapter{ctx: ctx}
	return adapter.Definition().EffectiveLease()
}

// pluginActionOwner is the user a plugin execution is attributed to: the actor
// the Job acts as, because the panel and the legacy listing both filter on the
// submitter and a retry or an occurrence runs as whoever the Job records.
func (ctx *MahresourcesContext) pluginActionOwner(execution jobs.Execution) *uint {
	if execution.Access.UserID == 0 {
		return nil
	}
	owner := execution.Access.UserID
	return &owner
}

// blockPluginActionJob records that one plugin-action Job cannot run and what
// has to change. The reason is a bounded code rather than an error's text, for
// the reason the queue bridge gives: only the Kind knows what in its own error is
// safe, and this lands on a timeline a reader searches.
func (ctx *MahresourcesContext) blockPluginActionJob(execution jobs.Execution, reason string) error {
	service := ctx.JobService()
	if service == nil {
		return nil
	}
	current, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, execution.JobID)
	if err != nil {
		return err
	}
	if current.State.Terminal() || current.State == jobs.StateBlocked {
		return nil
	}
	detail, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return err
	}
	_, err = service.Transition(ctx.jobDeps(), jobs.Transition{
		JobID:           execution.JobID,
		ExpectedVersion: current.Version,
		ExecutionToken:  execution.ExecutionToken,
		To:              jobs.StateBlocked,
		Event:           jobs.EventInput{Type: jobs.EventBlocked, Detail: detail},
	})
	if err != nil && mirrorRefusalIsSilent(err) {
		return nil
	}
	return err
}

// failPluginActionJob ends one plugin-action Job whose execution could not begin,
// with a bounded failure the control plane can aggregate on.
func (ctx *MahresourcesContext) failPluginActionJob(execution jobs.Execution, code, message string) error {
	service := ctx.JobService()
	if service == nil {
		return nil
	}
	current, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, execution.JobID)
	if err != nil {
		return err
	}
	if current.State.Terminal() {
		return nil
	}
	if _, err := service.Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: current.Version,
		Outcome:         jobs.StateFailed,
		Failure:         &jobs.Failure{Code: code, Class: jobs.FailureClassDependency, Message: message},
	}); err != nil && !mirrorRefusalIsSilent(err) {
		return err
	}
	return nil
}

// withdrawPluginActionJob ends one materialized execution that never began.
//
// It is `cancelled` rather than `failed` on purpose: nothing the plugin did
// failed, the tick could not get a slot or the VM, and recording a failure would
// put an accusation on the plugin's timeline for work it never entered. The phase
// and the event name the reason, which is what lets a caller — the scheduler —
// tell "not this time" from "ran and ended".
func (ctx *MahresourcesContext) withdrawPluginActionJob(execution jobs.Execution, phase, reason string) error {
	service := ctx.JobService()
	if service == nil {
		return nil
	}
	current, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, execution.JobID)
	if err != nil {
		return err
	}
	if current.State.Terminal() {
		return nil
	}
	// The phase is set on the progress snapshot before the outcome, because it is
	// what a caller reads to tell "not this time" from "ran and ended" — the
	// event names the reason and the state is cancelled either way.
	if _, err := service.UpdateProgress(ctx.jobDeps(), jobs.ExecutionRef{
		JobID: execution.JobID, ExecutionToken: execution.ExecutionToken,
	}, jobs.Progress{Phase: phase, Message: truncateTo(reason, jobs.MaxProgressMessageBytes)}); err != nil && !mirrorRefusalIsSilent(err) {
		return err
	}
	detail, err := json.Marshal(map[string]string{"reason": phase, "message": truncateTo(reason, 500)})
	if err != nil {
		return err
	}
	_, err = service.Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: current.Version,
		Outcome:         jobs.StateCancelled,
		Event:           jobs.EventInput{Type: "not-started", Detail: detail},
	})
	if err != nil && mirrorRefusalIsSilent(err) {
		return nil
	}
	return err
}

// pluginActionHandleFor answers the legacy handle one Job carries, so the
// in-memory panel entry and the durable Job answer to the same id.
func (ctx *MahresourcesContext) pluginActionHandleFor(jobID string) string {
	handle, err := ctx.jobHandleFor(jobID, PluginActionHandleNamespace)
	if err != nil {
		log.Printf("warning: could not read the legacy id of plugin job %s: %v", jobID, err)
		return ""
	}
	return handle
}

// pluginScheduleRegistration finds one live schedule registration by name.
func (ctx *MahresourcesContext) pluginScheduleRegistration(pm *plugin_system.PluginManager, pluginName, scheduleID string) (plugin_system.ScheduleRegistration, bool) {
	for _, candidate := range pm.DeclaredSchedules(pluginName) {
		if candidate.ScheduleID == scheduleID {
			return candidate, true
		}
	}
	return plugin_system.ScheduleRegistration{}, false
}

// pluginActionSummaryOf derives the sanitized summary from the already-validated
// input, through the codec, so the summary a listing searches and the input an
// execution opens are one description of one Job.
func pluginActionSummaryOf(input json.RawMessage) json.RawMessage {
	sanitized, err := pluginActionJobCodec().Sanitize(input)
	if err != nil {
		return nil
	}
	return sanitized
}

// pluginActionRuntimeOf reads the recorded runtime identity out of a sanitized
// summary. An unreadable summary yields an empty identity, which proves nothing.
func pluginActionRuntimeOf(summary json.RawMessage) string {
	if len(summary) == 0 {
		return ""
	}
	var decoded pluginActionSummary
	if err := json.Unmarshal(summary, &decoded); err != nil {
		return ""
	}
	return decoded.Runtime
}

// pluginActionJobIsQuiet reports whether an already-accepted Job carries the
// causal-suppression flag, read from its sanitized summary so no envelope has to
// be opened for it. A Job that cannot be read is not quiet: suppression has to be
// proved, and a Job nobody can read is one this process cannot claim to have
// covered.
func (ctx *MahresourcesContext) pluginActionJobIsQuiet(jobID string) bool {
	if ctx == nil || jobID == "" {
		return false
	}
	service := ctx.JobService()
	if service == nil {
		return false
	}
	snap, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return false
	}
	return pluginActionJobIsQuietOf(snap.Summary)
}

// pluginActionJobIsQuietOf reads the causal-suppression flag out of a summary.
func pluginActionJobIsQuietOf(summary json.RawMessage) bool {
	if len(summary) == 0 {
		return false
	}
	var decoded pluginActionSummary
	if err := json.Unmarshal(summary, &decoded); err != nil {
		return false
	}
	return decoded.NoTerminalHook
}

// pluginActionSubtypeOf reads the subtype out of a sanitized summary. An
// unreadable summary yields an empty subtype, which matches no executor.
func pluginActionSubtypeOf(summary json.RawMessage) string {
	decoded, ok := pluginActionSummaryDecoded(summary)
	if !ok {
		return ""
	}
	return decoded.Subtype
}

// pluginActionSummaryDecoded reads a sanitized summary whole. An unreadable or
// absent summary is reported as not-decoded rather than as a zero value, so a
// caller cannot mistake "nothing to read" for "read, and it named no plugin".
func pluginActionSummaryDecoded(summary json.RawMessage) (pluginActionSummary, bool) {
	if len(summary) == 0 {
		return pluginActionSummary{}, false
	}
	var decoded pluginActionSummary
	if err := json.Unmarshal(summary, &decoded); err != nil {
		return pluginActionSummary{}, false
	}
	return decoded, true
}

// accessUserID reads the acting user id out of an execution's access. A zero id
// is the host itself, which is the identity every actorless execution runs as.
func accessUserID(access jobs.Access) uint {
	return access.UserID
}

// actionJobTerminal reports whether one in-memory plugin execution has finished.
// The statuses are plugin_system's own spellings, which the compatibility
// projection keeps.
func actionJobTerminal(status string) bool {
	switch status {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

// truncateTo keeps a plugin-supplied string inside one bounded column, cutting on
// a byte boundary and marking the cut so a reader is not misled about where the
// text ended.
func truncateTo(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	const marker = "... (truncated)"
	if limit <= len(marker) {
		return value[:limit]
	}
	return value[:limit-len(marker)] + marker
}

// ProjectActionJob answers one plugin-action Job's legacy row from the durable record.
//
// The route that answers a plugin action by handle reads the plugin manager's registry,
// which is this process's memory: it holds the executions this process started and is
// empty after a restart. A submission accepted for a runtime with no capacity starts no
// execution anywhere, so that lookup found nothing and the client was told 404 for work
// the server itself had just named and answered an id for. The durable Job is the thing
// that outlives all of that, and this is the projection of it into the row shape the
// legacy route serializes.
//
// Visibility is the canonical read's: a handle that resolves to a Job this principal may
// not see is answered exactly as a handle that names nothing, which is what the handler
// already does with the owner it gets back.
func (ctx *MahresourcesContext) ProjectActionJob(handle string) (*plugin_system.ActionJob, error) {
	projected, err := ctx.ResolveJobHandle(PluginActionHandleNamespace, handle)
	if err != nil {
		return nil, err
	}
	if projected.Kind != JobKindPluginAction {
		return nil, fmt.Errorf("%w: %s", jobs.ErrNotFound, handle)
	}
	summary, _ := pluginActionSummaryDecoded(projected.Summary)

	progress := projected.Progress
	message := progress.Message
	percent := 0
	if progress.Total != nil && *progress.Total > 0 && progress.Completed != nil {
		percent = int(*progress.Completed * 100 / *progress.Total)
	}
	if projected.Failure != nil && message == "" {
		message = projected.Failure.Message
	}
	if message == "" {
		message = projected.Phase
	}

	entityType := summary.EntityType
	if entityType == "" && summary.Subtype == pluginActionSubtypeScheduled {
		entityType = "custom"
	}
	actionID := summary.Action
	if summary.Subtype == pluginActionSubtypeScheduled {
		actionID = "schedule:" + summary.ScheduleID
	}
	label := projected.Title
	if label == "" {
		label = pluginActionJobName(&pluginActionJobInput{
			Subtype: summary.Subtype, Plugin: summary.Plugin,
			Action: summary.Action, ScheduleID: summary.ScheduleID,
		})
	}

	return plugin_system.ProjectedActionJob{
		Handle:     handle,
		Plugin:     summary.Plugin,
		ActionID:   actionID,
		Label:      label,
		EntityType: entityType,
		Status:     actionJobStatusFromState(projected.State),
		Progress:   percent,
		Message:    truncateTo(message, jobs.MaxProgressMessageBytes),
		Owner:      projected.OwnerUserID,
		CreatedAt:  projected.AcceptedAt,
	}.ActionJob(), nil
}

// actionJobStatusFromState maps a normalized Job state onto the panel's own plugin-job
// vocabulary, which is what every legacy consumer switches on.
func actionJobStatusFromState(state jobs.State) string {
	switch state {
	case jobs.StateSucceeded:
		return "completed"
	case jobs.StateCancelled:
		return "cancelled"
	case jobs.StateFailed, jobs.StateInterrupted:
		return "failed"
	case jobs.StateRunning:
		return "running"
	case jobs.StatePaused, jobs.StateBlocked:
		return "paused"
	default:
		return "pending"
	}
}
