package plugin_system

import (
	"crypto/rand"
	"encoding/base64"
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
	// loop does not write a timeline. A non-nil answer means the snapshot was
	// not recorded and is worth sending again; a fence's refusal is nil.
	Progress(progress HostProgress) error
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
	// Admission, when set, is the claim this execution must be granted before it
	// starts: the host accepted the Job as waiting and has not claimed it, so the
	// execution asks from the head of its plugin's lane. Nil means the host
	// already claimed the Job and Sink is live from the start.
	Admission HostAdmission
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
//
// A pid means something only in one process table, and the fields together say
// which: the boot session names one run of one kernel (a hostname does not name
// a machine; clones and containers share them), and the pid namespace names one
// table inside it (containers on one kernel each have their own).
type RuntimeIdentity struct {
	// Host is the hostname, cut to MaxRuntimeHostBytes. It is for a reader; the
	// boot session and pid namespace are what place a pid.
	Host        string
	BootSession string
	PID         int
	// Nonce is drawn once per process. A restarted container keeps its hostname,
	// the kernel's boot session and usually its pid, so without it a new process
	// reads its predecessor's record as its own and that predecessor never ends.
	// Empty in a record written before nonces were recorded, which proves
	// nothing about a pid this process now holds.
	Nonce string
	// PIDNamespace is the pid namespace's inode on Linux, empty elsewhere. An
	// inode is unique among live namespaces and reused only after its namespace
	// has died, so an equal one is either this table or one whose processes are
	// all gone; a different one is a table this process cannot read.
	PIDNamespace string
}

// hasPIDNamespaces and currentPIDNamespace are the platform's answers, held in
// variables so a test can stand in for a Linux process that cannot read its
// namespace.
var (
	hasPIDNamespaces    = platformHasPIDNamespaces
	currentPIDNamespace = platformPIDNamespace
)

// MaxRuntimeHostBytes bounds the recorded hostname so the whole identity fits a
// Job claimant (jobs.MaxClaimantBytes, 120 bytes) with a 36-byte boot session,
// a 7-digit pid, the 22-character nonce and a 10-digit pid namespace inode.
const MaxRuntimeHostBytes = 40

// runtimeHost cuts a hostname to the recorded bound. Two hosts that differ only
// past it compare equal and are then told apart by their boot session.
func runtimeHost(name string) string {
	if len(name) > MaxRuntimeHostBytes {
		return name[:MaxRuntimeHostBytes]
	}
	return name
}

// processNonce is this process's nonce: 128 random bits, base64url so it holds
// no "/". A predecessor that held this pid in this pid namespace carries
// another, and two that match by chance would read a dead process as this one,
// so the size is what makes that a residual rather than a case.
var processNonce = func() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	return base64.RawURLEncoding.EncodeToString(raw[:])
}()

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
	return RuntimeIdentity{Host: runtimeHost(host), BootSession: boot, PID: os.Getpid(),
		Nonce: processNonce, PIDNamespace: currentPIDNamespace()}
}

// String is the recorded form: host, boot session, pid, nonce and pid namespace
// in one bounded field.
//
// It is stored in a Job's sanitized summary — a place a person reads — so it is
// deliberately one compact string rather than nested JSON, and it carries nothing
// but identity: no path, no user, no plugin value.
func (r RuntimeIdentity) String() string {
	if r.Nonce == "" {
		return fmt.Sprintf("%s/%s/%d", r.Host, r.BootSession, r.PID)
	}
	if r.PIDNamespace == "" {
		return fmt.Sprintf("%s/%s/%d/%s", r.Host, r.BootSession, r.PID, r.Nonce)
	}
	return fmt.Sprintf("%s/%s/%d/%s/%s", r.Host, r.BootSession, r.PID, r.Nonce, r.PIDNamespace)
}

// ParseRuntimeIdentity reads a recorded identity back, with or without the nonce
// and pid namespace an earlier release did not record. An unreadable value
// yields ok=false rather than a zero identity, so a caller cannot mistake
// "nothing was recorded" for "recorded, and this process is it".
func ParseRuntimeIdentity(recorded string) (RuntimeIdentity, bool) {
	parts := strings.Split(recorded, "/")
	if len(parts) < 3 || len(parts) > 5 {
		return RuntimeIdentity{}, false
	}
	pid, err := strconv.Atoi(parts[2])
	if err != nil || pid <= 0 {
		return RuntimeIdentity{}, false
	}
	if parts[0] == "" {
		return RuntimeIdentity{}, false
	}
	identity := RuntimeIdentity{Host: parts[0], BootSession: parts[1], PID: pid}
	for index, part := range parts[3:] {
		if part == "" {
			return RuntimeIdentity{}, false
		}
		if index == 0 {
			identity.Nonce = part
		} else {
			identity.PIDNamespace = part
		}
	}
	return identity, true
}

// RuntimeLiveness is what can be proved about the process that accepted work.
type RuntimeLiveness int

const (
	// RuntimeUnknown means nothing could be proved: another machine, another
	// boot, another pid namespace, or a process table that cannot be inspected.
	// Work whose runtime is unknown stays blocked; it is never interrupted and
	// never redispatched on this answer.
	RuntimeUnknown RuntimeLiveness = iota
	// RuntimeAlive means a process with that pid exists in this pid namespace in
	// this boot session. It may be a reused pid, so this is not proof of
	// ownership either — it is proof that the work is not provably finished.
	RuntimeAlive
	// RuntimeGone means the runtime cannot still be there: its pid is free in
	// the process table it was recorded in, or that pid is held by this process.
	RuntimeGone
)

// Liveness answers what can be proved about the process a recorded identity names.
//
// Only a record from this process table can prove anything, and only this
// process table can be read. The rules, and why each one is the conservative
// reading:
//
//   - no boot session recorded: Unknown. Without it a pid says nothing across a
//     reboot, and this is the platform where the primitive is unavailable.
//   - another hostname, another boot session, or another pid namespace:
//     Unknown. A hostname is not unique across machines, so a boot session that
//     differs may be a reboot here or a live process on another machine with
//     this name; a pid namespace that differs is a container's table this
//     process cannot read. This is the case §3 calls out: a mismatch alone
//     never proves death. The work waits for a person, or for its lease.
//   - this table, our own pid and our own nonce: Alive, and provably *this*
//     runtime.
//   - this table, our own pid and another nonce: Gone. One pid names one
//     process at a time and this process holds it, so the process that
//     recorded it has exited.
//   - this table, our own pid and no nonce: Unknown. The record was written
//     without one, by an earlier release or by a store that kept only some of
//     the fields, so it can neither name this process nor prove its writer gone.
//   - this table, another pid: Gone when the process does not exist, Alive when
//     one does. A reused pid reads as Alive, which errs toward leaving a Job
//     blocked rather than interrupting work that may still run.
//
// "This table" is equal hostname, boot session and pid namespace. On Linux, where
// pid namespaces exist, a side without one (a record from before they were
// recorded, or a process that could not read its own) names no table, so the
// answer is Unknown; on other systems neither side has one and the boot session
// is the table.
func (r RuntimeIdentity) Liveness() RuntimeLiveness {
	if r.Host == "" || r.BootSession == "" {
		return RuntimeUnknown
	}
	current := CurrentRuntimeIdentity()
	if hasPIDNamespaces && (r.PIDNamespace == "" || current.PIDNamespace == "") {
		return RuntimeUnknown
	}
	if r.Host != current.Host || r.BootSession != current.BootSession || r.PIDNamespace != current.PIDNamespace {
		return RuntimeUnknown
	}
	if r.PID == current.PID {
		switch r.Nonce {
		case "":
			return RuntimeUnknown
		case current.Nonce:
			return RuntimeAlive
		default:
			return RuntimeGone
		}
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
