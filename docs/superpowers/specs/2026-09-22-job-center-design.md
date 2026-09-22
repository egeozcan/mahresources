# Unified Job Center

Status: final — operator decisions approved
Date: 2026-09-22

## Goal

Replace the download-specific history page with a durable **Job Center** that can inspect, analyze, and control every user- or operator-facing background Job. A Job Kind exposes only the controls and outputs it actually supports; the interface does not infer behavior from download-shaped statuses or source-name checks.

The canonical domain language is recorded in [`CONTEXT.md`](../../../CONTEXT.md). Two enduring architectural decisions are recorded separately:

- [ADR 0006: One durable Job control plane with specialized executors](../../adr/0006-durable-job-control-plane.md)
- [ADR 0007: A retry creates a new immutable Job](../../adr/0007-retry-creates-a-new-job.md)

## Why this is needed

The current system has a partially unified live panel over several unrelated registries, but durable history is fragmented:

- downloads have `/downloads` and `DownloadHistoryEntry`;
- exports, imports, Resource Reduction computation, and maintenance work use generic in-memory queue jobs;
- plugin actions have a separate in-memory registry and event vocabulary;
- plugin commands and command imports have their own durable authoritative records;
- deferred downloads and plugin schedules have separate durable scheduling records.

The panel already calls this collection “Jobs,” yet its rendering and controls still special-case sources and statuses. Only downloads have a general user-facing durable history page. Work can therefore disappear from the only surface that exposes its result or controls, and adding a new Job Kind requires editing central UI conditionals.

## Scope

The Job Center includes every accepted unit of user- or operator-facing background work, including work that is scheduled or queued but has not started. Initial coverage is complete across:

- remote and deferred downloads;
- group export;
- group import parse and apply;
- Resource Reduction computation;
- user-facing maintenance work such as similarity recomputation;
- plugin actions, `mah.start_job`, and scheduled plugin occurrences;
- plugin command runs and command imports.

Internal housekeeping with no user/operator identity or useful control surface remains outside the Job domain.

## Non-goals

The first release does not provide:

- history for synchronous requests;
- a generic “launch Job” interface;
- arbitrary plugin-supplied Job Center UI, endpoints, or privileged custom commands;
- email, push, or read/unread notifications;
- general-purpose business intelligence;
- exactly-once execution;
- reconstruction of memory-only history already lost before migration;
- implicit deletion of Resources or domain entities when Job history expires;
- universal cancel, pause, retry, or repeat support across all Job Kinds.

## 1. Domain model

### Job and Job Kind

A **Job** is one durable execution of accepted background work. It exists before execution while scheduled or queued, remains inspectable after termination, and has a globally unique opaque UUIDv7 identifier. IDs encode no kind, owner, state, or authorization decision.

A **Job Kind** is a versioned family of Jobs with shared input semantics, phases, outputs, and supported commands. Each Kind retains its specialized executor behind the common Job control plane.

### Job State and phase

Every Job has exactly one normalized state:

| State | Meaning | Terminal |
|---|---|---|
| `scheduled` | Accepted for a concrete future time | no |
| `queued` | Eligible and waiting for dispatch | no |
| `running` | An executor owns the work | no |
| `paused` | Executor confirmed a resumable checkpoint | no |
| `blocked` | Cannot proceed without policy, dependency, or operator resolution | no |
| `succeeded` | Required side effects and outputs committed | yes |
| `failed` | Execution ended unsuccessfully | yes |
| `cancelled` | An accepted cancellation won lifecycle ownership and execution stopped | yes |
| `interrupted` | Execution ended unexpectedly and requires a new Job to continue | yes |

A Kind may publish a finer **phase**, such as “downloading segments,” “parsing manifest,” “pausing,” “quarantined,” or “assembling video.” Phase never redefines normalized state. The current progress message is also descriptive rather than a state.

Terminal state and identity are immutable. Output availability, dismissal, pinning, and replay-data expiry may change later without rewriting the outcome.

### Lineage

Jobs can have typed durable links:

- `retry-of` — recovery from an unsuccessful terminal Job;
- `repeat-of` — another execution of successful work;
- parent/child — independently scheduled or controlled stages of a workflow.

Every linked Job keeps its own identity, state, events, outputs, owner, and authorization.

**Retry lineage is linear by default.** Only the latest terminal leaf may advertise Retry, and at most one active Job may exist in the chain. A Kind may explicitly opt into branching only when parallel recovery is meaningful.

**Repeat lineage may branch.** Repeats are independent reruns, subject to the Kind’s duplicate and concurrency policy.

A parent does not universally inherit child outcomes or cancellation behavior. Each workflow declares which children are required, how they contribute to the parent outcome, and which child controls a parent command invokes.

### Retry, Repeat, and internal recovery

A Retry or Repeat:

- creates a new Job;
- copies unchanged replayable input;
- revalidates current role, scope, plugin availability, settings, and network/policy constraints;
- is owned by the person requesting it;
- never changes an ancestor’s outcome.

Changed input is a fresh submission, not Retry or Repeat.

Retry is for `failed`, `cancelled`, or `interrupted` work and is offered only when the Kind proves it safe. Repeat is for successful work and is offered only when repeating the side effect is meaningful and safe. No adapter gains either command merely from state.

Low-level transient recovery while a Job is still running—such as retrying an HTTP request or HLS segment—remains within that Job. Significant recovery may produce a bounded diagnostic event, but network-level attempts are not Jobs.

### Schedules

A recurring **Schedule** is a durable definition, not a Job. Each occurrence becomes a Job when materialized for dispatch. A skipped occurrence becomes a Job only if it was actually materialized or claimed; otherwise it remains schedule history.

A one-off deferred operation is a `scheduled` Job immediately because it already names one concrete execution.

### Provenance

One field cannot represent all provenance. A Job records separately:

- **Owner** — principal whose Job Center includes it;
- **Actor** — principal whose current authority execution or a command uses;
- **Origin** — UI, API, CLI, plugin, Schedule, or system component that initiated it;
- **Kind** — what work it performs.

These are historical facts. User deletion removes the live principal reference without transferring authority. A scheduled Job whose actor disappears becomes `blocked`; it never falls back to root or another administrator. Ownerless/system Jobs are admin-only.

## 2. One control plane, specialized executors

The **Job Service** owns:

- durable identity and normalized lifecycle;
- ownership, origin, visibility, and authorization chokepoints;
- Kind/version registration;
- lineage;
- significant events and current progress snapshots;
- advertised commands and idempotent command outcomes;
- typed outputs;
- scheduling, claims, leases, and recovery coordination;
- dismissal, pinning, retention, search, and aggregate summaries.

It does not implement every workload. Downloads, plugin VMs, plugin commands, Resource Reductions, imports, and other specialized systems keep executors suited to their safety and concurrency rules.

A Job Kind joins through a versioned adapter that supplies:

- input validation, sanitized summary, redaction, and replay encoding;
- title and presentation metadata;
- dispatch and restart reconciliation;
- progress and phase vocabulary;
- currently available commands and their handlers;
- output publication and access checks;
- failure classification;
- type-specific retention requirements.

Adapters request transitions through the Job Service; they never update Job lifecycle rows directly. If an adapter disappears after a deploy or plugin disablement, its history remains inspectable and nonterminal work becomes `blocked`. The system never falls back to a different executor.

## 3. Durable acceptance and dispatch

### Acceptance boundary

The system reports success only after it durably persists, in one transaction:

- the Job row;
- the initial Job Event;
- owner, actor, origin, and Kind version;
- sanitized searchable summary;
- replay envelope or an explicit non-replayable classification;
- initial scheduling information.

Execution begins only after commit. If a Job is spawned by another database mutation, Job creation occurs in that transaction or through a transactional outbox. No accepted user-facing Job exists only in memory.

### Delivery guarantee

Restorable work uses **at-least-once** dispatch, not exactly once. Each dispatch carries the stable Job ID and an execution token. A Kind must make side effects idempotent or implement reconciliation. Legacy closure-backed `mah.start_job` work is non-restorable: its durable record survives runtime loss, not its Lua callback or captured state.

A crashed `running` Job is never blindly rerun. Its adapter reconciles durable side effects and chooses one of: resume, queue, succeed, fail, block, or interrupt. If safe continuation requires another logical execution, the old Job becomes `interrupted`; the user may create a new Retry Job only if the Kind advertises safe replay.

### Claims and multiple processes

Execution claims are database-backed and include:

- claimant identity and execution token;
- lease expiry;
- heartbeat;
- atomic claim and state transition;
- stale-claim reconciliation through the Kind adapter.

Deployment-wide global or per-Kind concurrency limits use database-backed accounting. Process-local throttles remain only when intentionally scoped to one process and documented as such.

Lifecycle, progress, and output publication must atomically check the current execution token; obsolete executors cannot publish. Lease expiry or a missed heartbeat permits reconciliation, not an assumption that execution stopped. Token fencing alone cannot stop external side effects: occupied capacity and unresolved claims remain held, and replacement dispatch and terminal classification are prohibited, until the owning runtime proves that external work is quiescent. If that cannot be proved, the Job remains nonterminal and becomes `blocked`.

For the first release, plugin-command runs and command imports have **one fenced command runtime per database, owning that database's single staging namespace**. Other Job Kinds and the Job Service may remain multi-process. Independent command runtimes with different staging roots against the same database are unsupported. Only the fenced staging owner may dispatch, reconcile, or clean up command work; database ownership and the exclusive staging-root lease must both be enforced. A replacement owner must establish the prior runtime and its workers/process groups are quiescent before releasing their capacity or dispatching replacements. A boot-session mismatch on another host never proves process death. Multi-host command execution/storage affinity is outside this release.

### Restart behavior

- Restorable `scheduled` and `queued` Jobs are redispatched after policy and adapter validation.
- Closure-backed `mah.start_job` Jobs are never redispatched after originating-runtime loss. Once that loss is proven, queued or running instances become `interrupted`, retain their history, and offer no Retry. An expired lease alone is not proof of runtime loss.
- `running` Jobs are reconciled, never assumed safe to rerun.
- `paused` Jobs remain paused unless their adapter proves the checkpoint invalid, in which case they become blocked or interrupted.
- `blocked` Jobs remain blocked until an explicit command, policy change, or reconciliation makes progress possible.
- terminal Jobs never reopen.

## 4. State transitions and command races

The Job Service enforces one state machine. Every accepted transition and its Job Event commit atomically. Invalid transitions are refused.

### Durable control intent

Cancellation and pause can take time. A command first persists a durable control intent:

- while stopping, the Job remains `running` with a “Cancelling…” phase;
- while reaching a safe checkpoint, it remains `running` with a “Pausing…” phase;
- it enters `cancelled` only after execution stops or reconciliation proves it cannot continue;
- it enters `paused` only after the executor confirms a resumable checkpoint;
- queued or scheduled work with no executor may transition immediately.

Once cancellation intent wins, later success cannot overwrite it. If success committed first, cancellation is refused and the Job remains `succeeded`. If cancellation won first, unavoidable outputs already created remain attached and are clearly labelled. `cancelled` means cancellation won lifecycle ownership, not that no side effect occurred.

`blocked` may return to `queued` through an explicit command or reconciliation. `paused` may return to `queued`. `interrupted` is terminal; continuation requires Retry.

### Job Commands

A Job response explicitly advertises its commands. The UI never derives them from Kind and state conditionals. A command declaration includes:

- stable command key and label;
- endpoint;
- the Job version from which availability was computed;
- destructive flag and confirmation text;
- bulk eligibility;
- optional Kind-specific presentation metadata from a trusted host adapter.

Command execution atomically rechecks authorization, Job version, state, control intent, and Kind policy. A stale request returns `409` with the fresh visible snapshot. Hidden Job IDs remain indistinguishable from nonexistent IDs.

Commands accept an idempotency key. Repeating the same request returns its recorded outcome. Bulk execution records an independent result for every Job and may partially succeed.

The common command vocabulary includes cancel, pause, resume, retry, repeat, inspect output, download artifact, dismiss, pin, forget replay data, and trusted Kind-specific commands. A Kind advertises only commands it can honor safely.

## 5. Inputs, secrets, and versioned replay

Input has two representations:

1. a sanitized, bounded, searchable summary suitable for list and detail APIs;
2. an opaque replay envelope never returned by those APIs.

Kinds declare redaction rules. Query strings, cookies, authorization headers, sensitive plugin values, claim tokens, and raw payloads never enter searchable summaries or user-visible Job Events.

Secret replay fields are encrypted at rest with a dedicated `JOB_REPLAY_KEY`, separate from template signing. The deployment contract is:

- multi-process deployments configure one shared stable key;
- single-process persistent deployments may auto-generate a private `0600` key file under the application data root;
- ephemeral mode may use a per-boot key;
- envelopes carry key IDs so rotation can read old data and write with the active key;
- startup refuses configurations where durable secret work could be accepted without a stable key appropriate to the deployment.

Each envelope records its Job Kind version. An adapter must explicitly migrate old envelopes before replay. Migrated input is validated under current policy. Without a migration or decryption key, Retry/Repeat is not advertised; sanitized history remains viewable. Nonterminal work that cannot decode its required input becomes `blocked`, not silently discarded or executed with incomplete input.

Envelopes and decryption keys required for first execution, continuation, or reconciliation are protected throughout every nonterminal state, including scheduled, queued, running, paused, and blocked. Key rotation must preserve access to those envelopes. Restorable acceptance must retain all execution-required input; a non-replayable classification does not waive that requirement.

Replay envelopes have a shorter independent retention, seven days by default **from terminal completion (`finished_at`)**, never from acceptance. Owners may use **Forget replay data** on their finished Jobs, and administrators may enforce an earlier post-terminal ceiling. Forget or administrative purge is refused while input is execution-required: the Job must first be safely cancelled or reconciled to a terminal state. Purging post-terminal replay data removes replay commands without deleting history. Forget and expiry purge the canonical envelope and any remaining legacy replay copies atomically, recording a durable purge marker that migration/backfill must honor; neither compatibility readers nor migration may reconstruct purged input from a legacy URL or payload. The cutover retirement barrier in §18 makes this guarantee true for migrated Jobs too.

## 6. Events, progress, logs, and live delivery

### Job Events

A Job Event is a durable significant fact, including:

- accepted, scheduled, queued, and started;
- phase changes and meaningful progress checkpoints;
- control requested, accepted, or refused;
- warning or bounded internal recovery summary;
- output publication or expiry;
- retry, repeat, and parent/child linkage;
- terminal outcome.

Routine progress ticks are not events. Every event has a stable sequence. State transitions and their events are atomic.

Configurable hard limits bound sanitized event size, event count, output count, summary size, and plugin event rate. Host lifecycle and terminal events reserve capacity and cannot be displaced. When optional capacity is exhausted, one visible truncation warning is recorded, current progress continues updating, and the Job itself does not fail.

### Progress snapshot

Current progress is optional and structured:

- phase label;
- completed amount;
- total amount, if known;
- unit such as bytes, items, segments, or entities;
- human-readable message;
- optional adapter-supplied estimated completion time.

Unknown totals render as indeterminate. The Job Service stores only the latest bounded snapshot.

### Verbose logs

Verbose executor logs are not Job Events. They live in a Kind-specific log store or expiring Job Output. A Job may publish a sanitized tail and log reference. Logs usually expire sooner than history, and their loss never changes the recorded outcome. Secrets are redacted before persistence rather than merely hidden while rendering.

### Resumable SSE

`GET /v1/jobs/events?version=2` provides the canonical visibility-filtered resumable stream during compatibility. `Last-Event-ID` or an explicit cursor catches up from durable events before switching to live updates. Clients tolerate duplicates and reconcile by sequence. Unversioned `/v1/jobs/events` retains its legacy representation as specified in §12; the two representations do not share event IDs/cursors.

In-process notification, PostgreSQL `LISTEN/NOTIFY`, or polling may wake stream handlers, but wake-ups are optimizations. The durable event cursor is the correctness mechanism, giving SQLite and PostgreSQL equivalent behavior.

## 7. Outputs and successful completion

A Job may publish typed Job Outputs:

- entity link;
- downloadable artifact with expiry;
- report or detail link;
- structured summary;
- explicitly safe external link;
- verbose-log reference.

Each output has independent availability and retention. Access is reauthorized when the output is opened; visibility of the Job is not a capability token for the output.

A Job becomes `succeeded` only after:

- every required domain side effect commits;
- required output references are durable;
- required artifact availability is verified;
- the terminal state and outputs can no longer contradict one another.

Database-owned outputs publish transactionally where possible. Filesystem or external artifacts use staged-then-publish behavior. Optional output publication failures become warnings; required-output failure prevents success.

Artifact expiry never changes success to failure. Planned expiry is visible in advance. Confirmed expiry changes the output to `expired` or `removed` and records a Job Event. Cleanup is bounded and resumable; an already-missing artifact is treated as removed.

Pinning a Job does not pin an artifact unless that output explicitly supports and receives its own retention command.

## 8. Authorization and visibility

The Job Center is personal by default with administrative oversight:

- users see Jobs they own, subject to stricter Kind policy;
- administrators may see all Jobs;
- plugin-command Jobs remain admin-only;
- ownerless/system Jobs are admin-only;
- disabled or deleted owners never become globally visible.

Ownership grants visibility, not permanent control. Every command rechecks the current actor’s role, scope, plugin access, and Kind-specific authorization. A demoted user may still inspect sanitized history while losing cancel, retry, repeat, or output access.

Lineage does not grant transitive visibility. Every related Job is authorized independently. Hidden parents, children, retries, or repeats are neither named nor counted. Expired visible relatives may appear only as neutral gaps with no leaked metadata.

Job Events contain sanitized facts useful to someone authorized to inspect the Job. Fuller actor, policy, reconciliation, infrastructure, stack, and security detail remains in protected administrative audit logs. Unauthorized probes are recorded only there, never on the target Job.

## 9. Dismissal, pinning, deletion, and retention

These are distinct operations:

- **Dismiss** hides a Job from that viewer’s default list; it does not delete history.
- **Pin** exempts Job metadata and Job Events from automatic expiry.
- **Delete permanently** is an exceptional admin-only operation over Job metadata/events.
- **Forget replay data** removes the replay envelope without removing sanitized history.

“Clear completed” becomes **Dismiss finished**.

Users may pin their own Jobs; administrators may pin any Job or exempt operational lineages. The default per-user pin limit is 100 and is runtime-configurable. Pinning one Job does not pin related Jobs. The UI offers an explicit “Pin visible lineage” operation.

Default metadata retention is:

- succeeded and cancelled Jobs: 30 days;
- failed and interrupted Jobs: 90 days;
- replay envelopes: 7 days from terminal completion, with execution-required input and keys protected as specified in §5;
- all nonterminal Jobs, including blocked Jobs: never swept as ordinary history.

Metadata retention starts at terminal completion (`finished_at`). Abandoning blocked work requires an explicit, safely reconciled terminal transition before retention begins. Unresolved execution claims and recovery records remain intact, including quarantined command work whose process group may still be alive; neither expiry nor permanent deletion may bypass that protection.

Dismissal does not accelerate expiry. Artifact and verbose-log retention remain Kind-specific. Permanent deletion or automatic expiry never deletes produced Resources or domain records. Sweep work is bounded, resumable, and records output removal before pruning the relevant history.

## 10. Failure and time model

A failure records:

- stable failure code;
- broad class such as policy, validation, dependency, timeout, capacity, conflict, cancellation, or internal;
- sanitized user-facing message;
- protected diagnostic reference;
- adapter judgment about retryability.

Raw error text is neither a taxonomy nor command policy. Retry still comes from the advertised Kind command.

Every Job records common UTC instants and derived durations:

- accepted at;
- scheduled for, when applicable;
- queued at;
- first started at;
- last resumed at;
- finished at;
- cumulative running duration;
- cumulative paused/blocked duration;
- queue duration.

Durations derive from durable transitions and remain meaningful across restart.

## 11. Persistence model

The common relational core consists of:

- `jobs` — UUID, Kind/version, state, phase, ownership/provenance, lineage, timestamps, summary, control intent, version, and expiry;
- `job_events` — immutable ordered significant events;
- `job_outputs` — typed outputs and availability;
- `job_command_requests` — idempotency keys and command outcomes;
- `job_claims` — leases and heartbeats;
- encrypted replay envelopes, separate from searchable summaries;
- per-user dismissal and preference records.

Fields used for authorization, filtering, claiming, retention, and ordering are relational and indexed. Bounded sanitized structured details may use JSON. Type-specific domain data remains in its domain tables.

Legacy IDs are retained as searchable references on migrated Jobs but are not the new primary identity. A separate durable compatibility-handle mapping records the current retry leaf for legacy clients (§12); it does not change those historical references or canonical Job identities.

## 12. Canonical API

New integrations use:

- `GET /v1/jobs` — cursor-paginated visible Jobs and filters;
- `GET /v1/jobs/{id}` — authoritative detail with advertised commands and outputs;
- `GET /v1/jobs/{id}/events` — ordered visible timeline;
- `GET /v1/jobs/events?version=2` — canonical resumable visible SSE stream during compatibility;
- `POST /v1/jobs/{id}/commands/{command}` — one idempotent command;
- `POST /v1/jobs/commands/{command}` — bulk command with per-Job outcomes;
- `GET /v1/jobs/summary` — bounded aggregates under the same filters.

A cursor, not offset, paginates potentially large history. Authorization predicates are shared by list, detail, commands, outputs, event catch-up, and aggregate queries.

### Legacy identity and representation

Legacy queue and control routes remain compatibility adapters for at least one documented release cycle and no less than six months. This includes `/v1/download/*`, `/v1/jobs/queue`, `/v1/jobs/get`, and the existing verb endpoints under `/v1/jobs`; submission endpoints on their domain surfaces remain valid. Compatibility responses emit deprecation headers and link to replacement documentation. `/downloads` remains a long-lived redirect to `/jobs?Kind=download`, translating recognized legacy filters, because bookmarks are inexpensive to support.

A legacy `id` is a durable **compatibility handle**, not a canonical UUID. Existing IDs become handles at migration; legacy submission responses also supply handles. Each handle projects the current leaf of its linear Retry lineage. Canonical UUID routes always name exactly one immutable Job and never follow a handle or redirect to a successor.

- Legacy Retry preserves `{"status":"retrying"}` and adds `canonicalJobId` naming the new Job. Creating that Job, recording the command outcome, and moving the handle from the previous leaf commit atomically. The old Job's outcome is untouched. A linear Retry through the canonical API advances any handle for that same leaf in the same transaction, so legacy polling cannot strand a client on an ancestor.
- Legacy get, queue/list, events, and controls project or act on the handle's current leaf. Their `id` always remains the handle; `canonicalJobId` separately identifies the projected Job. Legacy lists/events expose one projection per handle rather than relabelling every ancestor as the same execution. Resume and cancel reach the projected execution, not the immutable ancestor.
- Every resolution rechecks visibility and every control rechecks current authorization on the resolved Job. A handle grants no rights over a successor or its lineage. A hidden successor is indistinguishable from a missing handle; it is not replaced with a visible ancestor. Resolution and command validation are atomic against handle movement so a racing command cannot act on the wrong leaf.
- Another Retry while the leaf is active is refused with `409`; once that leaf fails, is cancelled, or is interrupted, a later Retry may create its successor if the Kind permits it. A succeeded leaf cannot be retried. A request carrying an idempotency key returns its recorded outcome on repetition rather than retrying a later leaf. Unkeyed legacy requests retain the old state-based behavior. Repeat and any explicitly branching recovery remain canonical-only; a handle never guesses which branch to follow.

The shared SSE path uses an explicit query representation version: unversioned `/v1/jobs/events` and `/v1/download/events` retain the legacy event vocabulary and handle projection during compatibility; `/v1/jobs/events?version=2` emits canonical Job events with UUID `id` values. Legacy `id` never becomes a UUID on retry. Each representation has its own event IDs/cursors, and cross-version cursors are refused rather than misinterpreted. After the documented compatibility window, the unversioned default may become canonical. New clients use version 2 explicitly and canonical UUID routes.

## 13. Job Center and live panel

### Navigation

Visible naming is:

- navigation: **Jobs**;
- page heading: **Job Center**;
- header trigger: **Jobs**;
- panel footer: **All jobs**.

The existing keyboard shortcut remains initially. The download-only icon becomes a neutral activity/queue icon. “Action” remains reserved for invokable bulk and plugin actions.

### Job Center

The default page uses stable sections:

1. Needs attention (`blocked`, `failed`, `interrupted`, excluding dismissed);
2. active and scheduled;
3. recent finished.

The complete All view is newest-first. Live updates move rows only when their state class changes and preserve focus and expanded state.

Filters cover state, Kind, owner, actor, origin, date windows, retry/repeat relationship, pinned/dismissed state, and available command. Search covers UUID/legacy ID, title, sanitized input summary, sanitized error, and output labels. Filter state lives in the URL.

Bulk commands come from the intersection of commands supported by every selected Job and return per-Job outcomes.

### Live panel

The panel is the glanceable live surface: active state, newest relevant finished Jobs, attention count, immediate supported controls, and detail links. It consumes the same API, state model, and advertised commands as the Job Center. It has no download-source conditionals.

The badge distinguishes active work from attention-required work. This is not an unread inbox, and viewing a Job changes no operational state.

### Job detail

`/job?id=<uuid>` shows:

- title, Kind, state, phase, owner, actor, origin, and timestamps;
- available commands;
- sanitized input summary;
- current progress;
- outputs and expiry;
- retry/repeat and parent/child lineage;
- significant-event timeline;
- warnings, terminal error, and optional verbose-log link;
- raw ID and diagnostic metadata in a disclosure.

State is conveyed in text, not color alone. Live announcements cover meaningful state or phase changes only, preserve focus, and never replay the full timeline after reconnect.

## 14. Analysis and scale

Under current filters, the first release provides:

- counts by state and Kind;
- success/failure rate;
- median and high-percentile queue and run duration;
- common sanitized failure classifications;
- asynchronous CSV/JSON export of filtered summaries.

The default analytics window is 30 days and the maximum interactive window is 90 days. Longer analysis is an export Job. Aggregates use indexed normalized columns and never scan replay envelopes, verbose logs, or raw event payloads. Visibility is identical to the list.

The feature is not a general BI system. Individual Job investigation remains the primary analysis experience.

## 15. Plugin boundary

In the first release:

- plugin actions and `mah.start_job` use a host-owned `plugin-action` Job Kind;
- existing closure-backed `mah.start_job` submissions are non-restorable and never offer Retry; their queued/running records become interrupted after proven originating-runtime loss (§3). A durable named-handler submission API requires a separate design;
- plugins may report bounded phase, progress, sanitized events, and typed outputs;
- plugins may opt into host-defined cancel or retry behavior only where the host can enforce it;
- plugins may not register arbitrary Job Center UI, endpoints, or privileged commands.

Future plugin-defined custom Job Commands require a separate capability, consent comparison, declarative contract, and authorization design.

## 16. Existing Job Kind command matrix

Initial adapters use this conservative baseline:

| Job Kind | Initial commands and outputs |
|---|---|
| Remote download | Cancel; pause/resume; Retry unsuccessful outcomes; inspect created Resource |
| Deferred download | Cancel while scheduled; Retry unsuccessful outcomes after dispatch |
| Group export | Cancel; Retry unsuccessful outcomes; Repeat successful export; download unexpired artifact |
| Import parse | Cancel; Retry while the staged archive remains; inspect plan |
| Import apply | Cancel; Retry only when the persisted result proves replay safety; inspect result |
| Resource Reduction computation | Cancel; Retry only if the Reduction exists and its version remains compatible; inspect Reduction |
| Similarity/maintenance work | Cooperative Cancel where supported; Retry unsuccessful outcomes; admin-only |
| Registered plugin action | Progress and outputs; Cancel or Retry only when registration explicitly declares support |
| Closure-backed `mah.start_job` | Progress and outputs; host-enforceable Cancel only; no Retry or restart redispatch |
| Scheduled plugin occurrence | Same capabilities as its registered action; link to Schedule |
| Plugin command run | Cancel; inspect command history/output; no Retry or Repeat by default |
| Plugin command import | Cancel where safe; Retry only while the admitted source remains and the durable import record proves replay safety |

A partially applied import that is not replay-safe offers no Retry. It publishes the partial-result report, identifies authorized created/changed entities, explains why replay is unsafe, and links to the import surface for a fresh upload and plan. Any future cleanup command needs its own explicit transaction and authorization design.

## 17. Initial lineage mapping

- import parse → apply: parent/child, because review and decisions separate execution;
- plugin command run → command import: parent/child;
- plugin action → `mah.start_job` invoked during it: parent/child;
- Retry and Repeat: their explicit typed lineage, never parent/child;
- scheduled occurrence → Schedule: domain link, not Job parentage;
- Resource Reduction computation → Resource Reduction: Job Output/domain link;
- download → Resource: Job Output, not child Job.

## 18. Migration and rollout

### Historical migration

Backfill durable existing records from download history, plugin command runs/imports, scheduled downloads, and durable Resource Reduction execution records. Preserve provable timestamps, owner, outcome, outputs, and legacy IDs.

Do not fabricate relationships or events absent from existing records. A mutable old download row with `attempts > 1` becomes one migrated Job with a migration note; unavailable historical attempts are not invented. Already-removed plugin actions, exports, and imports cannot be reconstructed.

A migrated payload is replayable only if it can be safely converted to the versioned encrypted envelope.

### Plaintext retirement barrier

Keeping obsolete registries for compatibility does not permit keeping their plaintext execution inputs. Before external cutover, required inputs from download history and scheduled downloads (including headers and exact URLs) must be migrated to verified canonical encrypted envelopes. Legacy payload columns are cleared; raw URL columns are cleared or replaced with sanitized display-only values. Retained registries contain only safe projections and references to canonical Jobs, never a second replay source.

Retirement is resumable and idempotent, keyed by legacy source identity with durable migration checkpoints:

1. Copy required input into the canonical encrypted envelope and verify it is readable and complete for execution/reconciliation. Never scrub the sole required input for nonterminal work. A conversion failure prevents that source from passing the cutover barrier; it is not permission to discard input or execute an incomplete Job.
2. Drain old writers and enforce a durable database migration/version fence that refuses old binaries before they can write or dispatch. Stop legacy plaintext writes and switch all execution, reconciliation, and replay readers, including compatibility routes, to canonical envelopes only. Mixed-version writers and rollback to a plaintext-writing binary are unsupported past this fence. Reconcile and reverify any source revisions or new rows written since the initial copy before scrubbing; a pre-drain snapshot alone is insufficient.
3. Scrub source payloads and raw/secret-bearing URLs, then durably mark retirement complete. A crash at any boundary resumes from the recorded state, never from an assumption that copying implies scrubbing. The external cutover waits for verification that all retained sources are retired and new writes cannot recreate them.

Purge markers and source-to-Job migration mappings take precedence over backfill and survive as long as any source can be reprocessed. Forget or replay expiry atomically removes the envelope and all remaining legacy replay copies and marks the input purged. Backfill cannot restore it, even after Job metadata expires. Compatibility replay has no legacy payload/URL fallback. These guarantees concern the live stores; pre-migration backups remain subject to the operator's backup retention and must pass the same migration barrier if restored.

### Internally phased, externally complete cutover

1. Add the Job Service and adapters while existing UIs remain unchanged.
2. Dual-publish and reconcile every current runner into durable Jobs.
3. Backfill existing durable history.
4. Complete the plaintext retirement barrier and verify lifecycle, visibility, compatibility handles, commands, outputs, and retention against old sources.
5. Cut over the panel, Job Center, and canonical APIs together for every supported Kind; expose Forget replay data only after retirement is verified.
6. Keep old routes as adapters through the compatibility window.
7. Remove obsolete registries only after compatibility expires and parity is proven.

Users never encounter a “unified” page that omits supported Job Kinds.

Active memory-only work cannot be conjured after an old process exits. Deployment across the cutover therefore uses the dual-publication release and an ordinary bounded shutdown/drain before removing old registries.

## 19. Configuration

New runtime settings:

| Setting | Default | Purpose |
|---|---:|---|
| `job_history_retention` | 30 days | Succeeded/cancelled Job metadata and events |
| `job_attention_retention` | 90 days | Failed/interrupted Job metadata and events after terminal completion; blocked Jobs are exempt |
| `job_replay_retention` | 7 days | Replay envelopes after terminal completion; execution-required input and keys remain protected |
| `job_pin_limit` | 100 | User-owned pinned Jobs |

Artifact, verbose-log, and type-specific payload retention remain separate.

Existing download-retention settings act as compatibility aliases for one release cycle. Migrated rows receive an `expires_at` computed from the policy that governed them at migration time; later runtime changes apply according to the normal Job retention contract rather than rewriting historical outcomes.

`JOB_REPLAY_KEY` and its rotation/key-file behavior are deployment configuration rather than runtime-editable settings.

## 20. Acceptance properties

The implementation is complete only when all of these hold:

1. Every current user/operator-facing Job Kind appears durably in the Job Center.
2. No accepted Job can disappear solely because the process restarted or an in-memory registry evicted it.
3. Terminal outcomes never change; Retry and Repeat create new Jobs.
4. The UI renders only server-advertised commands and never infers eligibility from source/status combinations.
5. Every command reauthorizes and is race-safe, versioned, and idempotent.
6. A restart never blindly re-executes a previously running Job.
7. Significant SSE events are recoverable from a cursor; progress may be coalesced.
8. Hidden lineage, events, outputs, and aggregates reveal no inaccessible Job.
9. Secret replay input never appears in summaries, events, logs, search, or ordinary APIs.
10. Artifact expiry, dismissal, replay-data expiry, and Job retention remain distinct.
11. SQLite and PostgreSQL expose equivalent lifecycle and visibility behavior.
12. The panel and Job Center share one API and one interpretation of state, commands, and outputs.
13. Initial cutover contains every Kind in the command matrix rather than only downloads.
14. Ordinary retention never removes nonterminal Jobs, execution-required input/keys, or unresolved claims/recovery records; deferred work remains executable beyond the replay-retention duration.
15. Closure-backed `mah.start_job` records survive proven runtime loss as interrupted Jobs with no Retry or redispatch.
16. Obsolete execution tokens cannot publish lifecycle, progress, or outputs. Command recovery and cleanup run only under the single fenced database/staging owner; lease expiry or a cross-host boot mismatch never releases capacity or dispatches replacements while external work may still be alive.
17. Cutover leaves no legacy plaintext replay payload or raw secret-bearing URL in live stores; old writers are fenced, nonterminal input is preserved in verified envelopes, and Forget/expiry cannot be undone by backfill or compatibility replay.
18. Legacy Retry followed by get/list/events/control using the unchanged handle tracks the new execution, including successive retries, without changing a canonical UUID's meaning. Legacy and v2 SSE representations have explicit, noninterchangeable identities and cursors.
