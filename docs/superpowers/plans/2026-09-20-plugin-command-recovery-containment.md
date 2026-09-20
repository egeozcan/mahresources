# Plugin Command Recovery Containment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep the HTTP application available when plugin-command recovery or runtime-lease ownership is unresolved, while preserving non-publication guarantees, self-healing safely, and preventing stuck process groups from consuming unbounded CPU.

**Architecture:** A host-internal boot-session UUID is persisted atomically with each PGID and consumed only by a recovery-specific record. Recovery aggregates safety blockers without settling them, while an application-owned runtime controller retains/acquires the staging lease, retries quarantine states, atomically publishes the active dispatcher/exchange pair, and logs operator-visible state changes. Linux/Darwin retain zombie-aware liveness, live workers use a bounded second signal plus backed-off polling, and administrator cancellation reports quarantine rather than mutating work owned by another runtime.

**Tech Stack:** Go 1.25, GORM, SQLite/PostgreSQL, `golang.org/x/sys/unix`, Pongo2, Lua via gopher-lua, Playwright, Docker/PostgreSQL test harness.

**Spec:** `docs/superpowers/specs/2026-09-20-plugin-command-recovery-containment-design.md`

## Global Constraints

- Work only in `/Users/egecan/Code/mahresources/.worktrees/plugin-commands` on `feature/plugin-commands`.
- Use strict RED/GREEN evidence for every production behavior change and commit each task independently.
- Preserve shell-free, argv-preserving, generation-bound, consented execution independent of ambient `PATH`.
- Preserve `ActorlessAtSubmission` and the public `AddResource(contracts.File, string, *query_models.ResourceCreator)` signature.
- A different-boot shortcut is permitted only for host-unique boot-session UUIDs: Linux `boot_id` and Darwin `kern.bootsessionuuid`; never use `kern.boottime`.
- Boot identity is persisted in the same conditional statement as PGID and never appears in `RunRecord`, `RunView`, JSON/admin history, templates, or Lua.
- A live or possibly live recovery group remains nonterminal; quarantine never writes a terminal result or importable output for it.
- One staging root has at most one lease owner; an acquired lease spans recovery, quarantine, activation, dispatcher drain, and shutdown.
- Linux/Darwin all-zombie groups are dead even when `kill(-pgid, 0)` reports the PGID present.
- Recovery signals an identity-verified group at most once per server process; a live runner may signal its created group at most twice, with the second attempt requiring an immediately preceding alive observation.
- Quarantine never writes `cancel_requested`; another lease holder may own the process that must be stopped.
- Keep one writer. Independent reviewers are read-only and use fresh context.
- Use only `openai-codex/gpt-5.6-sol` if the operator authorizes subagent execution or review.
- Known unrelated baseline failures remain `plugin_system.TestBundledPluginLiteralURLsAreDeclared` and `server/api_tests.TestSidebar_IsWrappedInADisclosure`; do not fix or mask them.
- Report SQLite/CGO cross-compilation limitations rather than weakening behavior.

---

### Task 1: Pair a private boot-session UUID with PGID persistence

**Files:**
- Create: `plugin_commands/boot_session_linux.go`
- Create: `plugin_commands/boot_session_darwin.go`
- Create: `plugin_commands/boot_session_other.go`
- Create: `plugin_commands/boot_session_test.go`
- Create: `plugin_commands/boot_session_darwin_test.go`
- Modify: `models/plugin_command_run_model.go`
- Modify: `plugin_commands/types.go`
- Modify: `plugin_commands/store.go`
- Modify: `plugin_commands/usage.go`
- Modify: `plugin_commands/runner_unix.go`
- Modify: `plugin_commands/recovery.go`
- Modify: `plugin_commands/runner_test.go`
- Modify: `plugin_commands/dispatcher_test.go`
- Modify: `plugin_commands/disable_race_test.go`
- Modify: `plugin_commands/recovery_test.go`
- Modify: `application_context/plugin_command_store.go`
- Modify: `application_context/plugin_command_store_test.go`
- Modify: `application_context/plugin_command_store_pg_test.go`
- Modify: `application_context/plugin_command_disable_test.go`
- Modify: `application_context/plugin_command_recovery_unix_test.go`
- Modify: `plugin_system/fs_api_test.go`
- Modify: `server/api_tests/plugin_command_history_test.go`

**Interfaces:**
- Produces: `plugin_commands.CurrentBootSessionID() (string, error)`.
- Produces: `plugin_commands.ErrBootSessionIDUnavailable` for targets without a reliable host-unique UUID.
- Produces: `RecoveryRun { RunRecord; BootSessionID string }`, used only by `Store.NonterminalRuns() ([]RecoveryRun, error)`.
- Changes: `Store.SetRunProcessGroup(id string, pgid int, bootSessionID string) error`.
- Adds: `BootSessionID string` to `RunnerDependencies` and `Dependencies`; it is immutable for one runtime.
- Keeps: `RunRecord` and `RunView` free of boot identity.

- [ ] **Step 1: Write failing platform identity tests**

Add tests that require Linux parsing to trim whitespace and Darwin to use the UUID sysctl:

```go
func TestNormalizeBootSessionIDTrimsAndRejectsEmpty(t *testing.T) {
    got, err := normalizeBootSessionID(" 45D21A7C-39C3-4CBC-B5B9-F3C23F9680DC\n")
    if err != nil || got != "45D21A7C-39C3-4CBC-B5B9-F3C23F9680DC" {
        t.Fatalf("identity = %q, err = %v", got, err)
    }
    if _, err := normalizeBootSessionID(" \n"); err == nil {
        t.Fatal("empty identity accepted")
    }
}
```

In the Darwin-tagged test, compare `CurrentBootSessionID()` to `unix.Sysctl("kern.bootsessionuuid")` and assert it is nonempty. Do not reference `kern.boottime` anywhere in production.

- [ ] **Step 2: Write failing atomic persistence and privacy tests**

Change store tests to call:

```go
require.NoError(t, ctx.SetRunProcessGroup("run-transitions", 4321, "boot-session-a"))
```

Then query `models.PluginCommandRun` directly and assert both `ProcessGroupID` and `BootSessionID` changed in one transition. Assert queued rows contain neither. Add a lost-transition test proving a second call changes neither value.

Add privacy assertions:

```go
encoded, err := json.Marshal(runRecord(modelWithBootID))
require.NoError(t, err)
require.NotContains(t, string(encoded), "boot-session-secret")
```

Also assert the admin history response, rendered history page, and `runViewToLua` contain no boot-session value.

- [ ] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context ./plugin_system ./server/api_tests \
  -run 'Test.*(BootSession|ProcessGroup.*Boot|History.*Boot|RunView.*Boot)' -count=1
```

Expected: compile failures for the new function, recovery shape, model field, and `SetRunProcessGroup` signature.

- [ ] **Step 4: Implement platform identity providers**

Use these exact platform rules:

```go
// linux
func CurrentBootSessionID() (string, error) {
    raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
    if err != nil { return "", fmt.Errorf("read Linux boot session id: %w", err) }
    return normalizeBootSessionID(string(raw))
}

// darwin
func CurrentBootSessionID() (string, error) {
    raw, err := unix.Sysctl("kern.bootsessionuuid")
    if err != nil { return "", fmt.Errorf("read Darwin boot session id: %w", err) }
    return normalizeBootSessionID(raw)
}

// aix || dragonfly || freebsd || netbsd || openbsd || solaris || windows
func CurrentBootSessionID() (string, error) {
    return "", ErrBootSessionIDUnavailable
}
```

Declare `ErrBootSessionIDUnavailable` and `normalizeBootSessionID` in an untagged file so every build target shares validation.

- [ ] **Step 5: Implement the private recovery shape and atomic store update**

Add `BootSessionID string` only to `models.PluginCommandRun`, with `gorm:"size:64"`. Do not copy it in `runRecord` or `runModel`. Add:

```go
type RecoveryRun struct {
    RunRecord
    BootSessionID string
}
```

Map it only in `NonterminalRuns`. Change `SetRunProcessGroup` to one guarded update:

```go
Updates(map[string]any{
    "process_group_id": pgid,
    "boot_session_id": bootSessionID,
})
```

Pass the executor's immutable `RunnerDependencies.BootSessionID` into that method. Update every in-memory test store and fake to the same signature and recovery return type. Until Task 3 consumes the identity, adapt current recovery with `recoverRunning(ctx, run.RunRecord)` so this task compiles without changing recovery behavior.

- [ ] **Step 6: Run GREEN, PostgreSQL transition coverage, and cross-compilation**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context ./plugin_system ./server/api_tests \
  -run 'Test.*(BootSession|ProcessGroup|RunView|CommandHistory)' -count=1
go test --tags 'json1 fts5 postgres' ./application_context -run 'TestPluginCommandRunStartPG' -count=1
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c ./plugin_commands -o /tmp/plugin_commands-linux.test
```

On Darwin, also run:

```bash
go test --tags 'json1 fts5' ./plugin_commands -run 'Test.*Darwin.*BootSession' -count=1
```

- [ ] **Step 7: Commit**

```bash
git add models/plugin_command_run_model.go plugin_commands/boot_session_*.go \
  plugin_commands/types.go plugin_commands/store.go plugin_commands/usage.go \
  plugin_commands/runner_unix.go plugin_commands/recovery.go plugin_commands/*_test.go \
  application_context/plugin_command_store*.go application_context/plugin_command_disable_test.go \
  application_context/plugin_command_recovery_unix_test.go plugin_system/fs_api_test.go \
  server/api_tests/plugin_command_history_test.go
git commit -m "feat: bind command pgids to boot sessions"
```

---

### Task 2: Preserve zombie exclusion in Linux process inspection

**Files:**
- Modify: `plugin_commands/process_linux.go`
- Create: `plugin_commands/process_linux_scan_test.go`
- Modify: `plugin_commands/process_identity_test.go`
- Modify: `plugin_commands/process_darwin_platform_test.go`
- Modify: `plugin_commands/process_other_unix.go`

**Interfaces:**
- Produces: `inspectLinuxGroup(procRoot string, pgid int, runID string, probe func(int) error) (GroupIdentity, error)` for deterministic tests; `nativeProcessInspector` calls it with `/proc` and signal zero.
- Preserves: `ProcessInspector.InspectGroup(pgid, runID)`.
- Classifies: parsed all-zombie target groups as `GroupDead` without signal-zero override.

- [ ] **Step 1: Write failing classifier tests over a temporary proc tree**

Create synthetic `stat`, `status`, and `environ` files and a fake probe. Cover these exact cases:

```go
func TestLinuxInspectorDoesNotReviveZombieOnlyGroupWithSignalZero(t *testing.T) {
    root := fakeProcTree(t, linuxProc{PID: 210, PGID: 210, State: "Z"})
    probes := 0
    got, err := inspectLinuxGroup(root, 210, "run-a", func(int) error {
        probes++
        return nil // Linux signal zero succeeds for a zombie-only group.
    })
    if err != nil || got.State != GroupDead || probes != 0 {
        t.Fatalf("identity=%+v probes=%d err=%v", got, probes, err)
    }
}
```

Also require:

- a malformed `stat` with valid `status` proving another PGID is ignored;
- a live target member with matching environment is `GroupAliveOwned`;
- no observed target plus a successful probe is `GroupAliveUnverified`;
- no observed target plus `ESRCH` is `GroupDead`; and
- a sample that remains unresolved after the bounded retry cannot produce the all-zombie result.

- [ ] **Step 2: Add real zombie-only platform tests**

Start a helper in its own process group that exits immediately but do not call `Wait` until after inspection. Assert `InspectGroup` reports `GroupDead` while signal zero still reports the PGID present. Add the same lifecycle assertion to Darwin, where the existing `SZOMB` scan must remain authoritative.

- [ ] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./plugin_commands \
  -run 'Test(LinuxInspector|NativeProcessInspector.*Zombie|Darwin.*Zombie)' -count=1
```

Expected: compile failure for `inspectLinuxGroup`; after the first scaffold, the new fallback test fails if signal zero overrides the zombie scan.

- [ ] **Step 4: Implement scan classification and secondary state parsing**

Refactor Linux inspection to finish enumeration before deciding. Track live target PIDs, target zombies, and unresolved samples. Retry a failed `stat` read/parse once, then parse `/proc/<pid>/status` (`State` and `NSpgid`) as the secondary source. Apply this order:

```text
any live target member                         -> alive; inspect environments
only target zombies, no unresolved candidate -> GroupDead; do not probe
no target observed                            -> signal-zero probe
unresolved possible target                    -> GroupAliveUnverified or inspection error, never GroupDead
```

Document in `process_other_unix.go` that its probe-only implementation cannot exclude zombie-only groups and may require parent reap/server restart.

- [ ] **Step 5: Run GREEN and repeat lifecycle coverage**

```bash
go test --tags 'json1 fts5' ./plugin_commands \
  -run 'Test(LinuxInspector|NativeProcessInspector|Darwin.*Zombie)' -count=20
go test -race --tags 'json1 fts5' ./plugin_commands \
  -run 'TestNativeProcessInspector.*Zombie' -count=10
```

- [ ] **Step 6: Commit**

```bash
git add plugin_commands/process_linux.go plugin_commands/process_linux_scan_test.go \
  plugin_commands/process_identity_test.go plugin_commands/process_darwin_platform_test.go \
  plugin_commands/process_other_unix.go
git commit -m "fix: keep zombie command groups dead"
```

---

### Task 3: Aggregate recovery blockers and classify prior-boot rows

**Files:**
- Modify: `plugin_commands/dispatcher.go`
- Modify: `plugin_commands/recovery.go`
- Modify: `plugin_commands/recovery_test.go`
- Modify: `application_context/plugin_command_recovery_unix_test.go`

**Interfaces:**
- Produces: `RecoveryBlocker { RunID string; ProcessGroupID int; Reason string }`.
- Produces: `RecoveryBlockedError { Blockers []RecoveryBlocker }`, matched with `errors.As`.
- Consumes: `Dependencies.BootSessionID` and `RecoveryRun.BootSessionID`.
- Preserves: ordinary context, malformed-state, and store errors as non-blockage errors.

- [ ] **Step 1: Write failing boot-direction and cancellation-precedence tests**

Add tests with inspector call counters:

```go
func TestRecoveryDifferentBootNeverTouchesPersistedPGID(t *testing.T) {
    pgid := 42
    store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore(), bootIDs: map[string]string{"old": "boot-a"}}
    store.runs["old"] = RunRecord{ID: "old", Status: RunStatusRunning, ProcessGroupID: &pgid}
    inspector := &recoveryInspector{states: map[int][]GroupIdentity{pgid: {{State: GroupAliveOwned}}}}
    d := NewDispatcher(Dependencies{Store: store, Inspector: inspector, BootSessionID: "boot-b"})
    require.NoError(t, d.Recover(context.Background()))
    require.Empty(t, inspector.inspections)
    require.Empty(t, inspector.kills)
    record, _, err := store.Run("old")
    require.NoError(t, err)
    require.Equal(t, RunStatusInterrupted, record.Status)
    require.True(t, record.OutputUnverified)
}
```

Extend `recoveryStore` with `bootIDs map[string]string` and `recoveryInspector` with an `inspections []int` recorder so the snippet is complete.

Add the unsafe-direction control: equal IDs and either empty ID must call the inspector. Add a cancelled different-boot row and assert `cancelled + output_unverified`.

- [ ] **Step 2: Write failing blocker aggregation and safe-row continuation tests**

Seed two live-unverified rows with a dead row ordered between them. Assert `Recover` returns one `*RecoveryBlockedError` with both run IDs/PGIDs, leaves both blocked rows nonterminal, and finishes the dead row. Add an inspection-error blocker and assert its reason is retained.

Add a healing-scan test where an owned group is signalled once, remains alive through the deadline, then a second `Recover` observes it dead. Assert total `KillGroup` calls remain one.

- [ ] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context \
  -run 'Test.*Recover.*(Boot|Block|Continue|Signal|Unverified)' -count=1
```

Expected: recovery stops at the first blocker, has no typed aggregate, and has no boot-session classification.

- [ ] **Step 4: Implement the typed aggregate and per-process signal latch**

Give `Dispatcher` a mutex-protected `recoverySignals map[string]struct{}` initialized in `NewDispatcher`, plus recovery poll/drain durations defaulted to `groupPollInterval`/`groupDrainTimeout` and shortened directly by package tests. Refactor `Recover` to append blockers and continue:

```go
var blocked []RecoveryBlocker
for _, run := range runs {
    finish, blocker, err := d.recoverRunning(ctx, run)
    if err != nil { return err }
    if blocker != nil {
        blocked = append(blocked, *blocker)
        continue
    }
    // FinishRun only for settled rows.
}
if len(blocked) != 0 {
    return &RecoveryBlockedError{Blockers: blocked}
}
```

Different nonempty boot-session IDs return terminal `output_unverified` before any inspector call. Matching/unknown IDs retain process inspection. Wrap inspection, ownership-change, signal, post-signal inspection, and drain-timeout uncertainty as blockers. Check `ctx.Err()` separately so cancellation remains directly matchable.

Before recovered-group `KillGroup`, atomically claim the run's one signal attempt. A later healing scan may observe death but may not signal that run again in the same server process.

- [ ] **Step 5: Run GREEN and race verification**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context \
  -run 'Test.*Recover' -count=1
go test -race --tags 'json1 fts5' ./plugin_commands ./application_context \
  -run 'Test.*Recover.*(Block|Signal|Boot)' -count=20
```

- [ ] **Step 6: Commit**

```bash
git add plugin_commands/dispatcher.go plugin_commands/recovery.go \
  plugin_commands/recovery_test.go application_context/plugin_command_recovery_unix_test.go
git commit -m "feat: classify command recovery blockers"
```

---

### Task 4: Preserve the runtime-lease busy sentinel across the public boundary

**Files:**
- Create: `plugin_commands/runtime_lease_errors.go`
- Modify: `plugin_commands/runtime_lease_flock.go`
- Modify: `plugin_commands/runtime_lease_aix.go`
- Modify: `plugin_commands/runtime_lease_unix.go`
- Modify: `plugin_commands/runtime_lease_windows.go`
- Modify: `plugin_commands/runtime_lease_test.go`

**Interfaces:**
- Produces: exported `plugin_commands.ErrRuntimeLeaseBusy`.
- Guarantees: `errors.Is(AcquireRuntimeLease(root), ErrRuntimeLeaseBusy)` for actual contention.
- Keeps: invalid root, permission, malformed file, `ENOLCK`, and `EOPNOTSUPP` outside the busy class.

- [ ] **Step 1: Write failing public-boundary tests**

Hold one lease and acquire the same root again in-process:

```go
second, err := AcquireRuntimeLease(root)
if second != nil || !errors.Is(err, ErrRuntimeLeaseBusy) {
    t.Fatalf("second lease = %v, %v; want ErrRuntimeLeaseBusy", second, err)
}
if !strings.Contains(err.Error(), "active runtime") {
    t.Fatalf("busy error lost staging context: %v", err)
}
```

Also assert a relative-root error is not `ErrRuntimeLeaseBusy`.

- [ ] **Step 2: Run RED**

```bash
go test --tags 'json1 fts5' ./plugin_commands -run 'TestRuntimeLease' -count=1
```

Expected: `ErrRuntimeLeaseBusy` is undefined or `errors.Is` is false because the current wrapper discards the sentinel.

- [ ] **Step 3: Implement the exported wrapped sentinel**

Declare the sentinel in the untagged errors file, remove platform-private duplicate sentinels, and preserve it with:

```go
return closeOnError(fmt.Errorf(
    "%w: plugin command staging root %q already has an active runtime",
    ErrRuntimeLeaseBusy, root,
))
```

Keep only `EWOULDBLOCK`/`EAGAIN` mapped to busy.

- [ ] **Step 4: Run GREEN and subprocess coverage**

```bash
go test --tags 'json1 fts5' ./plugin_commands -run 'TestRuntimeLease' -count=20
```

- [ ] **Step 5: Commit**

```bash
git add plugin_commands/runtime_lease_*.go plugin_commands/runtime_lease_test.go
git commit -m "fix: expose command runtime lease contention"
```

---

### Task 5: Introduce the command-runtime controller and heal lease contention

**Files:**
- Create: `application_context/plugin_command_runtime_controller.go`
- Create: `application_context/plugin_command_runtime_controller_test.go`
- Modify: `application_context/context.go`
- Modify: `application_context/plugin_command_runtime.go`
- Modify: `application_context/plugin_state_context.go`
- Modify: `application_context/plugin_command_runtime_test.go`
- Modify: `application_context/plugin_command_lifecycle_test.go`
- Modify: `application_context/plugin_command_disable_test.go`
- Modify: `application_context/plugin_command_history.go`
- Modify: `application_context/plugin_command_recovery_unix_test.go`
- Create: `plugin_commands/runtime_errors.go`
- Create: `plugin_commands/runtime_errors_test.go`

**Interfaces:**
- Produces: `ErrCommandRuntimeQuarantined` and `RuntimeQuarantinedError { Reason string; RetryAfterDuration time.Duration }` in `plugin_commands`; `errors.Is` matches the sentinel and `RetryAfter() time.Duration` exposes retry timing.
- Produces: one application-owned `pluginCommandRuntimeController` shared by shallow context clones.
- Produces: `pluginCommandActive() (*pluginCommandActiveRuntime, error)` as the atomically loaded active `{dispatcher, exchange}` accessor for every command/filesystem operation and plugin-disable drain.
- Consumes: `ErrRuntimeLeaseBusy` from Task 4.

- [ ] **Step 1: Write failing runtime-error, normal-controller, and clone tests**

First pin the typed error contract:

```go
err := &RuntimeQuarantinedError{Reason: "lease busy", RetryAfterDuration: 2 * time.Second}
require.ErrorIs(t, err, ErrCommandRuntimeQuarantined)
require.Equal(t, 2*time.Second, err.RetryAfter())
```

Start the runtime normally and assert the controller is shared by `WithPrincipal`/`WithTransaction` clones, the active snapshot is nonnil, submission uses that snapshot, and `StopPluginCommands` removes availability before draining/releasing.

Replace tests that assign `ctx.pluginCommandDispatcher`, `ctx.pluginCommandExchange`, or `ctx.pluginCommandLease` directly with one test helper that installs an active controller snapshot. Preserve `SetPluginCommandDispatcher` as a test seam by making it update the controller, not a separate field.

- [ ] **Step 2: Write failing lease-contention healing tests**

Use a real first lease, then start a second context with injected short delays:

```go
cfg := defaultPluginCommandControllerConfig()
cfg.acquireBackoff = []time.Duration{5 * time.Millisecond, 10 * time.Millisecond}
require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), settings, cfg))
_, activeErr := ctx.pluginCommandActive()
require.ErrorIs(t, activeErr, plugin_commands.ErrCommandRuntimeQuarantined)
require.NoError(t, first.Close())
require.Eventually(t, func() bool {
    _, err := ctx.pluginCommandActive()
    return err == nil
}, time.Second, 5*time.Millisecond)
```

Assert acquisition attempts follow the injected sequence, a stop before release prevents later publication, and a non-busy lease error remains a startup error containing `-plugins-disabled`. Query `models.LogEntry` to prove lease quarantine writes one warning naming the staging root, automatic retry, and `-plugins-disabled`, then one informational entry on activation.

- [ ] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./application_context \
  -run 'TestPluginCommand.*(Controller|LeaseContention|Clone|StopBeforeActivation)' -count=1
```

Expected: lease contention still fails startup and no controller exists.

- [ ] **Step 4: Implement the controller state and active snapshot**

Use a focused controller file with these responsibilities:

```go
type pluginCommandActiveRuntime struct {
    dispatcher *plugin_commands.Dispatcher
    exchange   plugin_commands.Exchange
}

type pluginCommandRuntimeController struct {
    owner    *MahresourcesContext
    settings plugin_commands.Settings
    active   atomic.Pointer[pluginCommandActiveRuntime]
    mu       sync.Mutex
    state    pluginCommandRuntimeState
    reason   string
    retryAt  time.Time
    lease    *plugin_commands.RuntimeLease
    pending  *plugin_commands.Dispatcher
    cancel   context.CancelFunc
    done     chan struct{}
}
```

The injected config supplies `acquireLease`, `bootSessionID`, acquiring delays, recovery interval, inspector, and test barriers. Production defaults are 1s, 2s, 5s, 10s, 30s, 1m, 5m and a five-minute cap. Implement the typed error exactly as a wrapper around `ErrCommandRuntimeQuarantined`, with `RetryAfter() time.Duration` returning `RetryAfterDuration`.

`StartPluginCommands` installs exactly one controller, resolves boot identity once (logging and using empty identity on lookup failure), attempts synchronous acquisition, and starts one retry goroutine only for `ErrRuntimeLeaseBusy`. Lease quarantine and later activation are mirrored to stdout and `/logs` with entity type `plugin_command`. On acquisition, build the usage cache/executor/dispatcher/exchange with the same boot-session ID. Publish active state before setting the plugin manager's atomic submitter/mediator. Non-busy acquisition errors clear the controller and return an error naming `-plugins-disabled`.

- [ ] **Step 5: Implement stop and all runtime accessors through the controller**

Every `SubmitPluginCommand`, `CommandRuns`, list/read/import/discard operation, admin cancellation, and plugin-disable drain calls `pluginCommandActive()`. An unavailable snapshot returns `RuntimeQuarantinedError`; it never falls back to stale fields. Stop marks the controller stopping, cancels/joins retry, atomically clears active state, drains the dispatcher if present, and releases the lease only when `RuntimeLeaseReleasable` is true.

- [ ] **Step 6: Run GREEN and race verification**

```bash
go test --tags 'json1 fts5' ./application_context \
  -run 'TestPluginCommand.*(Controller|Lease|Clone|Shutdown|Disabled|Stop)' -count=1
go test -race --tags 'json1 fts5' ./application_context \
  -run 'TestPluginCommand.*(LeaseContention|StopBeforeActivation|Shutdown)' -count=20
```

- [ ] **Step 7: Commit**

```bash
git add application_context/context.go application_context/plugin_command_runtime*.go \
  application_context/plugin_state_context.go application_context/plugin_command_lifecycle_test.go \
  application_context/plugin_command_disable_test.go application_context/plugin_command_history.go \
  application_context/plugin_command_recovery_unix_test.go plugin_commands/runtime_errors*.go
git commit -m "feat: supervise plugin command runtime activation"
```

---

### Task 6: Quarantine blocked recovery, log it, and activate after healing

**Files:**
- Modify: `application_context/plugin_command_runtime_controller.go`
- Modify: `application_context/plugin_command_runtime.go`
- Modify: `application_context/plugin_command_runtime_controller_test.go`
- Modify: `application_context/plugin_command_recovery_unix_test.go`
- Modify: `application_context/plugin_command_lifecycle_test.go`
- Modify: `plugin_system/commands_api.go`
- Modify: `plugin_system/commands_api_test.go`
- Modify: `plugin_system/fs_api.go`
- Modify: `plugin_system/fs_api_test.go`

**Interfaces:**
- Consumes: `*RecoveryBlockedError` from Task 3.
- Produces: recovery quarantine that retains lease plus unstarted dispatcher and retries every five minutes (injectable in tests).
- Produces: stdout and `/logs` records with entity type `plugin_command`, full blocker details, deduped retry failures, and one healing event.

- [ ] **Step 1: Write failing recovery-quarantine lifecycle tests**

Inject an inspector that returns two blockers while a third row is dead. Assert initial startup returns nil, safe rows settle, blockers remain running, the lease cannot be reacquired, and all command/filesystem methods return `ErrCommandRuntimeQuarantined`.

Advance the injected retry interval after changing both blockers to `GroupDead`; assert the same dispatcher starts, command/filesystem operations become available without plugin reload, and the lease remains continuously held.

- [ ] **Step 2: Write failing logging and retry-error tests**

Query `models.LogEntry` and assert one quarantine warning includes both run IDs, PGIDs, reasons, `/logs` guidance, the process-group termination action, and `-plugins-disabled`. Make two retries fail with the same database error and assert one deduped retry warning. Heal and assert one informational entry.

Add a retry-time infrastructure failure test proving the already-serving process stays quarantined rather than exiting or releasing its acquired lease.

- [ ] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./application_context ./plugin_system \
  -run 'Test.*Command.*(RecoveryQuarantine|Healing|QuarantineLog|RetryFailure|HostUnavailable)' -count=1
```

Expected: `RecoveryBlockedError` still propagates as startup failure and no healing/log lifecycle exists.

- [ ] **Step 4: Implement recovery quarantine and deduped logging**

When synchronous `Recover` returns `*RecoveryBlockedError`, retain lease and dispatcher, set state/reason/retry deadline, log the complete blocker set, launch the retry loop, and return nil. A retry calls `Recover` on the same dispatcher so its per-process signal latch survives. On success call `Start`, construct/publish the exchange pair, and emit one healing log.

Use `ctx.Logger().Warning/Info` with action `"system"`, entity type `"plugin_command"`, and JSON details for blockers. Always mirror to `log.Printf`; DB-log failure must not alter lifecycle state. Deduplicate repeated state/reason warnings.

Update nil-host Lua errors from “until startup recovery completes” to accurate wording: the command runtime is unavailable, quarantined recovery retries automatically when applicable, and details are in `/logs`.

- [ ] **Step 5: Run GREEN and stop/heal race verification**

```bash
go test --tags 'json1 fts5' ./application_context ./plugin_system \
  -run 'Test.*Command.*(Recovery|Quarantine|Healing|HostUnavailable)' -count=1
go test -race --tags 'json1 fts5' ./application_context \
  -run 'TestPluginCommand.*(RecoveryQuarantine|Stop.*Healing|Retry)' -count=20
```

- [ ] **Step 6: Commit**

```bash
git add application_context/plugin_command_runtime*.go \
  application_context/plugin_command_recovery_unix_test.go \
  application_context/plugin_command_lifecycle_test.go \
  plugin_system/commands_api.go plugin_system/commands_api_test.go \
  plugin_system/fs_api.go plugin_system/fs_api_test.go
git commit -m "feat: heal quarantined command recovery"
```

---

### Task 7: Make administrator cancellation accurate during quarantine

**Files:**
- Modify: `application_context/plugin_command_history.go`
- Modify: `application_context/plugin_command_history_test.go`
- Modify: `server/api_handlers/plugin_command_handlers.go`
- Modify: `server/api_handlers/plugin_command_handlers_test.go`
- Modify: `server/template_handlers/template_context_providers/context_interfaces.go`
- Modify: `server/template_handlers/template_context_providers/plugin_command_history_context.go`
- Modify: `templates/pluginCommandHistory.tpl`
- Modify: `server/api_tests/plugin_command_history_test.go`
- Modify if generated: `public/tailwind.css`

**Interfaces:**
- Produces: `PluginCommandRuntimeAvailability() (available bool, reason string)` on `MahresourcesContext` and the history page interface.
- Consumes: `ErrCommandRuntimeQuarantined`/`RuntimeQuarantinedError` from Task 5.
- Maps: quarantined cancellation to HTTP 503 and `Retry-After` when positive.

- [ ] **Step 1: Write failing context and API tests**

With a queued durable row and a quarantined controller, assert:

```go
err := ctx.CancelPluginCommandRun(runID)
require.ErrorIs(t, err, plugin_commands.ErrCommandRuntimeQuarantined)
record, _, _ := ctx.Run(runID)
require.False(t, record.CancelRequested)
```

Add handler cases expecting 503, an actionable response body, and a positive `Retry-After` for a typed error carrying retry duration. Preserve 404 for a missing run only when an active dispatcher can authoritatively answer it.

- [ ] **Step 2: Write failing accessible-render tests**

Assert the history page exposes a visible quarantine notice, renders one disabled control for each queued row, includes the reason through text/`aria-describedby`, and contains no enabled cancellation form. Keep the exact active-state accessible name `Cancel queued run <run-id>` and the selected-row duplicate replacement `Shown above`.

- [ ] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./application_context ./server/api_handlers ./server/api_tests \
  -run 'Test.*PluginCommand.*(Cancel|Quarantin|History)' -count=1
```

Expected: cancellation returns not-found and the template still posts an actionable form.

- [ ] **Step 4: Implement the truthful mutation and presentation boundary**

`CancelPluginCommandRun` must load the active controller snapshot and return its typed quarantine error without touching the store when unavailable. Add availability/reason to the template context. Render a non-submit disabled button plus visible reason while quarantined; render the existing form only when active.

Map `ErrCommandRuntimeQuarantined` to 503. If `errors.As` finds an interface exposing `RetryAfter() time.Duration`, round up to whole seconds and set the header.

- [ ] **Step 5: Run GREEN, CSS scan, and accessibility coverage**

```bash
go test --tags 'json1 fts5' ./application_context ./server/api_handlers ./server/api_tests \
  -run 'Test.*PluginCommand.*(Cancel|Quarantin|History)' -count=1
npm run build-css
./scripts/css-scan-test.sh
```

Retain `public/tailwind.css` if the template introduces generated classes.

- [ ] **Step 6: Commit**

```bash
git add application_context/plugin_command_history*.go server/api_handlers/plugin_command_handlers*.go \
  server/template_handlers/template_context_providers/context_interfaces.go \
  server/template_handlers/template_context_providers/plugin_command_history_context.go \
  templates/pluginCommandHistory.tpl server/api_tests/plugin_command_history_test.go \
  public/tailwind.css
git commit -m "fix: disable command cancellation during quarantine"
```

---

### Task 8: Bound live re-signalling, back off polling, and audit pinned capacity

**Files:**
- Modify: `plugin_commands/usage.go`
- Modify: `plugin_commands/runner_unix.go`
- Modify: `plugin_commands/runner_test.go`
- Modify: `plugin_commands/process_darwin_platform_test.go`
- Modify: `application_context/plugin_command_runtime_controller.go`
- Modify: `application_context/plugin_command_lifecycle_test.go`

**Interfaces:**
- Produces: `RuntimeWarning { Event string; Message string; RunID string; ProcessGroupID int; ActiveLimit int }` and a dedicated `Warn func(RuntimeWarning)` dependency.
- Uses: `maxActiveCommands` directly for `ActiveLimit`; application wording never hardcodes four.
- Preserves: first live signal from creation-time authority.

- [ ] **Step 1: Write failing bounded-signal tests**

Use a fake inspector that remains `GroupAliveUnverified` after the first signal and becomes dead after the second. Assert terminal publication occurs and exactly two kills happen. Add the same test for `GroupAliveOwned`. Add an inspection-error case that remains blocked and never permits the second signal. Add a long-lived case proving no third signal occurs across later cleanup deadlines.

- [ ] **Step 2: Write failing poll-backoff and warning tests**

Inject executor timings (`pollInterval=5ms`, `stuckPollInterval=40ms`, `cleanupTimeout=20ms`, `stuckWarningAfter=80ms`). Record inspection timestamps and assert high-frequency polling stops after forced cleanup, later intervals are at least the injected slow interval minus scheduler tolerance, and exactly one warning contains run ID, PGID, and `ActiveLimit == maxActiveCommands`.

At application level, assert that warning creates one `/logs` warning explaining that a global command slot is pinned.

- [ ] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context \
  -run 'Test.*(Resignal|ReSignal|PollBackoff|Pinned.*Slot|Stuck.*Warning)' -count=1
```

Expected: current one-shot latch suppresses the second signal, polling remains at 20 ms, and no durable warning exists.

- [ ] **Step 4: Implement two-attempt live signalling and slow polling**

Replace the boolean with an attempt count. The initial call may signal without inspection. At the first forced-cleanup deadline, allow one final call only when the immediately preceding successful inspection returned `GroupAliveOwned` or `GroupAliveUnverified`. Set the attempt count before calling `KillGroup`; never exceed two.

When forced cleanup starts, reset the ticker to the configured slow interval. Track cleanup start and emit one `RuntimeWarning` after the configured warning duration. Continue polling indefinitely until group death/pipes close; neither the warning nor the second signal permits terminal publication by itself.

- [ ] **Step 5: Wire the dedicated application warning sink**

The runtime controller passes a sink that mirrors to stdout and `ctx.Logger().Warning("system", "plugin_command", ...)`. Format total capacity from `warning.ActiveLimit`, not a literal. Do not route unrelated `Logf` messages into `/logs`.

- [ ] **Step 6: Run GREEN, repeat Darwin coverage, and race verification**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context \
  -run 'Test.*(Runner|DarwinRunner|PollBackoff|Pinned.*Slot|Warning)' -count=1
go test -race --tags 'json1 fts5' ./plugin_commands ./application_context \
  -run 'Test.*(Resignal|PollBackoff|Stop.*Runner|Pinned.*Slot)' -count=20
```

- [ ] **Step 7: Commit**

```bash
git add plugin_commands/usage.go plugin_commands/runner_unix.go \
  plugin_commands/runner_test.go plugin_commands/process_darwin_platform_test.go \
  application_context/plugin_command_runtime_controller.go \
  application_context/plugin_command_lifecycle_test.go
git commit -m "fix: bound stuck command group cleanup"
```

---

### Task 9: Align invariants, complete verification, and record evidence

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs-site/docs/configuration/advanced.md`
- Modify: `docs-site/docs/features/plugin-lua-api.md`
- Modify: `docs/superpowers/plans/2026-09-20-plugin-command-runtime-remediation.md`
- Modify: `docs/todo.md`
- Modify: `docs/lessons.md` only if implementation reveals an additional reusable correction pattern

**Interfaces:**
- Documents: private boot-session identity, zombie semantics, lease/recovery quarantine, self-healing cadence, administrator cancellation refusal, `/logs` diagnostics, and operator process-group termination.
- Produces: merge-readiness evidence without hiding accepted unrelated failures.

- [ ] **Step 1: Update invariant-focused documentation**

Keep `CLAUDE.md` concise. Record these exact invariants:

- PGID and host-unique boot-session UUID are persisted together and identity is host-internal.
- Linux/Darwin all-zombie groups are dead; signal zero is only conservative existence evidence.
- Busy lease acquisition uses short capped backoff; recovery blockers retain the lease and retry every five minutes.
- Quarantine leaves rows nonterminal, withholds mutations, reports to `/logs`, and may be healed by terminating the named abandoned process group.
- Recovery gets one verified signal attempt per process; a live creator gets at most two signals and then slow polling.

Update operator/plugin docs to explain the typed unavailable state and automatic recovery rather than promising that startup recovery merely “completes.” Record the probe-only Unix limitation.

- [ ] **Step 2: Run formatting, generation, static checks, and focused repeats**

```bash
git diff --name-only -- '*.go' | xargs gofmt -w
git diff --check
npm run build
./scripts/css-scan-test.sh
go vet --tags 'json1 fts5' ./...
go test -race --tags 'json1 fts5' ./plugin_commands ./application_context ./plugin_system ./server/api_handlers ./server/api_tests \
  -run 'Test.*(PluginCommand|CommandRun|RuntimeLease|Recover|ProcessInspector)' -count=10
```

Expected: every command passes.

- [ ] **Step 3: Run focused PostgreSQL and command-history browser coverage**

```bash
go test --tags 'json1 fts5 postgres' ./application_context ./server/api_tests \
  -run 'Test.*PluginCommand' -count=1
cd e2e && npm run test:with-server -- --grep 'plugin command history'
cd ..
```

Expected: all selected tests pass.

- [ ] **Step 4: Run frontend and complete E2E suites**

```bash
npm run test:unit -- --run
cd e2e && npm run test:with-server:all
cd ..
```

Expected: Vitest and browser/auth/CLI/doctest harnesses pass with only intentional skips.

- [ ] **Step 5: Run aggregate Go verification and classify only accepted baselines**

```bash
go test --tags 'json1 fts5' ./... -count=1
```

Expected: either full pass or only:

```text
plugin_system.TestBundledPluginLiteralURLsAreDeclared
server/api_tests.TestSidebar_IsWrappedInADisclosure
```

Any other failure blocks completion.

- [ ] **Step 6: Run platform compile checks**

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c ./plugin_commands -o /tmp/plugin_commands-linux.test
GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 go test -c ./plugin_commands -o /tmp/plugin_commands-freebsd.test
```

Run native Darwin plugin-command tests when executing on Darwin. If broader cross-compilation fails through the repository's SQLite/CGO dependency, record it rather than changing runtime behavior.

- [ ] **Step 7: Review the final diff and update progress evidence**

```bash
git diff --check
git status --short
git diff --stat 283c2afab0ec2524840199e858a96156ca053599...HEAD
```

Update `docs/todo.md` with task commits, RED/GREEN commands, aggregate results, accepted baselines, and residual probe-only Unix limitation. Amend the remediation plan's review section with this follow-up.

- [ ] **Step 8: Commit documentation and generated assets**

```bash
git add CLAUDE.md docs-site/docs/configuration/advanced.md \
  docs-site/docs/features/plugin-lua-api.md \
  docs/superpowers/plans/2026-09-20-plugin-command-runtime-remediation.md \
  docs/todo.md docs/lessons.md public/tailwind.css
git commit -m "docs: record command recovery containment"
```

- [ ] **Step 9: Request independent closure reviews**

Review the completed diff against the approved spec on two explicit axes:

1. security/correctness/standards, including process identity, zombies, lease ownership, stop/heal races, and log/UI accuracy;
2. approved-spec compliance, including every goal, non-goal, platform limitation, and operator action.

If the operator authorizes delegation, use fresh read-only reviewers with only `openai-codex/gpt-5.6-sol`; otherwise perform both passes directly in the parent session. Fix every actionable finding with focused RED/GREEN evidence, rerun affected checks, and create a final correction commit. Do not push without explicit authorization.
