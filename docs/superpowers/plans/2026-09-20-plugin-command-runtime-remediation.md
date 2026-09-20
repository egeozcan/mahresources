# Plugin Command Runtime Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the process-group, import draining, quota, retention-scale, admission-latency, and command-history accessibility defects found in the completed plugin-command branch review.

**Architecture:** Live command workers use creation-time authority to terminate their own process groups and publish terminal state only after observable group death; restart recovery retains stronger identity verification and refuses to settle live unverifiable groups. Imports use AddResource's single managed scratch copy, delete admitted sources after durable success, represent cleanup state separately from import errors, and reserve a true two-copy peak. Durable exchange-removal markers bound sweep work, while cached staging usage removes filesystem traversal from the Lua admission path.

**Tech Stack:** Go 1.25, GORM, SQLite/PostgreSQL, `golang.org/x/sys/unix`, Pongo2, Lua via gopher-lua, Playwright, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-19-plugin-commands-design.md`

## Global Constraints

- Work only in `/Users/egecan/Code/mahresources/.worktrees/plugin-commands` on `feature/plugin-commands`.
- Preserve the public `AddResource(contracts.File, string, *query_models.ResourceCreator)` signature.
- Command execution remains shell-free, argv-preserving, generation-bound, consented, and independent of ambient `PATH`.
- Never signal a persisted process group during restart recovery without identity verification; a live unverifiable group remains nonterminal and keeps command-runtime startup closed.
- A live worker may signal the exact group it created, but terminal publication still requires observable group death and closed output pipes.
- Successful imports have `error == ""`; source cleanup state is represented by `SourceDeletePending`.
- Unix source deletion remains descriptor-relative/no-follow and performs an admitted-file identity check immediately before `unlinkat`.
- Run rows, import claims, and import maps remain durable indefinitely; bounded sweep work is achieved with `ExchangeRemovedAt`, not row deletion.
- Preserve `ActorlessAtSubmission` semantics.
- Keep one writer; no delegated implementation is required for this remediation.
- Use strict RED/GREEN evidence for every production behavior change.
- Known unrelated baseline failures remain `plugin_system.TestBundledPluginLiteralURLsAreDeclared` and `server/api_tests.TestSidebar_IsWrappedInADisclosure`.

---

### Task 1: Live process-group authority and lazy inspection

**Files:**
- Modify: `plugin_commands/runner_unix.go`
- Modify: `plugin_commands/runner_test.go`
- Modify: `plugin_commands/process_identity_test.go`
- Create: `plugin_commands/process_darwin_platform_test.go`

**Interfaces:**
- Consumes: `ProcessInspector.InspectGroup(pgid, runID)` and `ProcessInspector.KillGroup(pgid)`.
- Produces: live-worker termination that calls `KillGroup` from creation-time authority, waits for `GroupDead`, and starts `groupPollInterval` polling only after parent exit or termination.
- Does not change restart-recovery signaling rules.

- [x] **Step 1: Write the failing platform-independent live-ownership test**

Add a runner test whose fake inspector reports `GroupAliveUnverified` until `KillGroup` is called, then reports `GroupDead`. Run a timed-out command with a descendant and assert:

```go
if inspector.killCalls != 1 {
    t.Fatalf("group kills = %d, want 1", inspector.killCalls)
}
if outcome.Status != RunStatusFailed || outcome.AuthoritativeStatus != RunStatusFailed {
    t.Fatalf("outcome = %+v", outcome)
}
```

The test must fail because current `killGroup` kills only `cmd.Process` when inspection is unverified.

- [x] **Step 2: Write the failing no-continuous-polling test**

Count `InspectGroup` calls while a direct parent remains alive for at least `10 * groupPollInterval` without cancellation. Assert the count stays below 4 before the parent exits. The current ticker should make this fail with repeated calls.

- [x] **Step 3: Add the Darwin Apple-platform executable regression**

In a `//go:build darwin` test, execute `/bin/sh` from an explicit command path, have it launch `/bin/sleep 600` and write the descendant PID into the exchange directory, cancel the run, wait for durable terminal status, and assert `syscall.Kill(pid, 0)` returns `ESRCH`. This test must fail on the current environment-based local kill gate.

- [x] **Step 4: Run the RED tests**

Run:

```bash
go test --tags 'json1 fts5' ./plugin_commands -run 'TestRunner.*(Unverified|Poll)|TestDarwinRunnerKillsApplePlatformDescendant' -count=1
```

Expected: failures show no group kill for unverified local ownership, excessive inspection calls, and a surviving Darwin descendant.

- [x] **Step 5: Implement creation-time live authority**

Refactor `commandExecutor.Execute` so:

```go
killCreatedGroup := func() {
    if err := e.deps.Inspector.KillGroup(pgid); err != nil && !errors.Is(err, syscall.ESRCH) {
        appendReason(fmt.Sprintf("kill process group: %v", err))
    }
}
```

is used for timeout, cancellation, quota termination, disable, and shutdown after `cmd.Start` succeeds. Remove environment identity as a prerequisite for this live-worker signal. Keep direct-child kill only as a fallback when group signaling itself errors.

Do not create the group ticker at fork. Create/enable it only after `parentDone` becomes true or `terminate` first runs. After termination, keep polling until `InspectGroup` reports `GroupDead`; neither `GroupAliveUnverified` nor an inspection error permits the loop to publish terminal state.

- [x] **Step 6: Run GREEN and race verification**

Run:

```bash
go test --tags 'json1 fts5' ./plugin_commands -run 'TestRunner|TestNativeProcessInspector|TestDarwinRunner' -count=1
go test -race --tags 'json1 fts5' ./plugin_commands -run 'TestRunner.*(Unverified|Poll|Timeout|Cancel)' -count=20
```

Expected: all selected tests pass; Darwin descendant is absent before terminal publication.

- [x] **Step 7: Commit**

```bash
git add plugin_commands/runner_unix.go plugin_commands/runner_test.go plugin_commands/process_identity_test.go plugin_commands/process_darwin_platform_test.go
git commit -m "fix: terminate locally owned command groups"
```

---

### Task 2: Refuse terminal recovery for live unverifiable groups

**Files:**
- Modify: `plugin_commands/recovery.go`
- Modify: `plugin_commands/recovery_test.go`
- Modify: `application_context/plugin_command_lifecycle_test.go`

**Interfaces:**
- Consumes: durable `RunRecord.ProcessGroupID` and `ProcessInspector`.
- Produces: `Recover(context.Context) error` that leaves a live unverifiable run nonterminal and prevents dispatcher/runtime startup.
- Preserves the no-pgid crash shape as terminal `interrupted + OutputUnverified`.

- [x] **Step 1: Write the failing recovery-store test**

Seed a running row with a persisted pgid and configure the inspector to return `GroupAliveUnverified`. Assert:

```go
if err := dispatcher.Recover(context.Background()); err == nil || !strings.Contains(err.Error(), "ownership") {
    t.Fatalf("Recover error = %v", err)
}
record, _, _ := store.Run(runID)
if record.Status != RunStatusRunning || record.FinishedAt != nil {
    t.Fatalf("run was settled while its group remained alive: %+v", record)
}
```

- [x] **Step 2: Write the failing lifecycle-startup test**

At the application runtime seam, point a durable running row at the test process's live group without a matching run marker. Assert startup returns an ownership error, does not publish the dispatcher/exchange host, leaves the row running, and releases the failed startup attempt's staging-root lease.

- [x] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./plugin_commands -run 'TestRecover.*Unverified' -count=1
go test --tags 'json1 fts5' ./application_context -run 'TestPluginCommand.*Unverified.*Recovery' -count=1
```

Expected: current code stamps `interrupted + output_unverified`, so the assertions fail.

- [x] **Step 4: Implement fail-closed recovery**

Change `recoverRunning` to return `(RunFinish, bool, error)` or an equivalent explicit settlement decision. Required cases:

```text
no pgid                 -> settle interrupted + output_unverified
GroupDead               -> settle interrupted/cancelled
GroupAliveOwned         -> kill, verify dead, settle
GroupAliveUnverified    -> return error, do not call FinishRun
inspection error        -> return error, do not call FinishRun
```

`Recover` must stop before dispatcher startup when a run cannot be settled safely.

- [x] **Step 5: Run GREEN and race verification**

```bash
go test --tags 'json1 fts5' ./plugin_commands -run 'TestRecover' -count=1
go test -race --tags 'json1 fts5' ./plugin_commands ./application_context -run 'Test.*(Recover|Recovery|RuntimeLease)' -count=20
```

- [x] **Step 6: Commit**

```bash
git add plugin_commands/recovery.go plugin_commands/recovery_test.go application_context/plugin_command_lifecycle_test.go
git commit -m "fix: keep unverifiable command groups nonterminal"
```

---

### Task 3: Make import source cleanup real and semantically separate

**Files:**
- Modify: `models/plugin_command_import_model.go`
- Modify: `plugin_commands/types.go`
- Modify: `plugin_commands/imports.go`
- Modify: `plugin_commands/exchange_unix.go`
- Modify: `plugin_commands/exchange_windows.go`
- Modify: `plugin_commands/imports_test.go`
- Modify: `application_context/plugin_command_store.go`
- Modify: `application_context/plugin_command_import.go`
- Modify: `application_context/plugin_command_store_test.go`
- Modify: `plugin_system/fs_api.go`
- Modify: `plugin_system/fs_api_test.go`

**Interfaces:**
- Adds `SourceDeletePending bool` to `ImportRecord`, `ImportMapEntry`, `ImportFinish`, and `ImportResult`.
- Adds `SetImportSourceDeletePending(importID string, pending bool) error` to `plugin_commands.Store`.
- Produces Lua `imports[name].source_delete_pending` and callback `source_delete_pending`.
- Removes `RecordImportDeleteFailure`; successful cleanup state never occupies `Error`.

- [x] **Step 1: Write failing import success and cleanup-failure tests**

Replace the always-retained success assertion with:

```go
if _, err := os.Stat(sourcePath); !errors.Is(err, os.ErrNotExist) {
    t.Fatalf("successful import source still exists: %v", err)
}
if mapped.Error != "" || mapped.SourceDeletePending {
    t.Fatalf("successful import map = %+v", mapped)
}
```

Update the replacement/symlink cleanup failure test to assert status `succeeded`, resource id present, `Error == ""`, and `SourceDeletePending == true`.

- [x] **Step 2: Write failing persistence and Lua-shape tests**

In application-context tests, persist success with `SourceDeletePending: true`, clear it transactionally in both claim and map, and assert both rows changed. In `plugin_system/fs_api_test.go`, assert runs and callback tables expose a boolean `source_delete_pending` without converting it into `error`.

- [x] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context ./plugin_system -run 'Test.*(Import.*Delete|SourceDeletePending|Runs.*Import)' -count=1
```

Expected: missing fields/methods and retained source behavior fail.

- [x] **Step 4: Add durable cleanup state**

Add `SourceDeletePending bool` to `models.PluginCommandImport` and `models.PluginCommandImportMap`, thread it through converters and finish/map updates, and implement:

```go
func (ctx *MahresourcesContext) SetImportSourceDeletePending(importID string, pending bool) error
```

as one transaction updating the succeeded claim and its succeeded map entry. A successful resource import initially persists `SourceDeletePending: true`; this makes a crash between durable success and unlink conservative.

- [x] **Step 5: Restore descriptor-relative unlink**

After `os.SameFile(admittedInfo, currentInfo)` succeeds, call:

```go
return unix.Unlinkat(int(dir.Fd()), name, 0)
```

Remove `errExchangeAtomicUnlinkUnavailable` and its test-only after-identity hook. Preserve no-follow regular-file checks and classify real unlink errors normally.

- [x] **Step 6: Order cleanup and callback delivery**

After durable success:

1. attempt `deleteImportedSource`;
2. on success, call `SetImportSourceDeletePending(importID, false)`;
3. on delete or clear failure, leave the flag true and log the reason;
4. deliver completion only after those steps, with `Error == ""` and the final conservative flag.

A synchronous successful-map short circuit returns the resource id as before.

- [x] **Step 7: Run GREEN and race verification**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context ./plugin_system -run 'Test.*(Import|SourceDeletePending|Unlink|Runs)' -count=1
go test -race --tags 'json1 fts5' ./plugin_commands ./application_context -run 'Test.*Import' -count=20
```

- [x] **Step 8: Commit**

```bash
git add models/plugin_command_import_model.go plugin_commands/types.go plugin_commands/imports.go plugin_commands/exchange_unix.go plugin_commands/exchange_windows.go plugin_commands/imports_test.go application_context/plugin_command_store.go application_context/plugin_command_import.go application_context/plugin_command_store_test.go plugin_system/fs_api.go plugin_system/fs_api_test.go
git commit -m "fix: drain successful command imports"
```

---

### Task 4: Remove the redundant import snapshot and enforce a two-copy quota

**Files:**
- Modify: `plugin_commands/imports.go`
- Modify: `plugin_commands/imports_test.go`
- Modify: `application_context/plugin_command_import.go`
- Modify: `application_context/plugin_command_import_test.go`

**Interfaces:**
- `ImportSource.File` becomes the admitted exchange descriptor consumed through an exact-size, cancellation-aware reader.
- `ImportSource.CreateScratch` remains the sole AddResource snapshot factory.
- `reserveImportQuota` reserves `sourceSize`, not `2 * sourceSize`; measured exchange usage already includes the source.

- [x] **Step 1: Write the failing two-copy boundary test**

Create a run containing one source of size `S`. Set `PerRunQuota` to `2*S`; assert import succeeds. Set it to `2*S-1`; assert import fails before the importer receives bytes. The first case must fail under the current `usage + 2*S` reservation.

- [x] **Step 2: Write exact-size reader tests**

Add focused tests proving the reader:

- yields exactly `S` bytes for an unchanged source;
- returns a named error if the source ends before `S`;
- returns a named error without admitting extra bytes if the source grows beyond `S`;
- returns `context.Canceled` when cancelled during the copy.

- [x] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context -run 'Test.*(Import.*Quota|ExactSize|ImportResource)' -count=1
```

- [x] **Step 4: Remove the outer snapshot**

Delete `copyImportSnapshot`. Create only the claim temp directory and pass the admitted source descriptor directly to `Importer.ImportResource`. Wrap it in the exact-size/cancellation reader before `addResourceWithOptions`; keep `CreateScratch` pointing at `temp.Create("upload-")` so AddResource's copy is the one immutable snapshot.

- [x] **Step 5: Correct reservation arithmetic**

Change:

```go
reservation := sourceSize * 2
```

to one additional source size, with overflow-safe checks. Measured `usage` already includes every original exchange source and any current import temp; `reserved` covers concurrent scratch files not yet visible or not safely attributable between workers.

- [x] **Step 6: Run GREEN and race verification**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context -run 'Test.*(Import|Quota|ExactSize)' -count=1
go test -race --tags 'json1 fts5' ./plugin_commands ./application_context -run 'Test.*(Import|Quota)' -count=20
```

- [x] **Step 7: Commit**

```bash
git add plugin_commands/imports.go plugin_commands/imports_test.go application_context/plugin_command_import.go application_context/plugin_command_import_test.go
git commit -m "fix: bound command imports at two copies"
```

---

### Task 5: Bound durable exchange-retention work

**Files:**
- Modify: `models/plugin_command_run_model.go`
- Modify: `plugin_commands/types.go`
- Modify: `plugin_commands/store.go`
- Modify: `plugin_commands/dispatcher.go`
- Modify: `plugin_commands/lifecycle_test.go`
- Modify: `plugin_commands/dispatcher_test.go`
- Modify: `plugin_commands/runner_test.go`
- Modify: `plugin_commands/sweep_race_test.go`
- Modify: `application_context/plugin_command_store.go`
- Modify: `application_context/plugin_command_store_test.go`
- Modify: `application_context/plugin_command_store_pg_test.go`

**Interfaces:**
- Adds `ExchangeRemovedAt *time.Time` to `RunRecord` and `models.PluginCommandRun`.
- Adds `MarkRunExchangeRemoved(id string, removed time.Time) error` to `Store`.
- `ExpiredTerminalRunBoundary(before)` captures the newest eligible finish/id at the start of one retention cycle.
- `ExpiredTerminalRuns(before, after, through)` returns at most `pluginCommandSweepBatchSize` unswept rows ordered by finish/id inside that fixed cycle boundary.

- [x] **Step 1: Write failing store-selection tests**

Seed more than the batch size of expired terminal rows, one already marked row, and one nonexpired row. Assert only the first bounded unswept batch is returned. Mark one returned row and assert it never appears in a later call.

- [x] **Step 2: Write failing dispatcher sweep tests**

Assert successful deletion and `os.ErrNotExist` both call `MarkRunExchangeRemoved`. Assert leased runs and runs with nonterminal imports are not marked. Assert a lost/no-op mark is returned as an error and the row remains eligible for retry. Fill one batch with pinned rows, append at least one full batch of newly expired rows, and assert the next sweep still reaches the original fixed boundary so the following cycle can retry the pinned prefix.

- [x] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context -run 'Test.*(ExpiredTerminalRuns|Sweep.*Removed|ExchangeRemoved)' -count=1
```

- [x] **Step 4: Add schema and store behavior**

Add the nullable timestamp and a composite index over `exchange_removed_at, finished_at, id`. Thread it through `runModel`/`runRecord`. Add a bounded `LIMIT` to `ExpiredTerminalRuns`, filter `exchange_removed_at IS NULL`, and page after a finish/id cursor through a fixed cycle boundary so sustained new expiry cannot defer wraparound. Implement a conditional mark that errors unless exactly one terminal row wins.

- [x] **Step 5: Mark only completed sweep ownership**

In `Dispatcher.sweep`, after `removeExchangeRunDir` succeeds or returns `os.ErrNotExist`, persist `MarkRunExchangeRemoved` and atomically reconcile successful imports' `source_delete_pending` flags. Do not mark when lease acquisition fails, imports are nonterminal, filesystem removal fails, or the mark itself fails.

- [x] **Step 6: Run SQLite/PostgreSQL GREEN and race tests**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context -run 'Test.*(Sweep|ExpiredTerminalRuns|ExchangeRemoved)' -count=1
go test --tags 'json1 fts5 postgres' ./application_context -run 'TestPluginCommand.*(Sweep|Store)' -count=1
go test -race --tags 'json1 fts5' ./plugin_commands -run 'Test.*Sweep' -count=20
```

- [x] **Step 7: Commit**

```bash
git add models/plugin_command_run_model.go plugin_commands/types.go plugin_commands/store.go plugin_commands/dispatcher.go plugin_commands/lifecycle_test.go plugin_commands/dispatcher_test.go plugin_commands/runner_test.go plugin_commands/sweep_race_test.go application_context/plugin_command_store.go application_context/plugin_command_store_test.go application_context/plugin_command_store_pg_test.go
git commit -m "fix: bound command exchange retention sweeps"
```

---

### Task 6: Cache global staging usage outside command admission

**Files:**
- Modify: `plugin_commands/runner_unix.go`
- Modify: `plugin_commands/runner_windows.go`
- Modify: `plugin_commands/dispatcher.go`
- Modify: `plugin_commands/quota_test.go`
- Modify: `plugin_commands/lifecycle_test.go`

**Interfaces:**
- Adds an internal optional interface:

```go
type globalUsageRefresher interface {
    RefreshGlobalUsage() error
}
```

- Unix `commandExecutor.Prepare` reads an atomically published sample and never calls `pathUsageNoSymlinks`.
- `Dispatcher.sweep` refreshes the sample after retention work; initial `Start` fails if its first refresh fails.

- [x] **Step 1: Write the failing cached-admission test**

Refresh an empty staging root, then add a file larger than the global quota before calling `Prepare`. Assert `Prepare` uses the published sample rather than rescanning. Refresh again and assert the next `Prepare` refuses. Also assert no sample means an explicit `global staging usage is unavailable` refusal.

- [x] **Step 2: Write failing startup/sweep refresh tests**

Use a fake executor implementing `RefreshGlobalUsage`. Assert initial start calls it once, each sweep calls it after filesystem cleanup, and a startup refresh error prevents dispatcher start.

- [x] **Step 3: Run RED**

```bash
go test --tags 'json1 fts5' ./plugin_commands -run 'Test.*(CachedGlobal|RefreshGlobal|GlobalStaging)' -count=1
```

- [x] **Step 4: Implement atomic sample publication**

Add a shared usage cache to `commandExecutor`. `Refresh` walks the staging root and publishes only after a successful complete scan. `Prepare` reads the sample, checks the current live limit, and proceeds without filesystem traversal.

`NewDispatcher` adopts the executor's cache when the dependency is omitted, and `Start` refuses mismatched cache identities. Invoke refresh at the end of every sweep, including the initial sweep in `Start`. A failed periodic refresh logs/returns the sweep error but leaves the previous complete sample intact; a failed initial refresh prevents dispatcher start because readiness was never established.

- [x] **Step 5: Run GREEN and race verification**

```bash
go test --tags 'json1 fts5' ./plugin_commands -run 'Test.*(Quota|Global|Sweep|Start)' -count=1
go test -race --tags 'json1 fts5' ./plugin_commands -run 'Test.*(CachedGlobal|RefreshGlobal|Submit|Sweep)' -count=20
```

- [x] **Step 6: Commit**

```bash
git add plugin_commands/runner_unix.go plugin_commands/runner_windows.go plugin_commands/dispatcher.go plugin_commands/quota_test.go plugin_commands/lifecycle_test.go
git commit -m "perf: cache plugin command staging usage"
```

---

### Task 7: Give command-history cancellation controls distinct names

**Files:**
- Modify: `templates/pluginCommandHistory.tpl`
- Modify: `server/api_tests/plugin_command_history_test.go`
- Modify: `e2e/tests/plugins/plugin-command-history.spec.ts`

**Interfaces:**
- Visible button text remains `Cancel queued run`.
- Accessible names become `Cancel queued run <run-id>` in both detail and list rows.

- [x] **Step 1: Write failing API/template and E2E assertions**

Render two queued rows plus one detail row and assert the HTML contains run-specific labels. In Playwright, select exact buttons by `getByRole('button', {name: 'Cancel queued run <id>', exact: true})` and assert each resolves uniquely.

- [x] **Step 2: Run RED**

```bash
go test --tags 'json1 fts5' ./server/api_tests -run TestPluginCommandHistory -count=1
cd e2e && npm run test:with-server -- tests/plugins/plugin-command-history.spec.ts
```

Expected: current bare accessible names fail exact run-id lookup.

- [x] **Step 3: Add `aria-label` values**

Use Pongo interpolation:

```html
aria-label="Cancel queued run {{ commandRun.ID }}"
aria-label="Cancel queued run {{ run.ID }}"
```

Keep the visible label unchanged. When the selected detail run also appears in the table, replace its duplicate list action with a passive `Shown above` marker so the exact accessible name occurs only once.

- [x] **Step 4: Run GREEN**

```bash
go test --tags 'json1 fts5' ./server/api_tests -run TestPluginCommandHistory -count=1
cd e2e && npm run test:with-server -- tests/plugins/plugin-command-history.spec.ts
```

- [x] **Step 5: Commit**

```bash
git add templates/pluginCommandHistory.tpl server/api_tests/plugin_command_history_test.go e2e/tests/plugins/plugin-command-history.spec.ts
git commit -m "fix: label command cancellation controls"
```

---

### Task 8: Align operator and plugin documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs-site/docs/features/plugin-lua-api.md`
- Modify: `docs-site/docs/configuration/advanced.md`
- Modify: `docs/todo.md`
- Modify: `docs/superpowers/plans/2026-09-20-plugin-command-runtime-remediation.md`

**Interfaces:**
- Documents local versus recovery process authority, recovery startup refusal, immediate successful-source drain, `source_delete_pending`, two-copy sizing, sampled global quota, bounded sweep markers, and unique cancellation names.

- [x] **Step 1: Update documentation**

Keep `CLAUDE.md` invariant-focused. In the Lua API reference, document:

```lua
imports[name].source_delete_pending -- boolean, not an import error
```

and state that ordinary success removes source bytes immediately. In advanced configuration, state per-run import staging needs approximately `2 ×` the largest single source (plus other retained files), while global admission uses the most recent startup/sweep sample.

- [x] **Step 2: Record RED/GREEN evidence and task completion**

Add a remediation review section to `docs/todo.md` with each finding, root cause, test, fix commit, and remaining no-pgid limitation. Check off completed plan items only after their corresponding verification command passes.

- [x] **Step 3: Run documentation and generated-file checks**

```bash
npm run build-js
npm run build-css
./scripts/css-scan-test.sh
./mr docs lint
npm run skills-gen
git diff --check
```

Expected: all checks pass and generated committed assets are either unchanged or intentionally staged.

- [x] **Step 4: Commit**

```bash
git add CLAUDE.md docs-site/docs/features/plugin-lua-api.md docs-site/docs/configuration/advanced.md docs/todo.md docs/superpowers/plans/2026-09-20-plugin-command-runtime-remediation.md public/tailwind.css public/dist
git commit -m "docs: record plugin command runtime remediation"
```

---

### Task 9: Whole-branch verification and independent readiness audit

**Files:**
- Modify only if a failing regression requires a test-first correction.

**Interfaces:**
- Produces merge-readiness evidence; does not weaken behavior or suppress the two accepted unrelated baseline failures.

- [x] **Step 1: Run focused race suites**

```bash
go test -race --tags 'json1 fts5' ./plugin_commands ./application_context ./plugin_system ./server/api_tests -run 'Test.*(PluginCommand|Command|Import|Sweep|RuntimeLease)' -count=10
```

- [x] **Step 2: Run package and frontend suites**

```bash
go test --tags 'json1 fts5' ./plugin_commands ./application_context ./plugin_system ./server/api_tests -count=1
npm run test:unit -- --run
```

Classify only the two named baseline failures as accepted; every new failure is blocking.

- [x] **Step 3: Run PostgreSQL selections**

```bash
go test --tags 'json1 fts5 postgres' ./application_context ./server/api_tests -run 'Test.*PluginCommand' -count=1
cd e2e && npm run test:with-server:postgres -- tests/plugins/plugin-command-history.spec.ts
```

- [x] **Step 4: Run browser/CLI shards on ephemeral servers**

```bash
cd e2e
npm run test:with-server
npm run test:with-server:auth
npm run test:with-server:cli
npm run test:with-server:cli-doctest
```

Use bounded shards if the monolithic harness exceeds worker-server startup limits; do not mask application failures.

- [x] **Step 5: Run final static/build checks**

```bash
npm run build
go vet --tags 'json1 fts5' ./...
./scripts/css-scan-test.sh
./mr docs lint
npm run skills-gen
git diff --check
git status --short
```

- [x] **Step 6: Review the complete remediation diff**

Compare `ac0a5603..HEAD` against the approved design. Specifically inspect process terminal ordering, recovery non-mutation, import callback ordering, cleanup flag persistence, quota arithmetic, sweep marking conditions, global-cache publication, and accessible names.

- [x] **Step 7: Commit any verification-only documentation update**

```bash
git add docs/todo.md docs/superpowers/plans/2026-09-20-plugin-command-runtime-remediation.md
git commit -m "docs: complete plugin command runtime remediation"
```

## Recovery containment follow-up review (2026-09-20)

The approved recovery-containment follow-up preserves the remediation's process
ownership rules while containing safety refusals to the command runtime. PGID is
paired atomically with a private host-unique boot-session UUID; Linux and Darwin
exclude zombie-only groups from writer liveness; signal zero remains only
conservative existence evidence. Recovery blockers retain the staging lease and
retry every five minutes, while lease contention uses short capped backoff.
Quarantine keeps rows nonterminal, withholds command mutations, records
operator-actionable `/logs` warnings and activates automatically when safe.
Recovery has one verified signal attempt per run per process; a live creator has
at most two attempts and then one-second polling with a one-shot pinned-capacity
warning.

Final verification passed the build, CSS scan, tagged vet, ten focused race
repetitions, focused PostgreSQL command tests, SQLite command-history browser
coverage, all 1,412 frontend unit tests, and the full 2,242-pass/5-skip
browser/auth/CLI/doctest matrix. The aggregate tagged Go rerun reproduced only
the two accepted unrelated baselines. Its first run found one stale API fixture:
a schedule re-enable test disabled a plugin without starting the command runtime,
which now truthfully reports quarantine because durable work cannot be revoked.
The fixture was corrected to use the production runtime lifecycle and passes 10
ordinary and race repetitions. Native Darwin command tests pass. Linux and
FreeBSD `CGO_ENABLED=0` package compilation remain blocked by the repository's
SQLite stub (`sqlite3.SQLiteConn.Exec`), not this runtime.

The remaining platform limitation is deliberate: AIX, DragonFly BSD, FreeBSD,
NetBSD, OpenBSD and Solaris have probe-only inspection and cannot prove a
zombie-only group dead. Their quarantine may require parent reaping or restart.
Independent closure review remains the approval gate; `docs/todo.md` therefore
leaves recovery-containment Task 9 unchecked.
