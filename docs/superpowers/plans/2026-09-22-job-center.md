# Unified Job Center Implementation Plan

> **For agentic workers:** Execute one task at a time with strict red → green cycles. Do not expose the canonical Job Center until the cutover gate in Task 17 passes for every Kind. One writer owns a checkout. Commit after each task's focused and package-level checks pass.

**Goal:** Replace the download-specific history and the fragmented in-memory job registries with one durable, capability-driven Job control plane covering every user- or operator-facing background Job.

**Architecture:** Add a deep `jobs` module below `application_context`. Its small interface owns durable acceptance, lifecycle transitions, events, progress, outputs, commands, claims, lineage, visibility predicates, preferences, replay envelopes, compatibility handles, and retention. The module holds no captured database handle: every operation receives `jobs.Deps{DB, Clock}` so request scope and transaction membership remain on the caller's `*gorm.DB`. `application_context` supplies Kind adapters and authorization-aware facades; existing download, plugin, import/export, reduction, and command executors remain specialized. The rollout is dual-published and internally testable until all Kinds, migration, encrypted replay, legacy projection, and plaintext retirement pass the cutover gate.

**Tech Stack:** Go, GORM, SQLite/PostgreSQL, Gorilla Mux, Server-Sent Events, AES-256-GCM, Pongo2, Alpine.js, Vite/Vitest, Playwright, Cobra, and the existing download/plugin-command staging systems.

**Spec:** `docs/superpowers/specs/2026-09-22-job-center-design.md`

**ADRs:**
- `docs/adr/0006-durable-job-control-plane.md`
- `docs/adr/0007-retry-creates-a-new-job.md`

---

## Confirmed test seams

Tests cross the same interfaces production callers use:

1. **Durable Job module seam:** exported `jobs.Service` methods with a real SQLite/PostgreSQL database. Tests assert returned snapshots/events, not private SQL helpers.
2. **Kind adapter seam:** exported `jobs.Adapter` behavior driven through `jobs.Service.Accept`, dispatch/reconciliation, and `ExecuteCommand`; executor-specific tests remain in their current packages.
3. **Application seam:** request-scoped `MahresourcesContext` methods prove role/scope/ownership binding and transactional acceptance.
4. **HTTP seam:** canonical and compatibility handlers mounted through real router middleware; hidden IDs remain 404.
5. **Browser seam:** Job Center and header panel behavior through Vitest and Playwright, including keyboard/focus/live-region behavior.
6. **CLI seam:** Cobra commands against an ephemeral real server.
7. **Migration seam:** old-schema SQLite/PostgreSQL fixtures upgraded by the production migration coordinator, followed by canonical reads and plaintext/fence assertions.

Do not add tests that call unexported SQL helpers, inspect private mutexes, or replace the Job Service with mocks at HTTP/browser seams. Use injected clocks, claimants, wake channels, executors, and crypto randomness only where behavior genuinely varies.

## Global invariants

- A Job is accepted only when its row and initial event commit in the caller's transaction. Dispatch begins after commit.
- Canonical IDs are UUIDv7 strings and always identify one immutable execution. Terminal Jobs never reopen.
- Retry/Repeat create linked Jobs. Retry is linear by default; Repeat may branch.
- State transitions, lifecycle events, outputs required for success, and compatibility-handle movement use one transaction and optimistic Job-version checks.
- Every mutation checks the execution token or command/version precondition that owns it. Stale executors cannot publish.
- Every list/detail/event/output/aggregate/command path uses one shared visibility query. Hidden IDs and hidden lineage are indistinguishable from missing data.
- Commands are server-advertised. No client infers eligibility from state/Kind.
- Searchable summaries/events are bounded and sanitized. Replay input is encrypted and never returned by ordinary Job APIs.
- Every nonterminal state is exempt from ordinary retention; required replay keys/envelopes and unresolved claims survive with it.
- Plugin-command dispatch/recovery/cleanup remains behind the existing exclusive staging-root lease and gains a database runtime fence; a Job lease never authorizes a second command runtime.
- Unversioned SSE and legacy routes keep legacy handle identity. Canonical SSE is explicitly `version=2` and uses separate durable cursors.
- The panel/page cutover is one release gate after every listed Kind is represented; no partial “unified” UI ships.

## Module and file ownership map

| Area | Files | Responsibility |
| --- | --- | --- |
| Common persistence | `models/job_model.go`, `main.go` | Job rows, events, links, outputs, commands, claims, replay envelopes, preferences, handles, migration state, indexes |
| Deep Job module | `jobs/{types,service,store,registry,replay,dispatch,commands,query,retention}.go` | State machine, atomic writes, adapters, claims, replay, visibility-neutral query mechanics, retention |
| App facade/adapters | `application_context/job_*.go`, `application_context/context.go`, `application_context/user_admin_guard.go` | Per-call deps, current principal, Kind adapters, migration, startup/shutdown |
| Existing executors | `download_queue/*`, `plugin_system/*`, `plugin_commands/*`, export/import/reduction callers | Specialized work; publish through Job execution/progress interface |
| Canonical HTTP | `server/api_handlers/job_*.go`, `server/routes.go`, `server/routes_openapi.go` | List/detail/timeline/SSE/commands/outputs/summary |
| Compatibility | existing download/action handlers and `cmd/mr/commands/jobs.go` | Handle projection and deprecation headers |
| UI | `templates/listJobs.tpl`, `templates/displayJob.tpl`, `templates/partials/jobPanel.tpl`, `src/components/{jobCenter,jobPanel}.js`, `src/main.js` | One command-driven page and panel |
| Migration/cutover | `application_context/job_migration.go`, `models/job_migration_model.go` or the migration rows in `job_model.go`, `main.go` | Dual publication, backfill, writer epoch, source scrub, cutover readiness |
| Proof/docs | package tests, `server/api_tests`, `e2e/tests`, CLI help, docs-site pages, `CLAUDE.md` | Cross-engine, crash, security, migration, UI, and operator contract |

---

### Task 1: Create the relational core and atomic lifecycle

**Files:**
- Create: `models/job_model.go`.
- Create: `jobs/types.go`, `jobs/service.go`, `jobs/store.go`, `jobs/service_test.go`.
- Modify: `main.go`, `application_context/user_admin_guard.go`.
- Modify: `internal/arch/layering_test.go`.

**Interface:**

```go
// jobs/types.go
type State string
const (
    StateScheduled State = "scheduled"
    StateQueued State = "queued"
    StateRunning State = "running"
    StatePaused State = "paused"
    StateBlocked State = "blocked"
    StateSucceeded State = "succeeded"
    StateFailed State = "failed"
    StateCancelled State = "cancelled"
    StateInterrupted State = "interrupted"
)

type Deps struct { DB *gorm.DB; Now func() time.Time }
type Acceptance struct { Kind string; KindVersion uint; State State; OwnerUserID, ActorUserID *uint; Origin string; Title string; Summary json.RawMessage; Replay ReplayInput; ScheduledFor *time.Time; LegacyRefs []LegacyRef }
type Snapshot struct { /* bounded public fields, version, progress, failure, timestamps */ }
type Transition struct { JobID string; ExpectedVersion uint64; ExecutionToken string; To State; Phase string; Event EventInput; Failure *Failure }

func (s *Service) Accept(Deps, Acceptance) (Snapshot, error)
func (s *Service) Transition(Deps, Transition) (Snapshot, error)
func (s *Service) Get(Deps, Access, string) (Snapshot, error)
```

**Models:**

- `Job`: UUIDv7 primary key; Kind/version; normalized state/phase; title and bounded sanitized summary; owner/actor/origin; durable visibility class (`owner` or `admin`); failure code/class/message/diagnostic reference; control intent; optimistic version; accepted/scheduled/queued/started/resumed/finished timestamps; accumulated running/paused/blocked durations; expiry; progress fields; pin-independent terminal outcome. The registered Kind fixes the visibility class; request input cannot choose it.
- `JobEvent`: immutable event UUID plus Job ID, per-Job sequence/version, event type, bounded sanitized detail, created time, reserved-host flag, and nullable global delivery sequence.
- `JobEventSequence`: the single serialized allocator row used only by the post-commit publication stage. A publisher can assign a delivery sequence only to an already committed event. This deliberately separates atomic lifecycle recording from resumable-stream ordering: PostgreSQL transaction sequence allocation is not commit ordered.
- `JobLink`: typed `retry-of`, `repeat-of`, or `parent-child` relation, unique on type/from/to.
- `JobWriterEpoch`: one database row recording the minimum compatible dual-publisher epoch. `models.CheckJobWriterEpoch` reads it with raw SQL immediately after opening the database and before `NewMahresourcesContext`, AutoMigrate, cleanup goroutines, writes, or dispatch. A missing table means a fresh/pre-Job database and is allowed; Task 1 AutoMigrate creates/seeds the Release-A-supported epoch. Later releases may advance, never silently lower, it.
- Add indexes for visible pagination `(visibility_class,owner,state,accepted_at,id)`, admin ordering, Kind/state, terminal retention, actor/origin, unsequenced event publication, delivery cursor, and Job timeline.
- Add `Job` to `stampedModels()` so user deletion NULLs owner/actor references; preserve origin and sanitized history. Because owner and actor are separate columns, add an explicit actor-nulling pass in `nullCreatorReferences`.

**Red:**

- Write table-driven service tests proving: UUIDv7 acceptance; Job+`accepted` event are atomic; every legal transition works; every illegal transition fails; optimistic version conflict leaves no event; terminal state cannot change; timestamps/durations advance from an injected clock; owner deletion leaves an admin-visible ownerless Job but no ordinary-user visibility grant.
- Add a transaction test that rolls back a domain write and `Service.Accept` together.
- Add a two-transaction PostgreSQL regression: transaction A records event A but does not commit, transaction B records and commits event B, then A commits. The publisher must assign delivery sequences only after each commit and a reconnect after B's cursor must still receive A; no cursor may skip an event because database sequence values committed out of order.
- Add startup tests that an actual Release-A binary/preflight refuses an advanced `JobWriterEpoch` before migrations, context construction, cleanup, writes, or dispatch, and that a fresh database boots and seeds the supported epoch.
- Add a layering test that `jobs/` imports only leaf packages (`models`, stdlib, GORM) and never `application_context`, `server`, `contracts`, `download_queue`, `plugin_system`, or `plugin_commands`.

Run and observe failure:

```bash
go test --tags 'json1 fts5' ./jobs ./internal/arch ./application_context -run 'Test(Job|Layering|DeleteUser)' -count=1
```

**Green:**

- Implement the state-transition table once in `jobs.Service`; adapters request transitions and never write `jobs` directly.
- `Accept` generates `types.NewUUIDv7()`, inserts the Job and initial unsequenced event on `Deps.DB`, and returns only after the transaction/caller transaction accepts both.
- `Transition` updates with `WHERE id=? AND version=? AND state=?`, increments version, accounts elapsed state duration, and inserts the unsequenced event in the same transaction.
- Add a short post-commit publisher transaction that locks `JobEventSequence`, selects a bounded set of committed events with no delivery sequence, assigns consecutive values, and commits. All code acquires the event-sequence allocator only in this publisher—never inside domain/lifecycle transactions—so it adds no cross-domain lock ordering. Per-Job timelines remain available immediately; canonical SSE emits only globally sequenced events.
- Run `models.CheckJobWriterEpoch` at the earliest database-open point in `main.go`; no process-owned runtime or cleanup loop exists before it passes.
- Normalize all stored times to UTC.

**Verify:**

```bash
go test --tags 'json1 fts5' ./jobs ./internal/arch ./application_context -count=1
go test --tags 'json1 fts5 postgres' ./jobs -count=1
git diff --check
```

**Commit:** `feat(jobs): add durable lifecycle core`

---

### Task 2: Add bounded events, progress snapshots, links, and typed outputs

**Files:**
- Modify: `models/job_model.go`, `jobs/types.go`, `jobs/service.go`, `jobs/store.go`.
- Create: `jobs/outputs.go`, `jobs/events_test.go`, `jobs/outputs_test.go`.

**Interface:**

```go
type Progress struct { Phase string; Completed, Total *int64; Unit, Message string; ETA *time.Time }
type OutputInput struct { Key, Type, Label string; Reference json.RawMessage; Required bool; ExpiresAt *time.Time }
func (s *Service) UpdateProgress(Deps, ExecutionRef, Progress) (Snapshot, error)
func (s *Service) AppendEvent(Deps, ExecutionRef, EventInput) error
func (s *Service) PublishOutput(Deps, ExecutionRef, OutputInput) (Output, error)
func (s *Service) Finish(Deps, FinishRequest) (Snapshot, error)
func (s *Service) Link(Deps, LinkRequest) error
```

**Red:**

- Prove progress replaces one bounded snapshot and does not create routine events.
- Prove stale execution tokens cannot update progress, append events, publish outputs, or finish.
- Prove required outputs and success commit together; missing/unverified required outputs refuse success.
- Prove optional output failure yields a warning without changing a committed success.
- Prove event/summary/output count and size ceilings; optional events collapse to one truncation warning while lifecycle/terminal events still fit reserved capacity.
- Prove links do not make hidden relatives visible or countable.

**Green:**

- Add `JobOutput` with type, safe reference JSON, availability (`available`, `expired`, `removed`), required flag, expiry/removal timestamps, and independent version.
- Keep progress columns on `jobs`; do not store each tick as an event.
- Add bounded event insertion with reserved host-event capacity and one truncation event.
- Make `Finish` verify required output rows before writing terminal state/event.

**Verify:**

```bash
go test --tags 'json1 fts5' ./jobs -run 'Test(Event|Progress|Output|Link|Finish)' -count=1
go test --tags 'json1 fts5 postgres' ./jobs -run 'Test(Event|Progress|Output|Link|Finish)' -count=1
```

**Commit:** `feat(jobs): persist progress events links and outputs`

---

### Task 3: Encrypt versioned replay input and enforce purge semantics

**Files:**
- Create: `jobs/replay.go`, `jobs/replay_test.go`.
- Modify: `models/job_model.go`, `jobs/types.go`, `jobs/service.go`.
- Modify: `application_context/context.go`, `application_context/runtime_setting_spec.go`, `application_context/runtime_settings.go` only for replay-retention access (the key is not runtime-editable).
- Modify: `main.go`, deployment config parsing tests, `.env.example` if present.

**Key contract:**

- `JOB_REPLAY_KEY` accepts one or more comma-separated base64-encoded 32-byte keys; the first is active and older keys are decrypt-only. The key ID is a non-secret SHA-256 fingerprint stored with each envelope.
- Persistent SQLite with no env key creates/reads `<file-save-path>/_job_replay_key` at mode `0600` using create-exclusive + atomic rename. PostgreSQL requires `JOB_REPLAY_KEY`, because it may have multiple processes/hosts. Ephemeral mode may generate a per-boot key.
- AES-256-GCM associated data binds ciphertext to Job ID, Kind, and Kind version. The envelope stores ciphertext, nonce, key ID, schema version, created time, purged time/reason, and terminal-based expiry.

**Red:**

- Prove ordinary snapshots/events/JSON/log strings never contain exact URL query secrets, Cookie, Authorization, or plugin secret values supplied in a replay fixture.
- Prove ciphertext decrypts only for the same Job/Kind/version; tampering and missing key IDs fail closed.
- Prove rotation reads old envelopes and writes only with the active key.
- Prove persistent SQLite file permissions and restart stability; PostgreSQL configuration without a stable key refuses startup before Job acceptance.
- Prove replay expiry starts at `finished_at`, not acceptance; scheduled/queued/running/paused/blocked envelopes never expire.
- Prove Forget is rejected for nonterminal Jobs and atomically writes a purge marker with envelope deletion for terminal Jobs.

**Green:**

- Define Kind-owned `ReplayCodec` registration with `Sanitize`, `Encode`, `Decode`, and `Migrate` hooks. `Accept` receives already validated Kind input but owns encryption/persistence.
- Return only `ReplayAvailability` (`available`, `expired`, `forgotten`, `unreadable`) in public snapshots.
- Distinguish missing migration/key from corrupt ciphertext. Nonterminal decode failures request `blocked`; terminal failures merely suppress Retry/Repeat.

**Verify:**

```bash
go test --tags 'json1 fts5' ./jobs ./application_context -run 'Test.*Replay' -count=1
go test --tags 'json1 fts5 postgres' ./jobs ./application_context -run 'Test.*Replay' -count=1
```

**Commit:** `feat(jobs): encrypt versioned replay envelopes`

---

### Task 4: Add database claims, leases, fencing, and reconciliation

**Files:**
- Create: `jobs/registry.go`, `jobs/dispatch.go`, `jobs/dispatch_test.go`.
- Modify: `models/job_model.go`, `jobs/service.go`, `jobs/types.go`.
- Create: `application_context/job_runtime.go`, `application_context/job_runtime_test.go`.
- Modify: `application_context/context.go`, `main.go`.

**Adapter interface:**

```go
type Adapter interface {
    Definition() Definition
    Dispatch(context.Context, Execution) error
    Reconcile(context.Context, ReconcileRequest) (ReconcileDecision, error)
    Commands(context.Context, CommandContext) ([]Command, error)
    ExecuteCommand(context.Context, CommandExecution) (CommandOutcome, error)
}
```

`Definition` fixes Kind key/version, restorable/non-restorable classification, concurrency group, and limits. Registration rejects duplicate Kind/version pairs.

**Red:**

- Two SQLite connections and two PostgreSQL transactions racing a claim admit one winner.
- Claim sets `running`, execution token, lease, capacity record, and `started` event atomically.
- Heartbeats extend only the matching token. Expiry invokes `Adapter.Reconcile`; it never itself queues or terminates work.
- Stale token publication is refused after a replacement/reconciliation token exists.
- A decision of `blocked-external-work-unproven` retains claim/capacity and prevents dispatch/terminal deletion.
- Global/per-Kind capacity is database-backed and observed across two Service instances.
- Shutdown stops new claims, asks adapters to quiesce, and leaves unresolved work durable.

**Green:**

- Add `JobClaim` and `JobCapacityLease` rows; use conditional updates and deterministic lock order.
- Add one application-owned runtime loop started/stopped in `main.go`; polling is correctness, local wake channels are optimization.
- Adapters receive only an `Execution` with Job ID, token, decoded input, actor, and progress/output callbacks. They never receive `*gorm.DB` lifecycle tables.
- Reconciliation decisions are explicit (`resume`, `queue`, `succeed`, `fail`, `block`, `interrupt`, `remain-running`) and applied by Service.

**Verify:**

```bash
go test -race --tags 'json1 fts5' ./jobs ./application_context -run 'Test(Claim|Lease|Heartbeat|Reconcile|Capacity|JobRuntime)' -count=10
go test --tags 'json1 fts5 postgres' ./jobs ./application_context -run 'Test(Claim|Lease|Heartbeat|Reconcile|Capacity)' -count=1
```

**Commit:** `feat(jobs): add fenced durable dispatch`

---

### Task 5: Add visibility, cursor queries, preferences, retention, and aggregates

**Files:**
- Create: `jobs/query.go`, `jobs/retention.go`, `jobs/query_test.go`, `jobs/retention_test.go`.
- Modify: `models/job_model.go`, `jobs/types.go`, `jobs/service.go`.
- Create: `application_context/job_context.go`, `application_context/job_context_test.go`.
- Modify: `application_context/runtime_setting_spec.go`, `runtime_settings.go`, `runtime_settings_test.go`, `main.go` flag/env wiring.

**Interface:**

```go
type Access struct { UserID uint; Administrator bool }
type Filter struct { States, Kinds, Origins []string; OwnerID, ActorID *uint; AcceptedAfter, AcceptedBefore *time.Time; Relationship, Command, Search string; Pinned, Dismissed *bool }
type Cursor struct { AcceptedAt time.Time; ID string }
func (s *Service) List(Deps, Access, Filter, Cursor, int) (Page, error)
func (s *Service) Timeline(Deps, Access, string, uint64, int) ([]Event, error)
func (s *Service) Summary(Deps, Access, Filter, time.Duration) (Summary, error)
func (s *Service) SetPreference(Deps, Access, PreferenceRequest) error
func (s *Service) Sweep(Deps, RetentionPolicy, SweepCursor, int) (SweepResult, error)
```

**Red:**

- Admin sees all; ordinary users only owned Jobs whose durable visibility class is `owner`; ownerless/deleted-owner and `admin`-class Jobs are admin-only.
- An owned plugin-command run/import remains hidden from its non-admin owner across list, detail, timeline, SSE catch-up/live delivery, lineage, outputs, aggregates, and command preflight—even after role demotion. Existing `/admin/plugin-commands` access remains admin-only during compatibility.
- The same hidden Job is absent from list/detail/timeline/output/summary/command preflight and returns not-found.
- Keyset pagination is stable under concurrent insertion and never uses offset.
- Search excludes ciphertext and protected diagnostics.
- Dismiss changes only that viewer's default list. Pin observes a per-user limit, does not pin relatives/artifacts, and exempts only metadata/events from sweep.
- Sweep starts from `finished_at`, never touches nonterminal/unresolved-claim Jobs, is bounded/resumable, and removes output artifacts before metadata according to adapter cleanup outcomes.
- Aggregate windows cap at 90 days, default to 30, and share the exact visibility/filter predicate with List.

**Green:**

- Add `JobPreference` unique `(job_id,user_id)` with dismissed/pinned timestamps.
- Build one query constructor used by list, detail existence, timeline join, SSE catch-up/live recheck, output lookup, lineage joins/counts, command lookup, and summary. Its base predicate is `administrator OR (visibility_class='owner' AND owner_user_id=current user)`. Every stricter Kind inspection rule must compile to durable relational visibility fields/predicates used by that constructor; never post-filter in Go, which would make pagination and aggregates leak or drift. Adapters may narrow, never widen.
- Add runtime settings: `job_history_retention=720h`, `job_attention_retention=2160h`, `job_replay_retention=168h`, `job_pin_limit=100`; old download retention values remain read aliases until Task 17.
- Sweep in fixed batches with a persisted cursor/checkpoint; no unbounded delete.

**Verify:**

```bash
go test --tags 'json1 fts5' ./jobs ./application_context -run 'Test(Visibility|List|Cursor|Preference|Pin|Dismiss|Retention|Summary)' -count=1
go test --tags 'json1 fts5 postgres' ./jobs ./application_context -run 'Test(Visibility|List|Cursor|Preference|Retention|Summary)' -count=1
```

**Commit:** `feat(jobs): add visible history preferences and retention`

---

### Task 6: Add idempotent advertised commands and immutable lineage

**Files:**
- Create: `jobs/commands.go`, `jobs/commands_test.go`.
- Modify: `models/job_model.go`, `jobs/types.go`, `jobs/service.go`.
- Modify: `application_context/job_context.go`, `application_context/job_context_test.go`.

**Interface:**

```go
type Command struct { Key, Label, Endpoint string; JobVersion uint64; Destructive, Bulk bool; Confirmation string; Presentation json.RawMessage }
type CommandRequest struct { JobID, Key, IdempotencyKey string; ExpectedVersion uint64; Actor Access }
func (s *Service) AdvertisedCommands(context.Context, Deps, Access, string) ([]Command, error)
func (s *Service) ExecuteCommand(context.Context, Deps, CommandRequest) (CommandResult, error)
func (s *Service) ExecuteBulkCommand(context.Context, Deps, BulkCommandRequest) []CommandResult
```

**Red:**

- A command is returned only when the adapter advertises it under current state/policy/access.
- Execution atomically rechecks visibility, current authorization callback, version, state, control intent, replay availability, and Kind policy.
- Stale requests return conflict with a fresh visible snapshot; hidden Jobs return not-found.
- Same `(Job, command, actor, idempotency key)` returns the recorded outcome, including failure, without invoking the adapter again.
- Cancel/pause persist control intent and a significant event before contacting an executor; success cannot overwrite won cancellation.
- Retry creates a new UUID Job and `retry-of` link, never changes the ancestor, permits one active chain leaf, and copies replay only after current authorization/policy validation.
- Repeat may branch and uses `repeat-of`.
- Bulk command computes the intersection of advertised bulk commands and records one independent outcome per Job.

**Green:**

- Add `JobCommandRequest` with unique actor-scoped idempotency tuple, request hash, status, serialized bounded result, created/completed times.
- Add a chain-lock strategy: lock Retry ancestors/leaves in canonical ID order on PostgreSQL; on SQLite make the first transaction statement a write. A conditional active-leaf predicate prevents two successors.
- Host-owned common handlers implement dismiss, pin, forget, and lineage pin. Kind adapters implement workload commands.

**Verify:**

```bash
go test -race --tags 'json1 fts5' ./jobs ./application_context -run 'Test(Command|Retry|Repeat|Lineage|CancelIntent|Bulk)' -count=10
go test --tags 'json1 fts5 postgres' ./jobs ./application_context -run 'Test(Command|Retry|Repeat|Lineage|CancelIntent|Bulk)' -count=1
```

**Commit:** `feat(jobs): add advertised idempotent commands`

---

### Task 7: Dual-publish remote and deferred downloads

**Files:**
- Create: `application_context/job_download_adapter.go`, `application_context/job_download_adapter_test.go`.
- Modify: `download_queue/history.go`, `download_queue/job.go`, `download_queue/manager.go`, `download_queue/generic_job.go` only to carry canonical Job ID/execution token and report through a narrow sink.
- Modify: `application_context/download_history_context.go`, `scheduled_download_context.go`, `context.go`.
- Modify: `server/api_handlers/download_queue_handlers.go`, `download_history_handlers.go`, and their tests to bridge every currently deployed queue/get/event/control/Retry route before Release A.
- Create: `application_context/job_compatibility.go`, `job_compatibility_test.go` with the minimal durable handle resolver used by the existing UI/API; Task 14 extends its representations and deprecation metadata.

**Kinds:** `remote-download@1`, `deferred-download@1`.

**Red:**

- A fresh foreground submission durably accepts canonical Job(s) before queue dispatch and returns no success if acceptance fails.
- Multi-URL submission accepts each URL independently and reports per-URL failures; no accepted Job exists only in memory.
- Download progress/terminal outcome/resource output mirror to the canonical Job with token fencing while legacy queue/history remain available.
- Pause enters `paused` only after the manager confirms a restartable checkpoint; current restart-from-zero behavior must either be represented as queued recovery or remain unadvertised until the adapter can honestly checkpoint.
- Retry through either canonical internals or every existing `/v1/download/*`, `/v1/jobs/{retry,get,queue,events,cancel,pause,resume}`, and `/v1/downloads/retry` route creates a new canonical Job before execution; no deployed path calls `DownloadManager.Retry` in place once a Job has a canonical reference.
- A durable compatibility handle projects the current retry leaf. Successor acceptance, `retry-of` link, command outcome, and handle movement commit atomically before dispatch. Successive retries through the unchanged legacy ID keep working; an active/succeeded leaf refuses them.
- Current scope, duplicate-URL, plugin policy, headers, and actor validation rerun on Retry.
- A deferred download is a `scheduled` Job at acceptance; due dispatch transitions that same Job to queued/running rather than creating a second execution.
- Deleting/disabled actor makes deferred work blocked, never root-owned.

**Green:**

- Register replay sanitizer/codec for `ResourceFromRemoteCreator`; summary contains safe scheme/host/name/targets, never query/headers.
- Extend manager submissions with optional canonical execution reference; keep standalone zero-value behavior for package tests that never enter the canonical adapter.
- Replace in-place Retry with Job Service Retry everywhere a canonical reference/handle exists. Resolve legacy IDs to handles for get/list/queue/events/cancel/pause/resume/retry, keep `id` equal to the handle, and add `canonicalJobId` where additive fields are safe. Task 14 completes headers and remaining projections, not execution semantics.
- Test the exact existing UI/API sequence during Release A: fail → legacy Retry → legacy get/list/event/control with unchanged ID → fail again → Retry again. Every attempt has a distinct UUID and immutable ancestor while the handle advances.
- Make scheduled-download source rows compatibility projections/migration sources, not durable authority for new submissions; dual-write until retirement.

**Verify:**

```bash
go test -race --tags 'json1 fts5' ./download_queue ./application_context ./server/api_handlers -run 'Test.*(Download|Deferred|Retry|Pause|Resume)' -count=5
go test --tags 'json1 fts5 postgres' ./application_context ./server/api_tests -run 'Test.*(Download|Deferred|Retry)' -count=1
```

**Commit:** `feat(jobs): adapt downloads to durable jobs`

---

### Task 8: Adapt export, import parse/apply, Resource Reduction, and maintenance Jobs

**Files:**
- Create: `application_context/job_export_adapter.go`, `job_import_adapter.go`, `job_reduction_adapter.go`, `job_maintenance_adapter.go` plus tests.
- Modify: `server/api_handlers/export_api_handlers.go`, `import_api_handlers.go`, `resource_reduction_handlers.go` and focused tests.
- Modify: `application_context/resource_reduction_compute.go`, `admin_context.go`.
- Modify: `groupio` only where idempotency/reconciliation evidence must be exposed.

**Kinds:** `group-export@1`, `group-import-parse@1`, `group-import-apply@1`, `resource-reduction-compute@1`, `similarity-recompute@1` (and every other user-facing maintenance source found by the Task 17 inventory test).

**Red:**

- Export accepts before worker start, stages artifact, publishes verified artifact output, then succeeds. Crash before publication reconciles staged/missing/published files without blind rerun.
- Export Cancel is cooperative; Retry unsuccessful and Repeat successful create new Jobs. Artifact expiry changes output availability, not Job success.
- Import parse stores enough encrypted input/reference to retry only while the staged archive exists; its plan is an output. Apply is a child Job and stores decisions in its own envelope.
- Import apply Retry is advertised only when existing `shouldRestorePlan`/result evidence proves replay safe; unsafe partial apply publishes the report and no Retry.
- Import ownership survives queue eviction because authorization comes from canonical Job, fixing the current ephemeral `importJobDenied` dependency.
- Reduction computation outputs a Reduction link and Retry only when the row/version remains compatible. Existing deadline recovery maps to reconciliation rather than reopening a terminal Job.
- Maintenance is admin-only and advertises only executor-supported Cancel/Retry.

**Green:**

- Change each submit handler to accept the canonical Job in the same transaction as any domain claim where applicable, then enqueue by canonical ID.
- Use child links for parse→apply and typed domain outputs for Reduction/Resource/report links.
- Move output authorization from in-memory queue lookup to canonical Job + output adapter.

**Verify:**

```bash
go test -race --tags 'json1 fts5' ./application_context ./server/api_handlers ./groupio -run 'Test.*(Export|Import|Reduction|Similarity|Maintenance|Job)' -count=5
go test --tags 'json1 fts5 postgres' ./application_context ./server/api_tests -run 'Test.*(Export|Import|Reduction|Similarity)' -count=1
```

**Commit:** `feat(jobs): adapt generic background workflows`

---

### Task 9: Adapt plugin actions, schedules, and non-restorable `mah.start_job`

**Files:**
- Create: `application_context/job_plugin_action_adapter.go`, `job_plugin_action_adapter_test.go`.
- Modify: `plugin_system/action_jobs.go`, `schedules.go`, `manager.go`, `actions.go` as needed to accept a host Job execution reference instead of owning lifecycle.
- Modify: `application_context/plugin_scheduler.go`, `job_event_dispatcher.go`.
- Modify: `server/api_handlers/action_handlers.go` and tests.

**Kind:** `plugin-action@1`, with sanitized subtype/origin fields for registered action, scheduled occurrence, and closure-backed `mah.start_job`.

**Red:**

- Async registered action acceptance is durable and bulk submission reports per-entity acceptance; synchronous actions remain outside Job history.
- Current plugin access, registration fingerprint, target scope, actor role, params, and plugin availability are checked at dispatch and Retry.
- Scheduled occurrence materializes one Job when claimed; skipped unmaterialized ticks do not fabricate Jobs. Schedule remains a domain link, not Job parentage.
- An action invoking `mah.start_job` creates a child link.
- Closure-backed Jobs are tagged non-restorable, never advertise Retry, and are never redispatched.
- Graceful plugin-manager/process shutdown proves callback loss and interrupts queued/running closure Jobs. On crash recovery, same-host boot/PID inspection may prove loss; an expired heartbeat or cross-host boot mismatch alone yields `blocked`, not `interrupted` or replacement dispatch.
- Plugin event/output rate/size limits cannot displace lifecycle/terminal events.

**Green:**

- Let Job Service own Job identity and snapshots; keep VM semaphore/locking in `plugin_system`.
- Persist originating runtime identity for non-restorable work. Reuse the platform process/boot inspection primitives already used by plugin commands; do not infer death from lease expiry.
- Retire `ActionJob` lifecycle authority only after all plugin callers publish through the adapter; temporary in-memory entries may remain projections for compatibility until Task 17.

**Verify:**

```bash
go test -race --tags 'json1 fts5' ./plugin_system ./application_context ./server/api_handlers -run 'Test.*(ActionJob|StartJob|Schedule|PluginAction|RuntimeLoss)' -count=10
go test --tags 'json1 fts5 postgres' ./application_context ./server/api_tests -run 'Test.*(Action|Schedule|Job)' -count=1
```

**Commit:** `feat(jobs): adapt plugin background work`

---

### Task 10: Integrate plugin-command runs/imports without weakening runtime fencing

**Files:**
- Create: `application_context/job_plugin_command_adapter.go`, `job_plugin_command_adapter_test.go`.
- Modify: `application_context/plugin_command_store.go`, `plugin_command_runtime.go`, `plugin_command_runtime_controller.go` and tests.
- Modify: `plugin_commands/store.go`, `dispatcher.go`, `recovery.go`, `types.go` only for canonical Job references/fence callbacks.
- Modify: `models/plugin_command_run_model.go`, `plugin_command_import_model.go` to carry canonical Job IDs during compatibility.

**Kinds:** `plugin-command@1`, `plugin-command-import@1`.

**Red:**

- Command/import durable record and canonical Job acceptance commit together before dispatcher admission; a failure leaves neither half accepted.
- Run→import is parent/child; command history/output and imported Resource are typed outputs.
- Only the controller that owns both the database command-runtime fence and existing staging-root `RuntimeLease` may dispatch, reconcile, signal, sweep, or publish command Job state.
- Graceful handoff and crash recovery are distinct. On graceful stop, the current controller releases its database fence and then the staging lease only after its local dispatcher reports `RuntimeLeaseReleasable`.
- After a crash, no dead dispatcher can report. A candidate first acquires the exclusive staging-root lease, then CAS-acquires the database fence in recovery mode, and scans durable runs/imports. It uses persisted PGID/boot-session identity plus process inspection to prove quiescence or perform the existing bounded recovery signalling. Expiry or a different/cross-host boot ID alone never proves death.
- If any prior worker/process group remains possible, the recovery owner keeps both fences, keeps capacity/recovery records occupied, publishes `blocked`/quarantine, and dispatches no replacement. Only proven quiescence permits capacity release and normal dispatch.
- Existing PGID/boot-session recovery drives the canonical reconciliation result. Quarantine becomes `blocked` phase/state without losing claim/recovery records.
- Add a crash test whose prior dispatcher can never call `RuntimeLeaseReleasable`: recovery succeeds only after process inspection proves no worker remains; a cross-host/uninspectable worker stays blocked indefinitely.
- Stale command live-lane callbacks cannot publish after token change.
- Command run has Cancel and inspect only; no Retry/Repeat. Import Retry appears only while admitted source and durable import record prove safety.

**Green:**

- Add a database `JobRuntimeFence` keyed by `plugin-command:<database namespace>` and bind its token into the controller plus canonical claims. Acquisition order is staging-root lease then database fence; a database-fence conflict releases the staging lease and backs off, avoiding split ownership/deadlock.
- The existing file lease remains mandatory. The DB fence does not replace filesystem/process quiescence, and the in-memory releasable report is used only by the live graceful owner.
- Register plugin-command run/import with `admin` visibility class and pin the non-admin exclusion through every shared read/delivery seam.
- Preserve `PluginCommandRun`/`PluginCommandImport` as Kind-specific authoritative detail during compatibility; canonical lifecycle becomes the shared control-plane authority and must transition in the same store transactions.

**Verify:**

```bash
go test -race --tags 'json1 fts5' ./plugin_commands ./application_context -run 'Test.*(PluginCommand|CommandRun|CommandImport|RuntimeFence|Quarantine|Recovery)' -count=10
go test --tags 'json1 fts5 postgres' ./application_context ./server/api_tests -run 'Test.*PluginCommand' -count=1
```

**Commit:** `feat(jobs): integrate fenced plugin commands`

---

### Task 11: Build resumable backfill and advance the preinstalled writer epoch

**Files:**
- Create: `application_context/job_migration.go`, `job_migration_test.go`, `job_migration_pg_test.go`.
- Modify: `models/job_model.go`, `main.go`.
- Add old-schema fixtures under `application_context/testdata/job-migration/`.
- Modify: `download_history_context.go`, `scheduled_download_context.go`, plugin-command/reduction stores for source revision tracking.

**Migration state:**

- `JobSourceMapping`: `(source_kind, source_id)` → canonical Job ID, source revision/hash, copy verification, scrub state, purge marker.
- `JobMigrationCheckpoint`: phase/cursor/error for bounded resumable backfill.
- `JobWriterEpoch` already exists and is enforced before process construction from Task 1; this task adds the coordinated advancement from dual-publisher epoch to retirement epoch.

**Red:**

- Backfill download history, scheduled downloads, command runs/imports, and provable Reduction executions idempotently; preserve timestamps/owners/outcomes/legacy IDs without inventing attempts/events/links.
- A download row with `attempts>1` becomes one Job with a migration-note event.
- Re-running after a crash at every phase boundary neither duplicates Jobs nor loses source input.
- A purge marker always wins: backfill cannot recreate a forgotten/expired envelope from legacy payload/URL.
- New/changed source rows written during the initial copy are detected by revision/hash and reverified after drain.
- Re-run Task 1's actual Release-A-artifact test against an advanced database: it refuses before migrations, context construction, writes, cleanup, or dispatch. The retirement release raises the epoch only after every process is Release A or newer and drained. Pre-Release-A binaries are explicitly unsupported across this boundary and must be absent before the gate advances.
- Once retirement epoch is active, database constraints/triggers reject recreation of nonempty legacy payload and raw secret-bearing URL fields even if a compatibility writer is accidentally called.

**Green:**

- Run bounded migration after AutoMigrate and key initialization, before Job dispatch/plugin schedulers.
- Separate `copy`, `verify`, `drain/fence`, `scrub`, and `complete` states. Never infer scrub from copy.
- Record source hashes before/after drain. Scrub only after decrypt-and-compare verification of every execution-required field.
- Compatibility readers stop consulting legacy payload/URL before scrub begins.

**Verify:**

```bash
go test --tags 'json1 fts5' ./application_context -run 'TestJobMigration' -count=1
go test --tags 'json1 fts5 postgres' ./application_context -run 'TestJobMigration' -count=1
```

**Commit:** `feat(jobs): backfill durable legacy work`

---

### Task 12: Complete and verify plaintext retirement

**Files:**
- Modify: `application_context/job_migration.go`, `download_history_context.go`, `scheduled_download_context.go`.
- Modify: `models/download_history_model.go`, `scheduled_download_model.go` only to mark legacy fields as safe projections during compatibility.
- Create: `internal/arch/job_plaintext_retirement_test.go`.
- Modify: startup diagnostics in `main.go`.

**Red:**

- Seed exact URLs with credentials/query strings and headers in both legacy stores; after retirement, only sanitized display values and canonical references remain, while canonical retry/deferred execution still decrypts complete input.
- Simulate crash after verified copy, after writer fence, during each scrub batch, and before completion marker.
- Forget and replay expiry remove canonical envelope plus every remaining legacy replay copy in one transaction and set durable purge marker.
- A restored pre-retirement backup must rerun the barrier; no completion marker is trusted without source verification.
- Startup refuses external cutover readiness if any retained source is unverified/unscrubbed, any old writer epoch is active, or a nonterminal source lacks decryptable complete input.
- Architecture/static test rejects production reads of `DownloadHistoryEntry.Payload/URL` or `ScheduledDownload.Payload/URL` for replay after retirement; allowed uses are migration and sanitized compatibility projection only.

**Green:**

- Replace raw URL columns with sanitized display URLs (no userinfo/query/fragment) and clear payload bytes.
- Make canonical envelopes the only execution/reconciliation/retry reader.
- Emit an admin-visible readiness report with counts by phase and blockers, but never raw values.

**Verify:**

```bash
go test --tags 'json1 fts5' ./application_context ./internal/arch -run 'Test.*(Plaintext|Retirement|ReplaySource)' -count=1
go test --tags 'json1 fts5 postgres' ./application_context -run 'Test.*(Plaintext|Retirement)' -count=1
```

**Commit:** `feat(jobs): retire plaintext legacy replay data`

---

### Task 13: Add canonical Job HTTP, output access, and resumable SSE

**Files:**
- Create: `server/api_handlers/job_handlers.go`, `job_command_handlers.go`, `job_event_handlers.go`, and tests.
- Modify: `server/routes.go`, `server/routes_openapi.go`, `server/openapi/*` tests.
- Modify: `application_context/job_context.go`.

**Routes:**

- `GET /v1/jobs`
- `GET /v1/jobs/{id}`
- `GET /v1/jobs/{id}/events`
- `GET /v1/jobs/events?version=2`
- `POST /v1/jobs/{id}/commands/{command}`
- `POST /v1/jobs/commands/{command}`
- `GET /v1/jobs/summary`
- typed output routes resolved from advertised outputs (artifact/report/entity links never expose filesystem paths)

**Red:**

- Cursor pagination/filter/search/summary responses match the Service and reject invalid/oversized windows.
- Detail includes commands computed at its returned Job version and only authorized outputs/visible lineage.
- Command requires/accepts idempotency key, returns per-Job bulk outcomes, returns 409 with fresh snapshot on stale visible state, and 404 for hidden ID.
- Output open reauthorizes current role/scope and checks availability independently of Job visibility.
- SSE catch-up from `Last-Event-ID` or cursor streams every later visible **published** durable event, then live notifications; reconnect duplicates are tolerable and delivery-sequence-monotonic. An event committed after the client's current high-water mark but published later receives a later delivery sequence and cannot be skipped.
- Cross-version/unversioned cursor on `version=2` is rejected. Slow subscribers lose wake-ups, not durable events.
- SQLite polling and PostgreSQL wake strategy produce equivalent visible sequences.

**Green:**

- Keep wake channels/`LISTEN NOTIFY` optional. The post-commit publisher from Task 1 assigns commit-safe `delivery_sequence` values; query `job_events.delivery_sequence > cursor` as correctness. Never use auto-increment/sequence allocation order as commit order.
- Add heartbeat comments only, not fake Job events.
- Bound catch-up pages and response sizes.
- Generate OpenAPI from the same route metadata and include command/idempotency/conflict schemas.

**Verify:**

```bash
go test --tags 'json1 fts5' ./server/api_handlers ./server/openapi ./server/api_tests -run 'Test.*Job' -count=1
go test --tags 'json1 fts5 postgres' ./server/api_tests -run 'Test.*Job' -count=1
go run ./cmd/openapi-gen -output /tmp/job-center-openapi.yaml
go run ./cmd/openapi-gen/validate.go /tmp/job-center-openapi.yaml
```

**Commit:** `feat(api): add canonical job endpoints`

---

### Task 14: Preserve legacy handles and response/event representations

**Files:**
- Modify: `application_context/job_compatibility.go`, `job_compatibility_test.go` created in Task 7.
- Modify: `models/job_model.go`, `jobs/commands.go`.
- Modify: `server/api_handlers/download_queue_handlers.go`, `download_history_handlers.go`, `action_handlers.go` compatibility paths and tests.
- Modify: `server/routes.go`, `routes_openapi.go`.

**Prerequisite already delivered in Release A:** Task 7 creates compatibility handles and routes every existing get/list/queue/event/control/Retry path through them before dual publication can deploy. This task completes the public compatibility contract (all projections, additive fields, deprecation headers, filter translation, and OpenAPI); it must not be the first point at which legacy Retry becomes immutable.

**Red:**

- Every migrated/current legacy ID resolves through a durable compatibility handle to the current linear Retry leaf.
- Legacy get/list/queue/events/control keep `id=<handle>` and add `canonicalJobId`; they expose one projection per handle, never relabel ancestors.
- Legacy Retry preserves `{"status":"retrying"}`, creates new Job/link/command result, and advances the handle atomically. Canonical Retry of the same leaf advances its handle too.
- Successive retries keep the same handle; active leaf and succeeded leaf refuse Retry.
- Resolution+authorization+command validation is atomic against handle movement; a racing cancel cannot hit the wrong leaf.
- Hidden successor is missing, not replaced by visible ancestor.
- Keyed repeated legacy request returns its original result even if handle has since advanced; unkeyed requests keep old state-based behavior.
- Unversioned `/v1/jobs/events` and `/v1/download/events` keep legacy events/IDs. `version=2` never emits handles. Cursors cannot cross.
- Every legacy response carries `Deprecation`, `Sunset` (at least six months/documented release), and `Link` headers. `/downloads` redirects to translated `/jobs?kind=remote-download` filters.

**Green:**

- Add `JobCompatibilityHandle` unique `(namespace,handle)` with current Job ID/version.
- Implement legacy projection in one adapter, not separate handler conditionals.
- Keep submission domain routes but include both legacy handle and canonical Job ID where their response contract permits additive fields.

**Verify:**

```bash
go test -race --tags 'json1 fts5' ./jobs ./application_context ./server/api_handlers ./server/api_tests -run 'Test.*(Legacy|Compatibility|Handle|DownloadRoute|JobsEvents)' -count=10
go test --tags 'json1 fts5 postgres' ./application_context ./server/api_tests -run 'Test.*(Legacy|Compatibility|Handle)' -count=1
```

**Commit:** `feat(jobs): preserve legacy job handles`

---

### Task 15: Build the Job Center and shared live panel

**Files:**
- Create: `server/template_handlers/template_context_providers/job_template_context.go` and tests.
- Create: `templates/listJobs.tpl`, `templates/displayJob.tpl`, `templates/partials/jobPanel.tpl`.
- Create: `src/components/jobCenter.js`, `src/components/jobPanel.js` and Vitest suites.
- Modify: `src/main.js`, `templates/layouts/base.tpl`, navigation/header partials, `server/routes.go`.
- Remove only at Task 17 cutover: `templates/listDownloads.tpl`, `templates/partials/downloadCockpit.tpl`, `src/components/downloadsManager.js`, `src/components/downloadCockpit.js`.

**Red:**

- Vitest component fixtures prove rendering uses `commands[]` and `outputs[]` exclusively; inject an unknown Kind with valid commands and verify controls work without source/state conditionals.
- Default sections are Needs attention, Active and scheduled, Recent finished; All view is keyset-paginated newest-first.
- URL round-trips every filter. Bulk commands are the intersection of selected advertised bulk commands and show per-Job outcomes.
- SSE reducer dedupes by event sequence, catches up after reconnect, preserves focus/expanded rows, and moves rows only across state classes.
- Panel shows active count separately from attention count, immediate advertised controls, and “All jobs.”
- Dismiss finished replaces Clear completed. Pin/Forget confirmations accurately describe scope and artifact independence.
- Accessibility tests cover text state, indeterminate progress, meaningful live announcements, modal/focus trap, keyboard shortcut, no color-only meaning, no timeline replay announcements.
- Playwright covers each Kind from submission to detail/output/command, scoped user visibility, admin oversight, hidden lineage, retry successor navigation, reconnect, and mobile panel.

**Green:**

- Reuse existing focus/modality/live-region utilities.
- Fetch canonical `version=2` SSE and canonical detail/list/command endpoints only.
- Preserve current shortcut initially, rename visible chrome to Jobs / Job Center / All jobs, and use a neutral activity icon.
- Rebuild committed JS after each source change; final bundle lands only with the cutover commit.

**Verify:**

```bash
npm run test:unit -- --run
npm run build-js
cd e2e && npm run test:with-server -- tests/jobs tests/accessibility/job-center-a11y.spec.ts
```

**Commit:** `feat(ui): add unified job center`

---

### Task 16: Update CLI, analytics export, OpenAPI, and operator docs

**Files:**
- Modify: `cmd/mr/commands/jobs.go`, `cmd/mr/commands/jobs_help/*.md`, CLI tests.
- Add canonical commands for list/get/timeline/summary/command/bulk command; keep `job submit` as download-domain compatibility or move documentation to the download surface without breaking it.
- Create: a `job-summary-export@1` adapter using the common Job system for >90-day CSV/JSON exports.
- Modify: `docs-site/docs/features/job-system.md`, `download-queue.md`, `export-import.md`, configuration docs, API docs.
- Modify: `CLAUDE.md` configuration and Job lifecycle sections after implementation behavior is proven.

**Red:**

- CLI doctests and E2E prove cursor listing, detail, advertised command invocation with idempotency key, bulk partial outcomes, summaries, and legacy aliases.
- Analytics export is itself a durable admin/user-visible Job, observes identical filters/visibility, and publishes an expiring artifact.
- Documentation freshness tests fail until all new flags/env/runtime settings/routes and deprecations are documented.

**Green:**

- CLI never encodes state→command policy; it displays/invokes server declarations.
- Document replay-key rotation/file permissions, PostgreSQL stable-key requirement, writer-epoch rollout, six-month legacy window, backup restoration barrier, and command-runtime single-owner constraint.

**Verify:**

```bash
go test --tags 'json1 fts5' ./cmd/mr/commands ./server/openapi -count=1
npm run skills-gen
./mr docs lint
./mr docs check-examples
cd e2e && npm run test:with-server:cli
```

**Commit:** `docs(cli): expose the canonical job center`

---

### Task 17: Enforce complete-kind inventory and perform the external cutover

**Files:**
- Create: `internal/arch/job_kind_inventory_test.go`.
- Modify: all old submission sites identified by `rg 'SubmitJob|SubmitManagedJob|RunActionAsync|mah.start_job|ScheduledDownload|PluginCommandRun|ComputeJobID'` until inventory passes.
- Modify: `server/routes.go`, templates, `src/main.js`.
- Delete obsolete UI files listed in Task 15 after routes redirect/project correctly.
- Modify: `docs/todo.md` with task/verification evidence.

**Red gate:**

`job_kind_inventory_test.go` enumerates every user/operator-facing submission source and requires a registered adapter plus fixture proof. It fails on an unknown `download_queue.JobOptions.Source`, plugin action path, command/import submission, scheduled occurrence, or user-facing goroutine. The explicit expected inventory is:

- remote download and deferred download;
- group export;
- group import parse and apply;
- Resource Reduction compute;
- every current user-facing maintenance Job (including similarity recompute);
- registered plugin action, scheduled plugin occurrence, and closure-backed `mah.start_job`;
- plugin command run and plugin command import;
- Job summary export.

**Cutover checklist (all required):**

- [ ] Every inventory Kind dual-publishes and recovery tests pass.
- [ ] Backfill is complete and idempotent on SQLite/PostgreSQL fixtures.
- [ ] All nonterminal required input decrypts; all retained legacy plaintext is scrubbed.
- [ ] Minimum writer epoch is raised after old processes drain; rollback procedure uses a compatible canonical reader, never a plaintext writer.
- [ ] Forget/purge cannot be undone by migration or compatibility routes.
- [ ] Canonical APIs, resumable SSE, commands, outputs, summaries, and visibility are complete.
- [ ] Legacy handles and unversioned SSE pass successive-Retry/race tests.
- [ ] Plugin-command DB fence + staging lease prove one quiescent owner.
- [ ] Closure-backed runtime-loss behavior is interrupted-or-blocked exactly as proof permits, never redispatched.
- [ ] Job Center and panel include every Kind and contain no source/status control inference.
- [ ] Existing domain submission endpoints still work.

**Green:**

- Switch navigation/page/panel to `/jobs` and canonical APIs in one commit.
- Redirect `/downloads` with recognized filter translation.
- Stop old registries from being lifecycle authorities; retain only compatibility projections needed during the window.
- Do not drop compatibility tables/columns yet; they are scrubbed safe projections.

**Verify focused gate:**

```bash
go test --tags 'json1 fts5' ./internal/arch ./jobs ./application_context ./download_queue ./plugin_system ./plugin_commands ./server/api_handlers ./server/api_tests -count=1
go test --tags 'json1 fts5 postgres' ./jobs ./application_context ./server/api_tests -count=1
npm run test:unit -- --run
npm run build
./scripts/css-scan-test.sh
go vet --tags 'json1 fts5' ./...
git diff --check -- . ':(exclude)public/dist/main.js'
```

**Commit:** `feat(jobs): cut over every background job kind`

---

### Task 18: Run crash, race, migration, browser, and CLI release verification

**Files:**
- Add/modify only tests, fixtures, and documentation defects exposed by this gate.
- Update: `docs/todo.md` Review section with exact commands/outcomes and residual risks.

**Fault matrix:**

- kill after acceptance/before dispatch;
- kill after claim/before executor start;
- kill while running with external work alive;
- stale heartbeat with owner alive;
- stale execution token publishing progress/output/terminal state;
- cancellation vs success and pause vs completion;
- two simultaneous Retry commands, keyed and unkeyed;
- two processes claiming capacity;
- command-runtime takeover while PGID/runtime lease is unresolved;
- callback runtime loss proved/unproved;
- migration crash at copy/verify/fence/scrub/complete;
- key rotation/missing old key;
- replay expiry/Forget concurrent with Retry;
- output expiry concurrent with download;
- user demotion/deletion/scope change between acceptance and command/dispatch;
- SSE disconnect between catch-up and live wake;
- million-row list/summary query plans on both engines.

**Security probes:**

- Exact secret corpus absent from DB summary/event/output/command-result/search columns, HTTP responses, application logs, SSE, rendered HTML, CLI output, and generated diagnostics.
- Hidden Job/lineage/output/event/summary probes all produce no existence signal.
- Compatibility handle gives no authority and never resolves to a visible ancestor when leaf is hidden.
- Artifact paths cannot escape rooted storage; safe external links pass adapter allowlisting.

**Final commands:**

```bash
gofmt -w $(git diff --name-only -- '*.go')
git diff --check -- . ':(exclude)public/dist/main.js'

go test -race --tags 'json1 fts5' ./jobs ./application_context ./download_queue ./plugin_system ./plugin_commands ./server/api_handlers ./server/api_tests -count=3
go test --tags 'json1 fts5' ./... -count=1
go vet --tags 'json1 fts5' ./...

go test --tags 'json1 fts5 postgres' ./jobs ./application_context ./mrql/... ./server/api_tests/... -count=1

npm run test:unit -- --run
npm run build
./scripts/css-scan-test.sh

cd e2e && npm run test:with-server:all
cd e2e && npm run test:with-server:postgres

./mr docs lint
./mr docs check-examples
```

Capture query plans for Job list/summary/retention/claim queries on SQLite and PostgreSQL fixtures large enough to reveal table scans. Confirm indexes are used and no replay/event payload scan enters interactive analytics.

**Independent review:**

- Run Standards and Spec reviews from a clean base.
- Require no P0/P1 findings.
- If review changes behavior, add a red regression before the fix and rerun the relevant focused, cross-engine, and end-to-end gates.

**Commit:** `test(jobs): prove unified job center cutover`

---

## Rollout sequence

This plan intentionally spans at least two deployable releases:

1. **Release A — hidden dual publication:** Tasks 1–10. Task 1 already installs and preflight-enforces writer epochs before process construction; Task 7 already bridges legacy Retry/control/event identity through durable handles and immutable successors. Canonical rows exist, all executors publish, and the old UI remains authoritative to users without bypassing canonical lifecycle.
2. **Migration release — copy/verify/drain/fence/scrub:** Tasks 11–12. All running Release-A processes understand the fence before it advances. Pre-Release-A processes are drained and may not be restarted against the advanced database.
3. **Cutover release:** Tasks 13–17. Canonical API/UI becomes public only after complete Kind inventory and retirement readiness pass.
4. **Compatibility window:** Legacy handles/routes/projections remain for at least one documented release and six months.
5. **Removal release:** Drop scrubbed compatibility columns/registries only after telemetry, migration checkpoints, and support policy prove they are unused. This requires a separate removal plan.

## Residual risks to track during implementation

- Auto-generated SQLite replay-key storage requires a writable persistent application-data root; deployments with a remote DB and ephemeral media storage must provide `JOB_REPLAY_KEY` explicitly.
- A non-restorable callback whose originating host cannot be proved dead may remain blocked indefinitely; the first release deliberately prefers operator-visible blockage to duplicate execution.
- Plugin-command multi-host execution remains unsupported; the Job Service must not make it appear supported merely because other Kinds are multi-process.
- Backups taken before plaintext retirement still contain plaintext and must follow operator backup retention/restore migration rules.
- Durable event volume and aggregate latency need measured limits from production-sized fixtures; do not raise limits speculatively before query-plan evidence.
