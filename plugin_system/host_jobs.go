package plugin_system

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"mahresources/models/jobmetrics"
	"mahresources/plugin_commands"
)

// This file is the seam between plugin background work and the host's durable Job
// control plane.
//
// Plugin work has always kept its own lifecycle in memory (ActionJob): the panel
// reads it, the SSE stream announces it, and it is gone at the next restart. The
// Job Center requires the same work to be a durable Job instead, with the in-memory
// entry as a projection for the compatibility window — so plugin_system needs a way
// to report into a Job without knowing what one is. It may not import the jobs
// module: attributes of the control plane (states, transitions, tokens, retention)
// have no business inside a Lua host.
//
// So the seam is a sink rather than an executor. The host accepts the Job and hands
// back a HostJobRef; plugin_system reports facts through it and owns no lifecycle.
// The Job Service owns identity, state and the fencing token, and the adapter in
// application_context is what translates these five calls into transitions.
//
// The one thing plugin_system does own here is *whether the callback still
// exists*, because only it knows: a closure-backed mah.start_job carries a
// *lua.LFunction that lives in this process's VM. That is why the runtime identity
// below is part of this file rather than of the adapter — the proof that an
// execution can never be finished is a fact about a process, and this is the
// process that owns plugin VMs.

// HostJobSink is the durable Job one async plugin execution reports through.
//
// Every method is a fact, never a decision: whether a Job may finish, whether an
// outcome is acceptable, and what a stale execution's publish does are the control
// plane's answers. A sink is therefore safe to call from the goroutine that runs
// the Lua, and it must not block that goroutine for long: the method calls are
// where a worker's time goes if it is slow, and the reports are throttled by the
// caller rather than here.
//
// The two *terminal* reports answer with an error, and that answer is what
// plugin_system acts on. A terminal outcome is reported exactly once, and "exactly
// once" has to mean "once the durable plane has it": the host's publish reaches a
// database, so it can be refused transiently, and a caller that treated a refusal as
// a delivery would leave a returned callback's Job running and heartbeated forever —
// holding the deployment's capacity, unclassifiable by reconciliation, and hiding the
// work from the person waiting for it. A non-nil answer means "not durable yet"; the
// caller keeps the outcome and reports it again.
// HostProgress is one progress report a plugin makes: always a percent, and,
// when the plugin counts something, the count, its total and its unit, which
// is what gives the Job a speed and an ETA. Metrics are the named figures it
// reports beside that, already checked against jobmetrics.Validate.
type HostProgress struct {
	Percent   int
	Completed *int64
	Total     *int64
	Unit      string
	Message   string
	Metrics   []jobmetrics.Metric
}

type HostJobSink interface {
	// Started reports that the handler is about to be entered.
	Started(message string)
	// Progress replaces the Job's bounded progress snapshot. It is not an event:
	// the caller throttles it, so a plugin that reports every percent of a long
	// loop does not write a timeline.
	Progress(progress HostProgress)
	// Completed reports the action's own success, with its result table when the
	// plugin returned one. A non-nil answer means the outcome was not durably
	// recorded.
	Completed(message string, result map[string]any) error
	// Failed reports that the handler ended unsuccessfully, with the message the
	// plugin or the host produced. A non-nil answer means the same as Completed's.
	Failed(message string) error
	// CallbackLost reports that the callback this execution was reporting for can
	// never run or finish again: the VM that owned it is gone. It is how a
	// graceful shutdown proves runtime loss, which is a stronger statement than a
	// lease expiring and is why it is a method rather than an inference.
	//
	// It answers with nothing because it is reported at shutdown: there is no
	// second attempt to make, and the next process reconciles what it could not
	// settle.
	CallbackLost(reason string)
}

// HostJobRef names one durable Job an async plugin execution reports through.
//
// JobID is the canonical identity the control plane knows the execution by, and
// Handle is the short id the jobs panel and the legacy action-job endpoint answer
// with. They are different strings on purpose: the handle is a name a client
// already holds, the id is an identity the control plane owns.
type HostJobRef struct {
	JobID string
	// Handle is what mah.start_job returns to Lua and what GET
	// /v1/jobs/action/job answers with. Empty leaves the in-memory id as the only
	// name the work has.
	Handle string
	// ParentJobID names the Job whose execution started this one, for the
	// parent-child lineage a nested mah.start_job records. Empty when the closure
	// was started outside any Job.
	ParentJobID string
	Sink        HostJobSink
}

// HostJobs is the host's half of plugin background work: it accepts the durable
// Job and answers the reference the execution reports through.
//
// Declared here and implemented in application_context, the same shape
// HistoryRecorder and JobEventSink already have in download_queue: the layer that
// owns the database owns the durable record, and this package reaches it through
// one interface it can be tested against.
type HostJobs interface {
	// StartClosureJob accepts a non-restorable Job for a closure-backed
	// mah.start_job and returns the reference its goroutine reports through, or an
	// error when no durable Job could be accepted.
	//
	// It is asked *before* the closure runs, because acceptance is the promise
	// that the work is durable; a plugin whose host cannot accept the Job is told
	// so rather than handed a job id for work nobody can find later.
	StartClosureJob(request ClosureJobRequest) (*HostJobRef, error)
}

// ClosureJobRequest is what one closure-backed mah.start_job asks the host to
// accept.
type ClosureJobRequest struct {
	// PluginName is the plugin the closure belongs to.
	PluginName string
	// Label is the human name the plugin gave the work.
	Label string
	// ActorUserID is the user the call is attributed to, or 0 when there is none.
	ActorUserID uint
	// ParentJobID names the Job whose execution asked for this one. Empty when the
	// call is not executing inside a Job.
	ParentJobID string
	// JobEventDispatch reports that this call is the delivery of a terminal job
	// event — an after_job_completed, after_job_failed or after_job_cancelled
	// hook. A Job accepted from there must not announce its own terminal event,
	// or the feed hands the completion straight back to the hook that caused it,
	// which starts another Job, forever. It is a fact about the call chain rather
	// than about the plugin, so the host is told it rather than inferring it.
	JobEventDispatch bool
}

// SetHostJobs installs the host's Job control plane.
//
// It is a setter rather than a constructor argument for the reason SetKVStore and
// SetPolicyResolver are: the plugin manager is built while the application context
// is still assembling itself, so a closure passed through the constructor would
// capture a nil context. Optional: with no host Jobs installed, async work keeps
// its in-memory lifecycle exactly as it always had.
func (pm *PluginManager) SetHostJobs(h HostJobs) {
	if pm == nil {
		return
	}
	pm.hostJobsMu.Lock()
	pm.hostJobs = h
	pm.hostJobsMu.Unlock()
}

// hostJobsInstalled returns the installed host, or nil.
func (pm *PluginManager) hostJobsInstalled() HostJobs {
	if pm == nil {
		return nil
	}
	pm.hostJobsMu.RLock()
	defer pm.hostJobsMu.RUnlock()
	return pm.hostJobs
}

// RuntimeIdentity names the process that accepted non-restorable plugin work.
//
// It is recorded with the work because a closure callback cannot be restored: the
// *lua.LFunction belongs to one *lua.LState in one process, so "may this be run
// again?" is answered by "is that process still there?" and by nothing else. A
// lease expiring is not that answer — a runtime can be alive and unreachable, and
// a fresh process can be running beside it.
type RuntimeIdentity struct {
	Host        string
	BootSession string
	PID         int
}

// CurrentRuntimeIdentity names this process.
//
// The boot session is what makes a pid meaningful: pids are reused, and "pid 4123
// exists" says nothing about whether it is the process that accepted the work
// unless the machine has not rebooted since. It reuses plugin_commands' own
// platform primitive rather than reading the fact a second way, because two
// implementations of "which boot is this?" are two answers an operator would have
// to reconcile. An unavailable boot session is recorded as empty and makes every
// liveness answer Unknown, which is the fail-safe reading: a Job whose runtime
// cannot be inspected stays blocked instead of being interrupted by mistake.
func CurrentRuntimeIdentity() RuntimeIdentity {
	host, err := os.Hostname()
	if err != nil {
		host = ""
	}
	boot, err := plugin_commands.CurrentBootSessionID()
	if err != nil {
		boot = ""
	}
	return RuntimeIdentity{Host: host, BootSession: boot, PID: os.Getpid()}
}

// String is the recorded form: host, boot session and pid in one bounded field.
//
// It is stored in a Job's sanitized summary — a place a person reads — so it is
// deliberately one compact string rather than nested JSON, and it carries nothing
// but identity: no path, no user, no plugin value.
func (r RuntimeIdentity) String() string {
	return fmt.Sprintf("%s/%s/%d", r.Host, r.BootSession, r.PID)
}

// ParseRuntimeIdentity reads a recorded identity back. An unreadable value yields
// ok=false rather than a zero identity, so a caller cannot mistake "nothing was
// recorded" for "recorded, and this process is it".
func ParseRuntimeIdentity(recorded string) (RuntimeIdentity, bool) {
	parts := strings.Split(recorded, "/")
	if len(parts) != 3 {
		return RuntimeIdentity{}, false
	}
	pid, err := strconv.Atoi(parts[2])
	if err != nil || pid <= 0 {
		return RuntimeIdentity{}, false
	}
	if parts[0] == "" {
		return RuntimeIdentity{}, false
	}
	return RuntimeIdentity{Host: parts[0], BootSession: parts[1], PID: pid}, true
}

// RuntimeLiveness is what can be proved about the process that accepted work.
type RuntimeLiveness int

const (
	// RuntimeUnknown means nothing could be proved: a different host, or a host
	// whose process table cannot be inspected. Work whose runtime is unknown
	// stays blocked; it is never interrupted and never redispatched.
	RuntimeUnknown RuntimeLiveness = iota
	// RuntimeAlive means a process with that pid exists on this host in this boot
	// session. It may be a reused pid, so this is not proof of ownership either —
	// it is proof that the work is not provably finished.
	RuntimeAlive
	// RuntimeGone means the runtime cannot still be there: this host has booted
	// since, or the process does not exist.
	RuntimeGone
)

// Liveness answers what can be proved about the process a recorded identity names.
//
// The rules, and why each one is the conservative reading:
//
//   - another host: Unknown. Its process table is not ours to read, and a boot
//     session id from another machine's clock is not comparable. This is the case
//     §3 calls out — a cross-host mismatch alone never proves death.
//   - this host, a different boot session: Gone. The machine has rebooted since,
//     so no process from that boot exists, whatever its pid does now.
//   - this host, this boot, our own pid: Alive, and provably *this* runtime.
//   - this host, this boot, another pid: Gone when the process does not exist,
//     Alive when one does. A reused pid reads as Alive, which errs toward
//     leaving a Job blocked rather than interrupting work that may still run.
//   - no boot session recorded: Unknown. Without it a pid says nothing across a
//     reboot, and this is the platform where the primitive is unavailable.
func (r RuntimeIdentity) Liveness() RuntimeLiveness {
	if r.Host == "" || r.BootSession == "" {
		return RuntimeUnknown
	}
	current := CurrentRuntimeIdentity()
	if r.Host != current.Host {
		return RuntimeUnknown
	}
	if r.BootSession != current.BootSession {
		return RuntimeGone
	}
	if r.PID == current.PID {
		return RuntimeAlive
	}
	return pidLiveness(r.PID)
}

// ProjectedActionJob is the in-memory shape of one durable plugin-action Job, for a
// reader that asks about work this process does not hold.
//
// The plugin manager's registry is process memory: it is populated by the executions
// this process started and emptied at every restart. The durable Job outlives both, and
// the legacy route that answers one action job by handle has to as well — otherwise a
// client polling the id the server answered with gets a 404 for work that is plainly
// still going to run, and the panel loses a row it was told about.
type ProjectedActionJob struct {
	Handle         string
	CanonicalJobID string
	Plugin         string
	ActionID       string
	Label          string
	// EntityType is the plugin action's entity kind, or "custom" for a schedule.
	EntityType string
	// Status is the panel's own vocabulary: pending, running, paused, completed,
	// failed or cancelled.
	Status   string
	Progress int
	Message  string
	// Owner is the account the Job belongs to, for the per-user visibility rule the
	// route applies. Nil is an ownerless Job, which is admin-only.
	Owner     *uint
	CreatedAt time.Time
}

// ActionJob renders the projection as the entry the route serializes.
func (p ProjectedActionJob) ActionJob() *ActionJob {
	return &ActionJob{
		ID:             p.Handle,
		CanonicalJobID: p.CanonicalJobID,
		Source:         "plugin",
		PluginName:     p.Plugin,
		ActionID:       p.ActionID,
		Label:          p.Label,
		EntityType:     p.EntityType,
		Status:         p.Status,
		Progress:       p.Progress,
		Message:        p.Message,
		CreatedAt:      p.CreatedAt,
		ownerUserID:    p.Owner,
	}
}
