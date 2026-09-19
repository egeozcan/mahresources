# Plugin-Declared Server Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a consented plugin run fixed, shell-free server command templates as durable background jobs and safely mediate their output files into library resources.

**Architecture:** Add a deep `plugin_commands` module below `application_context` and `plugin_system`. It owns declaration validation, argv substitution, the dedicated command/import dispatchers, process groups, staging files, quotas, recovery, leases and lifecycle state behind small host/store interfaces. `plugin_system` owns manifest consent and the Lua-facing `mah.commands`/`mah.fs` adapters; `application_context` owns GORM persistence, actor/scope validation and resource creation; `download_queue` remains the live job/SSE registry for **dispatched** work through a six-entry managed-job lane, but queued rows stay in the dispatcher and it does not schedule command or import concurrency.

**Tech Stack:** Go, GORM, SQLite/PostgreSQL, gopher-lua, `golang.org/x/sys/unix`, Afero, Gorilla Mux, Pongo2, Cobra, Alpine.js, Tailwind, and Playwright.

**Spec:** `docs/superpowers/specs/2026-09-19-plugin-commands-design.md`

## Global Constraints

- This is trusted service-account execution, not sandboxing. The host never starts a shell and never reparses a parameter.
- `argv[0]` is a literal nonempty executable basename: no placeholder, leading dash, `/`, `\`, or `..`; resolve it from the operator-controlled `PATH` at run time.
- A placeholder occupies one whole argv element. `{{exchange_dir}}` is host-filled and cannot be supplied in `params`.
- One run accepts at most 32 parameters, 8 KiB per value and 64 KiB aggregate parameter bytes. Empty substitutions and missing placeholders are refused.
- Command timeout defaults to 3,600 seconds and is capped at 86,400 seconds.
- Command dispatch is FIFO per plugin, capped at 2 running commands per plugin and 4 globally. Imports use a separate global pool capped at 2.
- Pending command and import admission is bounded at `download_queue.MaxQueueSize` (100) **per plugin** in dispatcher-owned queues. Queued work consumes no download-registry slot. Only dispatched work enters a separate six-entry managed-live lane (4 command + 2 import); ordinary downloads retain their own 100-entry registry budget and semaphore.
- Command statuses are exactly `queued`, `running`, `succeeded`, `failed`, `cancelled`, `interrupted`; import statuses are exactly `pending`, `running`, `succeeded`, `failed`, `cancelled`, `interrupted`.
- Timeout/quota/cannot-start is `failed`; operator cancel/plugin disable is `cancelled`; restart/shutdown/dispatch loss is `interrupted`.
- v1 command execution is Unix-only. Command-bearing manifests still parse on Windows; runs refuse there.
- Spawned commands receive only `PATH`, `HOME`, private per-run `TMPDIR`, `LANG`, `TZ`, `MAHR_PLUGIN_NAME`, `MAHR_COMMAND_RUN_ID`, and `MAHR_EXCHANGE_DIR`; stdin is `os.DevNull`. `PATH` is the validated `-plugin-command-path` value (nonempty absolute directories), defaulted once at startup from the inherited process `PATH`, which is therefore a documented trust boundary until explicitly pinned.
- Commands receive no plugin egress enforcement and may reach private/loopback addresses. Documentation and consent copy must state this.
- Default per-run exchange quota is 8 GiB; default global staging quota is 50 GiB. Imports may drain staging while above the global quota.
- The output record stores a 64 KiB combined stdout/stderr tail. Output rows default to 30-day retention; run rows, import claims and import maps are not pruned in v1.
- Exchange directories default to 7-day retention measured from completion and are private (`0700`).
- Plugin-visible files are top-level regular files addressed only by `(run_id, name)`; lexical validation, `lstat`, relative `O_NOFOLLOW` open and post-open verification are all required.
- `mah.fs.list` returns at most 10,000 entries plus `truncated=true`; `mah.fs.read` has a hard 4 MiB maximum.
- Command and import callbacks are at-most-once, run under the exclusive VM lock and `plugin_system.MaxAsyncJobDuration`, and are never replayed after restart.
- `mah.fs.create_resource` requires both `commands` and `db:write`, refuses inside `mah.db.transaction`, atomically claims work and returns before bytes move.
- Command history and captured output are administrator-only, even when a non-admin submitted the run.
- Generated CLI docs, OpenAPI output and `public/dist` assets changed by source work are committed.
- The separate `yt-dlp-plugin-for-mahresources` repository is not implemented by this plan. This plan ends with a host-side fixture proving the public interface it will consume.
- Tests use ephemeral data. One writer owns the checkout. The planning baseline is `283c2afa`.

---

## Execution and file ownership map

Read `CLAUDE.md`, `docs/lessons.md` and the spec before execution. Create an isolated worktree; do not implement on `master` without explicit operator consent. Task 1 is a standalone integrity commit and must remain revertible independently from commands.

| Module | Files and responsibility |
| --- | --- |
| Shared upload integrity | `application_context/resource_upload_context.go` and focused tests: replay-safe content-addressed destination replacement and import-owned scratch selection |
| Declaration/consent | `plugin_commands/declaration.go`, `plugin_system/manifest.go`, `plugin_system/consent.go`: command templates, capability, identity and explicit consent |
| Durable records | New `models/plugin_command_*_model.go` and `application_context/plugin_command_store.go`: run/output/import/map transitions |
| Runtime | New `plugin_commands/dispatcher.go`, `runner_unix.go`, `process_{linux,darwin}.go`, `exchange_unix.go`: pools, process groups, output, quotas, recovery and leases |
| Live jobs | `download_queue/generic_job.go`, `job.go`, `manager.go`: externally scheduled managed jobs with explicit terminal outcomes and control policy |
| Host adapters | New `application_context/plugin_command_context.go` and `plugin_command_import.go`: plugin generation/actor checks, resource imports and startup/shutdown wiring |
| Lua interface | New `plugin_system/commands_api.go`, `fs_api.go`, `command_callbacks.go`: `mah.commands`, `mah.fs`, transaction guards and callbacks |
| Consent/UI/CLI | Plugin enable handler/context, manage template, `cmd/mr/commands/plugins.go` and help: two-step acknowledgement and verbatim command display |
| History/docs | New admin handler/provider/template, routes/OpenAPI, plugin docs and configuration docs |

Tasks 1–3 establish safe storage and informed consent. Tasks 4–8 establish durable execution and mediated files. Tasks 9–11 expose the host interface and lifecycle. Tasks 12–13 integrate operator surfaces and complete end-to-end verification.

---

### Task 1: Make shared resource destinations replay-safe

**Files:**
- Modify: `application_context/resource_upload_context.go:1051-1370`.
- Create: `application_context/resource_upload_replay_test.go`.
- Extend: `application_context/resource_upload_concurrency_test.go`.

**Interfaces:**
- Produces private `addResourceOptions{ScratchDir string}` and `addResourceWithOptions(file, fileName, creator, opts)`.
- Preserves the public `AddResource(contracts.File, string, *query_models.ResourceCreator)` interface unchanged.
- Later Task 9 calls the private helper from `application_context` with an import claim's managed scratch directory.

- [x] **Step 1: Write the failing interrupted-copy replay test**

Create the spec's mid-destination-copy crash/replay test with an Afero wrapper whose first `Create` for the resource destination returns a file that writes a strict prefix and then returns a sentinel error. Call `AddResource`, assert the first call fails and leaves the prefix, restore ordinary writes, call it again, and assert the committed resource's backing file equals the complete literal payload.

```go
func TestAddResource_ReplayReplacesATruncatedUncommittedDestination(t *testing.T) {
    payload := bytes.Repeat([]byte("complete-payload-"), 4096)
    ctx, fs := newReplayUploadContext(t)
    fs.FailNextResourceCopyAfter(1024)

    _, err := ctx.AddResource(newBytesFile(payload), "replay.bin", &query_models.ResourceCreator{Name: "replay.bin"})
    require.ErrorIs(t, err, errInjectedDestinationCopy)

    got, err := ctx.AddResource(newBytesFile(payload), "replay.bin", &query_models.ResourceCreator{Name: "replay.bin"})
    require.NoError(t, err)
    stored, err := afero.ReadFile(ctx.fs, got.Location)
    require.NoError(t, err)
    require.Equal(t, payload, stored)
}
```

- [x] **Step 2: Run the replay test and verify the stale-prefix failure**

Run: `go test --tags 'json1 fts5' ./application_context -run TestAddResource_ReplayReplacesATruncatedUncommittedDestination -count=1`

Expected: FAIL because the second call takes the current `Stat`/reuse branch and commits a resource whose backing file is the short prefix.

- [x] **Step 3: Write the committed-row precedence and concurrent controls**

Add one test that creates a committed resource, replaces its backing file with a same-path sentinel, and repeats the upload. Assert the hash lookup returns/merges the committed row without opening, deleting or replacing the destination. Add the spec's concurrent same-content test with two goroutines, asserting one logical resource and a complete backing file.

```go
func TestAddResource_CommittedHashLookupPrecedesDestinationRepair(t *testing.T) {
    payload := []byte("already committed")
    ctx, fs := newReplayUploadContext(t)
    first, err := ctx.AddResource(newBytesFile(payload), "first.bin", &query_models.ResourceCreator{Name: "first.bin"})
    require.NoError(t, err)

    fs.FailIfDestinationIsTouched(first.Location)
    second, err := ctx.AddResource(newBytesFile(payload), "second.bin", &query_models.ResourceCreator{Name: "second.bin"})
    require.NoError(t, err)
    require.Equal(t, first.ID, second.ID)
    fs.AssertDestinationWasNotTouched(t)
}

func TestAddResource_ConcurrentSameContentNeverUnlinksTheWinner(t *testing.T) {
    payload := bytes.Repeat([]byte("same-content"), 4096)
    ctx, _ := newReplayUploadContext(t)
    start := make(chan struct{})
    results, errs := runTwoUploads(t, start, func() (*models.Resource, error) {
        return ctx.AddResource(newBytesFile(payload), "same.bin", &query_models.ResourceCreator{Name: "same.bin"})
    })
    require.NoError(t, errs[0])
    require.NoError(t, errs[1])
    require.Equal(t, results[0].ID, results[1].ID)
    stored, err := afero.ReadFile(ctx.fs, results[0].Location)
    require.NoError(t, err)
    require.Equal(t, payload, stored)
}
```

- [x] **Step 4: Add the private options-aware helper**

Keep the public method as the compatibility wrapper and route only its temporary snapshot directory through the option:

```go
type addResourceOptions struct {
    ScratchDir string
}

func (ctx *MahresourcesContext) AddResource(file contracts.File, fileName string, q *query_models.ResourceCreator) (*models.Resource, error) {
    return ctx.addResourceWithOptions(file, fileName, q, addResourceOptions{})
}

func (ctx *MahresourcesContext) addResourceWithOptions(file contracts.File, fileName string, q *query_models.ResourceCreator, opts addResourceOptions) (*models.Resource, error) {
    scratch := opts.ScratchDir
    if scratch == "" {
        scratch = ctx.Config.HLSTempDir
    }
    tempFile, err := os.CreateTemp(scratch, "upload-")
    // existing implementation continues here
}
```

- [x] **Step 5: Validate and replace only a mismatched uncommitted destination**

Under `ResourceHashLock`, retain the committed-row lookup as the first branch. Add `io/fs` to the imports. Only after `gorm.ErrRecordNotFound`, compare `Stat().Size()` with `preFileSize`; reuse equal-size files, but close/remove a mismatched file and create/copy the complete snapshot. Rewind the snapshot before every copy.

```go
stat, statErr := targetFs.Stat(filePath)
switch {
case statErr == nil && stat.Size() == preFileSize:
    savedFile, err = targetFs.Open(filePath)
    fileExists = true
case statErr == nil:
    if err = targetFs.Remove(filePath); err == nil {
        savedFile, err = targetFs.Create(filePath)
    }
case errors.Is(statErr, fs.ErrNotExist):
    savedFile, err = targetFs.Create(filePath)
default:
    return nil, fmt.Errorf("stat resource destination: %w", statErr)
}
```

Treat non-`IsNotExist` stat errors as errors; do not turn an unreadable destination into permission to overwrite it.

- [x] **Step 6: Verify red/green and mutation sensitivity**

Run the three focused tests, then temporarily restore unconditional stale-file reuse and confirm the replay test fails. Restore the fix and run:

```bash
go test --tags 'json1 fts5' ./application_context -run 'TestAddResource_(Replay|Committed|ConcurrentSameContent)' -count=1
go test --tags 'json1 fts5' ./application_context -run 'TestAddResource|TestUpload' -count=1
git diff --check
```

- [x] **Step 7: Commit the standalone fix**

```bash
git add application_context/resource_upload_context.go application_context/resource_upload_replay_test.go application_context/resource_upload_concurrency_test.go
git commit -m "fix: repair truncated resource files on upload replay"
```

---

### Task 2: Parse and compare command declarations

**Files:**
- Create: `plugin_commands/declaration.go`, `plugin_commands/declaration_test.go`.
- Modify: `plugin_system/manifest.go`, `plugin_system/manifest_test.go`, `plugin_system/capability_grants_test.go`.
- Modify: `internal/arch/layering_test.go`.

**Interfaces:**

```go
// plugin_commands/declaration.go
const (
    DefaultTimeout = time.Hour
    MaxTimeout = 24 * time.Hour
    MaxParameters = 32
    MaxParameterBytes = 8 << 10
    MaxAggregateParameterBytes = 64 << 10
)

type Declaration struct {
    Name            string        `json:"name"`
    Argv            []string      `json:"argv"`
    Timeout         time.Duration `json:"timeout"`
    SensitiveParams []string      `json:"sensitive_params,omitempty"`
}

type Invocation struct {
    Argv         []string
    RedactedArgv []string
    ParamView    map[string]string
}

func ValidateDeclaration(Declaration) error
func BuildInvocation(Declaration, map[string]string, string) (Invocation, error)
func SameDeclarations([]Declaration, []Declaration) bool
func ShellJoin([]string) string
```

The manifest commands field is `plugin_system.Manifest.Commands []plugin_commands.Declaration`. `SameDeclarations` supplies its identity comparison. `CapCommands = "commands"` joins all capability catalogues with surfaces `mah.commands, mah.fs`; its consent label explicitly says commands run as the server service account, have unrestricted process networking, can read anything that OS account can read (including sibling plugin exchange folders), and require `db:write` to import output.

- [x] **Step 1: Add red table tests for declaration parsing**

Cover valid defaults, duplicate command names, partial placeholders, empty argv, placeholder/literal/path/leading-dash argv[0], timeout cap, a missing `commands` capability and a legacy table that names `commands` without `api_version`.

```go
func TestManifestCommands(t *testing.T) {
    _, err := parseManifestSource(t, `plugin={api_version=1,capabilities={"commands"},commands={{name="download",argv={"tool","--","{{url}}"}}}}`)
    require.NoError(t, err)
}
```

- [x] **Step 2: Run the manifest tests and verify `commands` is unknown**

Run: `go test ./plugin_system -run 'TestManifestCommands|TestCommandManifest' -count=1`

Expected: FAIL because `commands` is not a manifest key/capability and no declaration parser exists.

- [x] **Step 3: Implement declaration validation and substitution**

Use one anchored placeholder regexp (`^\{\{([a-z][a-z0-9_]*)\}\}$`) and separately reject any literal element containing `{{` or `}}`. Reserve `exchange_dir`; reject a caller param with that key. Preserve element boundaries exactly and never construct a shell string for execution. `ShellJoin` is display-only and single-quotes every non-safe element.

- [x] **Step 4: Parse Lua command tables into declarations**

Add `commands` to `manifestKeys`. Require command `name` slug uniqueness, argv list shape, numeric whole-second timeout, string-list `sensitive_params`, and `commands` capability presence. Default timeout before validation. Include commands in `Manifest.Equal`; argv compares positionally, declarations by command name, sensitive names as a set.

- [x] **Step 5: Add substitution and identity red tests**

Assert the literal built vector, redaction of exactly the parameter-derived elements, host-filled exchange path, missing/empty/reserved params, 32/33 keys, 8 KiB/8 KiB+1 values and aggregate boundaries. Assert no `/bin/sh`, `sh -c`, `cmd.exe` or shell-joined value appears in the launch-facing type.

- [x] **Step 6: Pin the new package's layer**

Extend `internal/arch/layering_test.go` with `TestPluginCommandsStaysBelowItsConsumers`: `plugin_commands/` may import standard/third-party packages plus `models/`, `models/query_models/`, `contracts/` and `constants/`; it may not import `application_context/`, `server/` or `plugin_system/`.

- [x] **Step 7: Verify and commit**

```bash
go test ./plugin_commands ./plugin_system ./internal/arch -count=1
git diff --check
git add plugin_commands plugin_system/manifest.go plugin_system/manifest_test.go plugin_system/capability_grants_test.go internal/arch/layering_test.go
git commit -m "feat: declare consented plugin command templates"
```

---

### Task 3: Require persistent, explicit command consent

**Files:**
- Modify: `plugin_system/consent.go`, `plugin_system/consent_test.go`, `plugin_system/consent_load_test.go`.
- Modify: `application_context/plugin_consent_store.go`, `application_context/plugin_consent_store_test.go`.
- Modify: `application_context/plugin_state_context.go`, its enable/concurrency tests.
- Modify: `server/api_handlers/plugin_api_handlers.go`, `server/api_handlers/handler_interfaces.go`.
- Modify: `server/template_handlers/template_context_providers/plugin_manage_context.go`, `context_interfaces.go`.
- Modify: `templates/managePlugins.tpl`.
- Modify: `cmd/mr/client/client.go`, `cmd/mr/commands/plugins.go`, `cmd/mr/commands/plugins_test.go`, `cmd/mr/commands/plugins_help/plugin_enable.md`.

**Interfaces:**

```go
type CommandGrant struct {
    Name            string   `json:"name"`
    Argv            []string `json:"argv"`
    TimeoutSeconds  int64    `json:"timeout_seconds"`
    SensitiveParams []string `json:"sensitive_params,omitempty"`
}

type Grants struct {
    // existing fields
    Commands             []CommandGrant `json:"commands,omitempty"`
    CommandsAcknowledged bool           `json:"commands_acknowledged,omitempty"`
}

type ConsentStore interface {
    ConsentFor(string) (Grants, bool, error)
    RecordConsent(string, Grants) error
    Persistent() bool
}

type PluginEnableOptions struct { ConfirmCommands bool }
func (ctx *MahresourcesContext) SetPluginEnabledWithOptions(string, bool, PluginEnableOptions) error
```

Keep `SetPluginEnabled(name, enabled)` as a wrapper passing zero options so existing non-command callers compile; it must refuse command-bearing enables without confirmation.

- [x] **Step 1: Add consent widening tests before changing production code**

Cover positional argv change, added command, timeout/sensitive-set change, removed command as narrowing, legacy consent plus commands as widening, absent record plus commands as refusal, and memory store plus commands as refusal. Assert command comparison runs before the legacy short-circuit.

- [x] **Step 2: Verify red consent failures**

Run: `go test ./plugin_system -run 'Test.*Command.*Consent|Test.*Commands.*Grant' -count=1`

Expected: FAIL because grants contain no command declarations or acknowledgement.

- [x] **Step 3: Extend grants and fail closed without durable consent**

`GrantsFromManifest` records declarations but not acknowledgement. Add `GrantsForEnable(manifest, confirm)` which refuses a command manifest unless `confirm` and sets `CommandsAcknowledged=true`. `memoryConsentStore.Persistent()` returns false; `pluginConsentStore.Persistent()` returns true. In `enforceConsent`, refuse command manifests on nonpersistent stores and never grandfather an absent command consent.

- [x] **Step 4: Add typed confirmation refusal and two-step enable context**

Define `ErrCommandConfirmationRequired` plus an error carrying `[]CommandDisplay{Name, DisplayArgv, TimeoutSeconds}`. `SetPluginEnabledWithOptions` validates the explicit confirmation before writing consent/enabled state. For HTML, the first refused POST redirects to `/plugins/manage?confirm_commands=<name>`; the provider validates the name against discovered plugins and renders a separate confirmation form with `confirm_commands=1`. The ordinary plugin-card button never includes that field.

- [x] **Step 5: Render the warning panel and exact command display**

Every command-bearing card displays the trusted-execution warning, shell-joined argv and timeout. It says the executable basename is searched only in the operator's configured plugin-command path; the process runs as the server service account, is not sandboxed, has unrestricted networking including private/loopback addresses, can read anything that OS account can read (including sibling plugin exchange folders), and needs separate `db:write` consent to import. The confirmation branch repeats the same fields and submit copy `Confirm and enable`. Escape through Pongo2; never mark argv safe.

- [x] **Step 6: Return structured JSON and teach the CLI the second step**

On JSON refusal return HTTP 409:

```json
{"error":"command confirmation required","requiresCommandConfirmation":true,"commands":[{"name":"download","argv":"yt-dlp '--' '{{url}}'","timeoutSeconds":7200}]}
```

Make `client.APIError` exported with `StatusCode`, `Message` and raw JSON body while preserving existing `Error()` text. `mr plugin enable NAME` prints the returned command list and `Re-run with --confirm-commands`; with the flag it posts `confirm_commands=1`.

- [x] **Step 7: Verify browserless and CLI flows**

Run focused application, handler and Cobra tests. Assert: first request does not change `PluginState.Enabled`/`GrantsJSON`; confirmed request records acknowledgement; CLI without flag exits nonzero after printing every command; CLI with flag posts the acknowledgement.

- [x] **Step 8: Commit**

```bash
go test ./plugin_system ./application_context ./server/api_handlers ./server/template_handlers/template_context_providers ./cmd/mr/... -run 'Command|PluginEnable' -count=1
./mr docs lint
git diff --check
git add plugin_system/consent.go plugin_system/*consent*_test.go application_context/plugin_consent_store* application_context/plugin_state_context.go application_context/*plugin*enable*test.go server/api_handlers/plugin_api_handlers.go server/api_handlers/handler_interfaces.go server/template_handlers/template_context_providers/plugin_manage_context.go server/template_handlers/template_context_providers/context_interfaces.go templates/managePlugins.tpl cmd/mr/client/client.go cmd/mr/commands/plugins.go cmd/mr/commands/plugins_test.go cmd/mr/commands/plugins_help/plugin_enable.md
git commit -m "feat: require explicit persistent command consent"
```

---

### Task 4: Add durable run, output, import and map records

**Files:**
- Create: `models/plugin_command_run_model.go`, `models/plugin_command_import_model.go`.
- Create: `plugin_commands/types.go`, `plugin_commands/store.go`.
- Create: `application_context/plugin_command_store.go`, `application_context/plugin_command_store_test.go`, `application_context/plugin_command_store_pg_test.go`.
- Modify: `application_context/user_admin_guard.go` (`stampedModels`).
- Modify: `plugin_system/manager.go`; create `plugin_system/generation_test.go`.
- Modify AutoMigrate inventories: `main.go`, `server/openapi/drift_test.go`, `server/api_tests/api_test_utils.go`, `server/api_tests/pg_test_helper_test.go`, and focused test harnesses that mirror production models.

**Interfaces:**

```go
// models status constants use the exact vocabularies in Global Constraints.
type PluginCommandRun struct {
    ID string `gorm:"primaryKey;size:32"`
    PluginName string `gorm:"index;size:50;not null"`
    CommandName string `gorm:"size:50;not null"`
    ParamsJSON string `gorm:"type:text;not null"`
    Status string `gorm:"index;size:16;not null"`
    ExitCode *int
    Error string `gorm:"type:text"`
    ProcessGroupID *int
    CancelRequested bool
    OutputUnverified bool
    ActorlessAtSubmission bool
    CreatedByUserId *uint `gorm:"index"`
    CreatedAt time.Time `gorm:"index"`
    StartedAt *time.Time
    FinishedAt *time.Time `gorm:"index"`
}

type PluginCommandRunOutput struct {
    RunID string `gorm:"primaryKey;size:32"`
    ArgvJSON string `gorm:"type:text;not null"`
    OutputTail string `gorm:"type:text"`
    CreatedAt time.Time `gorm:"index"`
}

type PluginCommandImport struct {
    ID string `gorm:"primaryKey;size:32"`
    RunID string `gorm:"index;size:32;not null"`
    FileName string `gorm:"size:255;not null"`
    PluginGeneration uint64
    CreatedByUserId *uint `gorm:"index"`
    Status string `gorm:"index;size:16;not null"`
    Error string `gorm:"type:text"`
    CreatedAt time.Time `gorm:"index"`
    StartedAt *time.Time
    FinishedAt *time.Time
}

type PluginCommandImportMap struct {
    ID uint `gorm:"primaryKey"`
    RunID string `gorm:"uniqueIndex:idx_command_import_file;size:32;not null"`
    FileName string `gorm:"uniqueIndex:idx_command_import_file;size:255;not null"`
    ImportID string `gorm:"index;size:32;not null"`
    ResourceID *uint
    Status string `gorm:"index;size:16;not null"`
    Error string `gorm:"type:text"`
}

// plugin_commands/store.go: application_context supplies the GORM adapter.
var (
    ErrRunNotFound = errors.New("command run not found")
    ErrRunNotCancellable = errors.New("command run is not cancellable")
)

type Store interface {
    CreateRun(RunRecord, RunOutput) error
    MarkRunRunning(id string, started time.Time) (bool, error)
    SetRunProcessGroup(id string, pgid int) error
    RequestRunCancel(id, reason string) error
    FinishRun(id string, finish RunFinish) (bool, error)
    Run(id string) (RunRecord, RunOutput, error)
    Runs(Access) ([]RunView, error)
    NonterminalRuns() ([]RunRecord, error)
    ExpiredTerminalRuns(before time.Time) ([]RunRecord, error)
    PruneRunOutputs(before time.Time) (int64, error)

    ImportMap(runID, name string) (ImportMapEntry, bool, error)
    ClaimImport(ImportClaimRequest) (ImportClaimResult, error)
    MarkImportRunning(importID string, started time.Time) (bool, error)
    FinishImport(importID string, finish ImportFinish) (bool, error)
    InterruptNonterminalImports(time.Time) error
    NonterminalImports() ([]ImportRecord, error)
    HasNonterminalImports(runID string) (bool, error)
}
```

`plugin_commands/types.go` defines the plain domain records used by this interface: `RunRecord`, `RunOutput`, `RunFinish`, `RunView`, `Access`, `ImportRecord`, `ImportMapEntry`, `ImportClaimRequest`, `ImportClaimResult` and `ImportFinish`. They contain the fields of the GORM models above but no `gorm` tags or database handle.

- [x] **Step 1: Add the plugin generation primitive before its consumers**

Add a monotonically increasing generation to each loaded `PluginInfo` and VM registration now, before run/import types store it. Expose `GenerationForState(L)` internally and `GenerationActive(plugin,generation)` for the application adapter. A disable/re-enable receives a different generation even when the manifest is unchanged. Test two consecutive loads, a revoked state, and active/mismatched lookups.

- [x] **Step 2: Write red transition and deleted-actor tests**

Test atomic run creation/queued→running→terminal transitions, cancellation latch, stale terminal writer refusal, nonterminal recovery query and output pruning without run deletion. Create both an ordinary actor-owned run and an intentional `ActorlessAtSubmission=true` run; delete the actor and assert `stampedModels` nulls the ordinary run/import columns, the ordinary run is inaccessible to every plugin principal, its pending import cannot start, and the intentional actorless run alone retains the documented plugin-wide access.

- [x] **Step 3: Write red import-claim state-table tests**

Use literal cases: absent→new pending; pending/running→same import ID; succeeded→resource ID; interrupted→same ID reset pending with refreshed generation/actor; failed/cancelled→new ID replacing the map. Race eight claimers on one `(run,name)` and assert one active claim.

- [x] **Step 4: Implement the store with conditional updates and transactions**

Every transition includes its expected prior status in `WHERE`. `RequestRunCancel` atomically stores `cancel_requested=true` plus the reason in `Error` only on `queued`/`running` rows with `cancel_requested = false`, and returns typed `ErrRunNotFound` versus `ErrRunNotCancellable`; it never reports a terminal row as successfully cancelled. `FinishRun` updates output tail and terminal row in one transaction. Claim logic locks/creates the map row in a transaction and uses the composite unique index as the final race guard. Keep old claim rows when a failed/cancelled map entry is replaced.

- [x] **Step 5: Add PostgreSQL race coverage**

Under the existing `postgres` tag, run concurrent run-start, terminal and import-claim cases. Assert exactly one winner rather than relying on SQLite writer serialization.

- [x] **Step 6: Wire migration and fail-closed ownership inventories**

Add all four models to production and test AutoMigrate lists. Add run/import claim models (the two with `CreatedByUserId`) to `stampedModels`; map/output rows are not actor-owned. In `user_admin_guard.go`, document the invariant beside the existing fail-closed models: nulling a run creator never flips `ActorlessAtSubmission`, so deleted-user runs become inaccessible; a null import actor cannot pass worker-start revalidation or be re-driven through that inaccessible run.

- [x] **Step 7: Verify and commit**

```bash
go test --tags 'json1 fts5' ./models ./plugin_system ./application_context ./server/openapi ./server/api_tests -run 'PluginCommand|PluginGeneration|StampedModels' -count=1
go test --tags 'json1 fts5 postgres' ./application_context -run 'PluginCommand.*PG' -count=1
git diff --check
git add models/plugin_command_* plugin_commands/types.go plugin_commands/store.go plugin_system/manager.go plugin_system/generation_test.go application_context/plugin_command_store* application_context/user_admin_guard.go main.go server/openapi/drift_test.go server/api_tests/api_test_utils.go server/api_tests/pg_test_helper_test.go
git commit -m "feat: persist plugin command runs and imports"
```

---

### Task 5: Add managed live jobs and dedicated dispatch pools

**Files:**
- Modify: `download_queue/generic_job.go`, `download_queue/job.go`, `download_queue/manager.go`, `download_queue/errors.go`.
- Create/extend: `download_queue/managed_job_test.go`.
- Modify: `plugin_commands/types.go`.
- Create: `plugin_commands/dispatcher.go`, `plugin_commands/dispatcher_test.go`.

**Interfaces:**

```go
// download_queue
const MaxManagedLiveJobs = 6 // four command workers + two import workers

type JobControls struct { Cancel, Pause, Resume, Retry bool }
type ManagedJobOutcome struct {
    Status JobStatus
    AuthoritativeStatus string
    Error string
}
type ManagedJobRunFn func(context.Context, *DownloadJob, ProgressSink) ManagedJobOutcome
type ManagedJobOptions struct {
    JobOptions
    Controls JobControls
    Cancel func(reason string) error
}
func (m *DownloadManager) SubmitManagedJob(ManagedJobOptions, ManagedJobRunFn) (*DownloadJob, error)

// plugin_commands
type Settings interface {
    StagingRoot() string
    PendingPerPluginLimit() int
    PerRunQuota() int64
    GlobalStagingQuota() int64
    ExchangeRetention() time.Duration
    OutputRetention() time.Duration
    CommandPath() string
}

type RunJobSpec struct {
    RunID string
    PluginName string
    OwnerUserID *uint
}
type ImportJobSpec struct {
    ImportID string
    PluginName string
    OwnerUserID *uint
}
type Progress interface { SetPhase(string); SetPhaseProgress(int64, int64) }
type Outcome struct { Status, Error string }

type LiveJobs interface {
    SubmitCommandJob(RunJobSpec, func(string) error, func(context.Context, Progress) Outcome) (string, error)
    SubmitImportJob(ImportJobSpec, func(context.Context, Progress) Outcome) (string, error)
}

type Result struct {
    OK bool
    ExitCode *int
    Error string
    RunID string
}
type CommandRequest struct {
    PluginName string
    PluginGeneration uint64
    ActorUserID *uint
    Declaration Declaration
    Params map[string]string
    Completion func(Result)
}
type QueuedRun struct {
    RunID string
    Request CommandRequest
    ExchangeDir string
    Invocation Invocation
}

type Executor interface {
    Execute(context.Context, QueuedRun) Outcome
}

type Dependencies struct {
    Store Store
    Jobs LiveJobs
    Executor Executor
    Settings Settings
    Logf func(string, ...any)
}

func NewDispatcher(Dependencies) *Dispatcher
func (d *Dispatcher) Start(context.Context) error
func (d *Dispatcher) Stop(context.Context) error
func (d *Dispatcher) Submit(CommandRequest) (string, error)
func (d *Dispatcher) Cancel(runID, reason string) error
```

`DownloadJob` gains private controls, a private managed-lane marker and serialized `authoritativeStatus,omitempty`. Managed jobs bypass the download semaphore and ordinary 100-job admission count; their lane is independently capped at six. The dispatcher does not call `SubmitManagedJob` until a command/import owns an execution slot, so pending durable rows live only in its per-plugin queues.

- [x] **Step 1: Write red managed-lane isolation tests**

Fill the ordinary registry with 100 parked downloads, then register six dispatched managed jobs and assert all six start without the download semaphore and without evicting/refusing an ordinary entry. Assert a seventh concurrent managed job is refused by the managed-lane cap. In a fresh manager with six managed jobs parked, admit 100 ordinary jobs and assert only the 101st ordinary job is refused, proving the full ordinary budget remains. Exercise the two independent removal implementations as separate cases. First fill the managed lane with one terminal entry and assert the next managed admission removes it through `evictJob` and succeeds. Then use a fresh manager, age a terminal managed entry past retention, invoke `cleanupOldJobs` directly (its in-place `delete(dm.jobs, id)`/`jobOrder` rebuild path), and assert the next managed admission succeeds. Neither case adds counter bookkeeping. Also assert pause/resume/retry return `StateConflictError`, Cancel invokes the managed job's injected callback outside `dm.mu`/`job.mu` (the test callback re-enters `Snapshot` so either held lock deadlocks the test), and terminal `authoritativeStatus` is preserved in snapshots/SSE.

- [x] **Step 2: Implement a derived managed-live lane**

Keep `SubmitJobWithOptions` behavior for downloads/exports/imports and make `makeRoomForNewJob` count only ordinary entries. Under `dm.mu`, `SubmitManagedJob` derives managed occupancy by scanning `m.jobs` for the private managed marker; `m.jobs` is the sole owner/source of truth and there is **no managed counter**. At the six-entry cap it removes an oldest terminal managed entry through `evictJob` before recounting/reuse, registers/announces identically, bypasses `m.semaphore`, applies its explicit outcome atomically and notifies once. Because occupancy is derived, both independent removal paths release capacity without bookkeeping: managed admission's `evictJob` call, and `cleanupOldJobs`' existing direct `delete(dm.jobs, id)` plus in-place `jobOrder` rebuild. Existing downloads retain all current controls. A cancel-enabled managed job supplies a callback which `DownloadManager.Cancel` invokes outside manager/job locks instead of cancelling an opaque generic context; the command adapter binds that callback to `Dispatcher.Cancel(runID, reason)`, so live cockpit and admin-history cancellation share `Store.RequestRunCancel` and the pre-fork latch. Imports supply no running cancel control. Total registry growth is bounded at 106 live entries, never at 100 plus an unbounded command queue.

- [x] **Step 3: Write red dispatcher queue/registration tests**

Submit interleaved runs for plugins A/B/C. Park workers and assert no plugin exceeds two, global active never exceeds four, each plugin starts in submission order, and a blocked A does not prevent eligible B. Fill plugin A's pending command queue to 100 and assert A's 101st is refused without a DB row/folder leak while plugin B is still admitted. Assert the fake `LiveJobs` adapter receives **zero** registrations for pending work and exactly one only after a dispatcher slot is assigned. Cancel one queued A run through `Dispatcher.Cancel`: assert `Store.RequestRunCancel`, removal from the private queue, terminal `cancelled`, callback delivery, and admission of a replacement — still with zero live-job registration. Mirror the per-plugin 100-pending cap for imports.

- [x] **Step 4: Implement dispatcher-owned per-plugin queues**

Use one owner goroutine and a wake channel, not a goroutine per queued item. Keep a bounded FIFO queue per plugin for commands and imports using `Settings.PendingPerPluginLimit()`; the application adapter returns `download_queue.MaxQueueSize`, preserving the shared numerical policy without making leaf package `plugin_commands` import `download_queue`. Track `activeGlobal`, `activeByPlugin`, running cancel functions and run-id queue membership, and schedule round-robin across eligible plugin queue heads without reordering within one plugin. `Cancel` first persists `RequestRunCancel`; for a still-queued entry the owner goroutine removes it, finishes the durable row as `cancelled` and delivers its callback without registering a live job; for active work it invokes the tracked cancel function. Acquire the command/import execution slot first, then call `SubmitManagedJob`; a registry-lane refusal releases the slot and records dispatch failure rather than leaving an invisible running row.

- [x] **Step 5: Verify concurrency under the race detector**

```bash
go test -race ./download_queue ./plugin_commands -run 'ManagedJob|Dispatcher' -count=1
git diff --check
```

- [x] **Step 6: Commit**

```bash
git add download_queue plugin_commands/types.go plugin_commands/dispatcher.go plugin_commands/dispatcher_test.go
git commit -m "feat: add dedicated plugin command dispatch pools"
```

---

### Task 6: Execute commands with hardened argv, environment, output and quotas

**Files:**
- Create: `plugin_commands/runner_unix.go`, `plugin_commands/runner_windows.go`, `plugin_commands/output_tail.go`, `plugin_commands/usage.go`.
- Create: `plugin_commands/runner_test.go`, `plugin_commands/helper_process_test.go`, `plugin_commands/quota_test.go`.
- Modify: `plugin_commands/dispatcher.go`.

**Interfaces:**

```go
// CommandRequest and Result are the Task 5 types. The real executor added here
// consumes QueuedRun and returns the explicit durable Outcome.
type ProcessInspector interface {
    InspectGroup(pgid int, runID string) (GroupIdentity, error)
    KillGroup(pgid int) error
}

type RunnerDependencies struct {
    Store Store
    Settings Settings
    Inspector ProcessInspector
    Logf func(string, ...any)
}
func NewExecutor(RunnerDependencies) Executor
```

- [ ] **Step 1: Build a real helper-process harness and red argv/env/stdin tests**

Use `os.Executable()` plus a temporary basename symlink in the configured command path; a helper mode in `TestMain` records argv/env/stdin/cwd, can emit interleaved stdout/stderr, write bytes, spawn a same-group descendant and sleep. Put a same-named rogue executable earlier in the process's inherited `PATH` and assert it is ignored when absent from `Settings.CommandPath()`. Assert `exec.Cmd.Args` equals the literal substituted vector and no shell process appears.

- [ ] **Step 2: Implement private run directories and invocation records**

Create `<root>/plugin_exchange/<plugin>/<run-id>` and a private `<exchange-dir>/.tmp` with `0700`. Build/redact argv once, insert the queued run/output rows, and persist only redacted values. Reject global-quota admission before the row/folder becomes visible; clean up both if later queue admission fails.

- [ ] **Step 3: Implement Unix spawning**

Resolve `argv[0]` with a private `resolveExecutable(base, Settings.CommandPath())` that walks only the validated configured directories, requires an executable regular file, and never consults the ambient process `PATH`. Copy the configured value into the child `PATH`. Set `cmd.Path` to the resolved path while keeping `cmd.Args[0]` as the manifest basename. Set `cmd.Dir`, explicit environment, `os.DevNull`, stdout/stderr pipes and `SysProcAttr=&syscall.SysProcAttr{Setpgid:true}`. Mark the row running before `Start`; persist pgid immediately after `Start`.

- [ ] **Step 4: Add the bounded combined tail**

Drain both pipes concurrently into one mutex-protected 64 KiB ring. Strip terminal control characters before persistence, but preserve ordinary newlines/text. Do not HTML-escape in storage; escape at render time so JSON clients receive text rather than HTML entities.

- [ ] **Step 5: Implement timeout, cancellation and group-death finalization**

Use a timer starting after spawn. Operator/plugin cancellation kills `-pgid` and records `cancelled`; timeout kills it and records `failed` with timeout named. Wait for group death and both pipe drains before final output. Exit 0→`succeeded`; nonzero/cannot-start→`failed`. `runner_windows.go` returns the documented unsupported error without spawning.

- [ ] **Step 6: Enforce sampled quotas**

Every second, compute run exchange plus that run's import-temp usage without following symlinks. Above 8 GiB/configured per-run quota, kill group and fail naming the quota. Before each new run, measure the full staging root; above global quota refuse command admission but do not refuse imports. Include import temps in both counters.

- [ ] **Step 7: Test descendants and final output**

Have the helper spawn a long-lived descendant that retains stdout and writes after the parent exits. Assert timeout/cancel kills the descendant, no bytes arrive after terminal publication, and output tail remains exactly the last 64 KiB. Assert fast writes can overshoot the sampled quota but are killed at the next sample.

- [ ] **Step 8: Verify and commit**

```bash
go test -race ./plugin_commands -run 'Runner|ProcessGroup|Quota|OutputTail|Environment' -count=1
git diff --check
git add plugin_commands
git commit -m "feat: execute plugin commands in managed process groups"
```

---

### Task 7: Recover crashed runs and coordinate disable/shutdown

**Files:**
- Create: `plugin_commands/process_linux.go`, `plugin_commands/process_darwin.go`, `plugin_commands/process_other_unix.go`, `plugin_commands/recovery.go`.
- Create: `plugin_commands/recovery_test.go`, `plugin_commands/disable_race_test.go`.
- Modify: `plugin_commands/dispatcher.go`, `application_context/plugin_state_context.go`.

**Interfaces:**

```go
type GroupState int
const (
    GroupDead GroupState = iota
    GroupAliveOwned
    GroupAliveUnverified
)

type GroupIdentity struct {
    State GroupState
    PIDs []int
}

func (d *Dispatcher) Recover(context.Context) error
func (d *Dispatcher) DisablePlugin(string, string) error
```

- [ ] **Step 1: Write red recovery matrix tests with a fake inspector**

Literal cases: queued/cancel requested→cancelled without spawn; queued/no cancellation→interrupted verified; running/cancel requested with owned group→kill then cancelled with persisted reason; running/cancel requested but unverifiable group→interrupted + output_unverified; running/no pgid→interrupted + output_unverified; running/dead pgid→interrupted; running/owned live group→recheck, kill, recheck, interrupted; running/reused pgid→no kill, interrupted + output_unverified. File operations on unverified output are covered in Task 8.

- [ ] **Step 2: Implement Linux and macOS identity inspection**

Linux enumerates `/proc/*/stat` for matching process group IDs and reads `/proc/<pid>/environ` for `MAHR_COMMAND_RUN_ID`. Darwin uses `sysctl` process listings plus `KERN_PROCARGS2` to inspect each group member's environment. Unsupported Unix inspectors return unverifiable rather than guessing ownership.

- [ ] **Step 3: Close the pre-fork cancellation races**

Make `Dispatcher.Cancel` the single per-run cancellation primitive used by both an administrator request and `DisablePlugin`. It persists `RequestRunCancel` and sets the atomic latch before taking the dispatch mutex: while queued it removes/finishes without a spawn; before fork it cancels without `Start`; after fork it waits for pgid persistence, then identity-checks, kills/reaps and publishes `cancelled` with the supplied reason. `DisablePlugin` iterates its runs through that primitive with reason `plugin disabled`. Test direct operator and disable barriers immediately before `Start` and immediately after `Start`/before pgid persistence.

- [ ] **Step 4: Integrate plugin disable**

After `PluginManager.DisablePlugin` successfully revokes the VM, `SetPluginEnabled(false)` calls `Dispatcher.DisablePlugin`. Queued command runs/imports become cancelled; running command groups are killed; running imports are not cancelled and may finish their commit. If cancellation persistence fails, log to the application log and return the failure rather than report a complete disable while command execution remains active.

- [ ] **Step 5: Implement shutdown classification**

Stop admission, finish rows with a pre-existing durable cancel request as `cancelled`, mark every other queued command/import interrupted, terminate other running command groups with shutdown reason, and wait boundedly for workers. Import workers use a cancel-aware source reader so pre-commit copies stop; terminal recording is drained before `Stop` returns. Shutdown outcomes are `interrupted`, never operator `cancelled`.

- [ ] **Step 6: Mutation-check identity before kill**

Temporarily remove the run-id environment comparison and verify the pgid-reuse test observes the forbidden kill. Restore it and run:

```bash
go test -race ./plugin_commands ./application_context -run 'Recovery|Cancel.*Fork|Disable.*Fork|Shutdown.*Command' -count=1
git diff --check
```

- [ ] **Step 7: Commit**

```bash
git add plugin_commands application_context/plugin_state_context.go application_context/*disable*test.go
git commit -m "feat: recover and terminate plugin command process groups"
```

---

### Task 8: Implement safe exchange-folder operations and leases

**Files:**
- Create: `plugin_commands/exchange.go`, `plugin_commands/exchange_unix.go`, `plugin_commands/exchange_windows.go`, `plugin_commands/leases.go`.
- Create: `plugin_commands/exchange_test.go`, `plugin_commands/sweep_race_test.go`.

**Interfaces:**

```go
const (
    MaxListEntries = 10_000
    MaxReadBytes = 4 << 20
    MaxFileNameBytes = 255
)

type Entry struct { Name string; Size int64; Modified time.Time }
type Listing struct { Entries []Entry; Truncated bool }

type Exchange interface {
    List(Access, string) (Listing, error)
    Read(Access, string, string, int64) ([]byte, error)
    Discard(Access, string, string) error
    DiscardRun(Access, string) error
}

func NewExchange(Store, Settings) Exchange
```

- [ ] **Step 1: Write red ownership/state/name tests**

Test wrong plugin, wrong actor, an intentional `ActorlessAtSubmission=true` run (allowed to any current principal acting for that plugin), and a deleted-actor run (`CreatedByUserId=nil`, flag false) that remains inaccessible. Also cover queued/running run, swept run, `output_unverified`, empty/`.`/`..`/slashes/backslashes/NUL/overlong names, and cross-run IDs. Assert errors use the spec's explicit phrases.

- [ ] **Step 2: Write red filesystem attack tests**

Create top-level regular files, directory, FIFO/device where supported and a symlink to an outside secret. Add a barrier after `lstat`, swap a regular file for the symlink, and assert read/discard/import open refuses it and never reads/removes the target.

- [ ] **Step 3: Implement lexical and relative-open enforcement**

Open the run directory fd, call `unix.Openat` with `O_NOFOLLOW|O_CLOEXEC`, then `Fstat` and require regular mode. For delete, verify with relative no-follow open before `Unlinkat`. Windows implementations return unsupported because v1 cannot express the same reparse-point guarantee.

- [ ] **Step 4: Implement list/read/discard/discard_run**

List only top-level entries and return the first 10,000 regular-file records plus `Truncated`; do not follow or expose symlink targets. Read requires caller `max_bytes` in `1..4MiB` and reads `max_bytes+1` to refuse overflow. `discard_run` refuses nonterminal runs/imports and active leases; it alone permits `output_unverified`.

- [ ] **Step 5: Coordinate leases and sweep locks**

A per-run lock owns lease acquisition and sweep's check/delete decision. File operations take a file lease; import claims pin at admission before waiting for the pool. Prove a sweep cannot delete after its skip-check but before an operation acquires its lease.

- [ ] **Step 6: Verify and commit**

```bash
go test -race ./plugin_commands -run 'Exchange|Symlink|Lease|ListTruncates|DiscardRun' -count=1
git diff --check
git add plugin_commands
git commit -m "feat: mediate plugin command exchange folders"
```

---

### Task 9: Dispatch durable, replayable resource imports

**Files:**
- Create: `plugin_commands/imports.go`, `plugin_commands/imports_test.go`.
- Create: `application_context/plugin_command_import.go`, `application_context/plugin_command_import_test.go`.
- Modify: `application_context/resource_upload_context.go` to consume Task 1's private scratch option.
- Modify: `plugin_commands/dispatcher.go`, `plugin_commands/exchange.go`.

**Interfaces:**

```go
type ResourceFields struct {
    Name string
    Description string
    TagIDs []uint
    GroupIDs []uint
    Meta map[string]any
}

type Importer interface {
    ValidateImport(ImportValidation) error
    ImportResource(context.Context, ImportSource, ResourceFields, string) (uint, error)
}

type ImportSubmission struct {
    Access Access
    RunID string
    Name string
    Fields ResourceFields
    PluginGeneration uint64
    ActorUserID *uint
    Completion func(ImportResult)
}

type ImportSubmitResult struct {
    ImportID string
    ResourceID *uint
}

func (d *Dispatcher) SetImporter(Importer)
func (d *Dispatcher) SubmitImport(ImportSubmission) (ImportSubmitResult, error)
```

- [ ] **Step 1: Write red lifecycle tests through the dispatcher**

Cover every state-table row from Task 4 at the public submission seam, pool admission failure→failed (not pending), restart pending/running→interrupted, interrupted resubmission same ID, failed/cancelled new ID, succeeded source-deleted short-circuit and no callback on a synchronous short-circuit.

- [ ] **Step 2: Snapshot the source inside the claim temp**

After map short-circuit and safe file open, atomically claim/pin, create `<root>/import_tmp/<import-id>/`, and copy the source into a unique outer snapshot there. Call `addResourceWithOptions` with the same claim directory so its inner `upload-*` snapshot is also managed/accounted. Always derive hash/MIME/size from the current snapshot.

- [ ] **Step 3: Implement actor/generation validation at worker start**

The application adapter verifies the plugin generation is still active, both `commands` and `db:write` remain granted, the stored actor is non-nil and still resolves to an enabled account with write role, and every requested group is inside scope (tags are global). A nil actor on an import claim — including one nulled by user deletion — cancels the claim fail-closed. An intentional actorless run may be opened by a current plugin principal, but `create_resource` stamps the **current** principal onto the new claim before enqueue. Bind that principal before calling `AddResource`; copy actor pointers before GORM stamping.

- [ ] **Step 4: Implement disable and shutdown boundaries**

Disable cancels pending imports with `plugin disabled`; a worker already marked running completes and records success/failure. Shutdown makes pending imports interrupted and cancels running pre-commit reads so they record interrupted. A disable/re-enable never auto-retries cancelled imports.

- [ ] **Step 5: Delete source only after durable success**

Record resource ID/status in the map before removing the exchange file. If deletion fails, keep the map succeeded and record/log `imported-pending-delete`; later submission returns the resource ID without requiring the file. Sweep may delete the leftover bytes.

- [ ] **Step 6: Account and recover import temp directories**

Per-run usage includes claim temp dirs for that run; global usage includes all `import_tmp`. Terminal cleanup removes claim temp immediately. Startup marks nonterminal claims interrupted and deletes their partial temps; add a fixture representing a crash after the inner `upload-*` file was populated.

- [ ] **Step 7: Prove MemoryFS and same-content concurrency**

Run a real staging file through the adapter into `afero.NewMemMapFs`; assert bytes are readable from the created resource. Run two imports of identical content concurrently and assert one valid backing file remains and both map entries resolve consistently through `AddResource` deduplication.

- [ ] **Step 8: Verify and commit**

```bash
go test -race --tags 'json1 fts5' ./plugin_commands ./application_context -run 'Import|MemoryFS|SameContent' -count=1
git diff --check
git add plugin_commands application_context/plugin_command_import.go application_context/plugin_command_import_test.go application_context/resource_upload_context.go
git commit -m "feat: import plugin command output asynchronously"
```

---

### Task 10: Expose `mah.commands` and `mah.fs` with generation-bound callbacks

**Files:**
- Create: `plugin_system/commands_api.go`, `plugin_system/fs_api.go`, `plugin_system/command_callbacks.go`.
- Create: `plugin_system/commands_api_test.go`, `plugin_system/fs_api_test.go`, `plugin_system/command_callback_test.go`.
- Modify: `plugin_system/manager.go`, `plugin_system/action_jobs.go`.
- Create: `application_context/plugin_command_context.go`, tests.
- Modify: `application_context/context.go` wiring.
- Extend: `internal/arch/plugin_capability_gate_test.go`, `internal/arch/plugin_db_chokepoint_test.go` where surfaces are enumerated.

**Interfaces:**

```go
// plugin_system seams implemented by application_context.
type CommandSubmitter interface {
    SubmitPluginCommand(plugin_commands.CommandRequest) (string, error)
}
type ExchangeMediator interface {
    CommandRuns(plugin_commands.Access) ([]plugin_commands.RunView, error)
    ListCommandFiles(plugin_commands.Access, string) (plugin_commands.Listing, error)
    ReadCommandFile(plugin_commands.Access, string, string, int64) ([]byte, error)
    SubmitCommandImport(plugin_commands.ImportSubmission) (plugin_commands.ImportSubmitResult, error)
    DiscardCommandFile(plugin_commands.Access, string, string) error
    DiscardCommandRun(plugin_commands.Access, string) error
}
```

- [ ] **Step 1: Write red Lua contract tests**

Assert `commands` installs both `mah.commands` and `mah.fs`; without it neither exists. `mah.fs.create_resource` exists only when `db:write` is also granted. Test argument/table types, literal return arity, transaction refusals, revoked VM refusal and Windows runtime refusal. Use the generation primitive already delivered by Task 4; this task must not introduce a second generation counter.

- [ ] **Step 2: Implement `mah.commands.run`**

Look up the declaration captured from the load-time manifest, parse string→string params without coercion, enforce counts before submission, capture actor/generation and wrap an optional callback. Return `run_id` or `(nil,error)`. The completion wrapper starts a tracked background callback, takes the original VM's exclusive lock, installs the captured actor invocation, applies `MaxAsyncJobDuration`, and passes exactly `{ok,exit_code,error,run_id}`.

- [ ] **Step 3: Implement `mah.fs` operations**

Build `Access{PluginName, ActorUserID}` from the live invocation. Convert `runs()` and listing/import maps to complete Lua tables. `create_resource` accepts exactly the existing resource-import fields `name`, `description`, `tags`, `groups` and `meta`; validates ID-shaped fields using `checkEntityIDOpts`; rejects unknown keys; requires no open transaction; captures generation/actor and optional `on_import`; and passes the documented result table. The already-succeeded synchronous short-circuit returns resource ID and does not call `on_import`.

- [ ] **Step 4: Track and drop callbacks safely**

Reuse the per-plugin in-flight wait group and VM-lock machinery, but not the action semaphore: callbacks are terminal notifications, not user jobs. `LockVM` returning nil drops the callback. Protect every callback with panic recovery and log timeout/error without altering the durable command/import outcome.

- [ ] **Step 5: Wire the application host**

Construct the dispatcher after context dependencies exist, provide `PendingPerPluginLimit()` from `download_queue.MaxQueueSize`, set command/exchange adapters on `PluginManager`, and leave calls unavailable until `StartPluginCommands` completes recovery. `WithPrincipal`/`WithTransaction` clones share the process-lifetime dispatcher pointer; the host rebuilds scoped DB handles per import, never captures a request DB.

- [ ] **Step 6: Verify and commit**

```bash
go test -race --tags 'json1 fts5' ./plugin_system ./application_context ./internal/arch -run 'Command|Exchange|CapabilityGate|Generation' -count=1
git diff --check
git add plugin_system application_context/plugin_command_context.go application_context/plugin_command_context_test.go application_context/context.go internal/arch
git commit -m "feat: expose plugin commands and exchange files to Lua"
```

---

### Task 11: Wire startup recovery, configuration, sweep and shutdown

**Files:**
- Modify: `application_context/context.go`, `context_test.go` helpers.
- Modify: `main.go` flag/env parsing and startup order.
- Create: `application_context/plugin_command_lifecycle_test.go`.
- Modify: `CLAUDE.md` configuration table.

**Interfaces/configuration:**

Add `MahresourcesConfig` fields and flags/env:

| Flag | Environment | Default |
| --- | --- | --- |
| `-plugin-command-path` | `PLUGIN_COMMAND_PATH` | snapshot of the server process `PATH`; entries must be nonempty absolute directories |
| `-plugin-command-staging-path` | `PLUGIN_COMMAND_STAGING_PATH` | `<file-save-path>/_plugin_commands`; a private process temp root for ephemeral MemoryFS |
| `-plugin-command-run-quota` | `PLUGIN_COMMAND_RUN_QUOTA` | `8589934592` (8 GiB) |
| `-plugin-command-staging-quota` | `PLUGIN_COMMAND_STAGING_QUOTA` | `53687091200` (50 GiB) |
| `-plugin-command-exchange-retention` | `PLUGIN_COMMAND_EXCHANGE_RETENTION` | `168h` |
| `-plugin-command-output-retention` | `PLUGIN_COMMAND_OUTPUT_RETENTION` | `720h` |

- [ ] **Step 1: Add red config/default tests**

Test flags/env precedence, positive quotas/retentions, explicit staging root, persistent default under `file-save-path`, and private temporary root for ephemeral MemoryFS. Snapshot inherited `PATH` only when neither flag nor env is set; refuse empty entries, relative entries and an empty configured value; prove an explicit path overrides a rogue inherited executable. Refuse an unusable/non-directory staging root at startup.

- [ ] **Step 2: Wire production startup in the correct order**

After AutoMigrate and runtime settings load, start the dispatcher and run recovery **before** `ActivateEnabledPlugins`, so no Lua callback/import can observe unresolved writers. Then activate plugins and start schedulers. Because startup occurs before the worker-defer block, register `defer context.StopPluginCommands()` immediately **after** the existing `defer context.DownloadManager().Shutdown()` at the worker block: LIFO then stops job events/scheduler first, the command dispatcher second, the download manager third, and the plugin manager last. This lets terminal managed-job publication drain while both downstream systems still exist.

- [ ] **Step 3: Run periodic sweep**

Give the dispatcher its own five-minute cleanup ticker and stop/drain it in `Dispatcher.Stop`; do not add command-specific callback slots to `download_queue`. Delete exchange folders only for terminal runs completed beyond retention and not pinned by leases/nonterminal imports. Delete only old output rows; never delete run/import/map rows. Clean orphan import temps at startup.

- [ ] **Step 4: Add lifecycle integration tests**

Seed queued/running rows and temp folders, start the host, assert recovery precedes a plugin page call, output pruning leaves durable maps, and shutdown stamps command/import outcomes before returning. Verify plugins-disabled mode creates no dispatcher goroutine and command calls remain unavailable.

- [ ] **Step 5: Document each setting and the merge-peak warning**

Add configuration rows and state that per-run quota must cover temporary merge peak (roughly 2× final output for separate video/audio plus mux), enforcement is sampled, commands have unrestricted process networking, and MemoryFS imports consume RAM. Document inherited `PATH` as a startup trust boundary and recommend pinning `-plugin-command-path`/`PLUGIN_COMMAND_PATH` to the minimal absolute trusted directories containing the declared executables and their helpers.

- [ ] **Step 6: Verify and commit**

```bash
go test --tags 'json1 fts5' ./application_context ./... -run 'PluginCommandLifecycle|PluginCommandConfig|RuntimeSettingRegistry' -count=1
go build --tags 'json1 fts5' ./...
git diff --check
git add main.go application_context/context.go application_context/*plugin_command*lifecycle* CLAUDE.md
git commit -m "feat: configure and recover plugin command staging"
```

---

### Task 12: Add administrator command history and authoritative job UI

**Files:**
- Create: `server/api_handlers/plugin_command_handlers.go`, tests.
- Create: `server/template_handlers/template_context_providers/plugin_command_history_context.go`, tests.
- Create: `templates/pluginCommandHistory.tpl`.
- Modify: `server/api_handlers/handler_interfaces.go`, `server/template_handlers/template_context_providers/context_interfaces.go`.
- Modify: `server/routes.go`, `server/routes_openapi.go`, `server/authz_policy.go`.
- Modify: `templates/managePlugins.tpl`, `templates/partials/downloadCockpit.tpl`.
- Modify: `src/components/downloadCockpit.js`, `src/components/downloadCockpit.test.ts`; rebuild `public/dist`.
- Create: `server/api_tests/plugin_command_history_test.go`, `e2e/tests/plugins/plugin-command-history.spec.ts`.

**Interfaces/routes:**

- `GET /v1/plugin/command-runs` — admin-only paginated run summaries.
- `GET /v1/plugin/command-run?id=<run-id>` — admin-only run + optional output row + imports.
- `POST /v1/plugin/command-run/cancel` — admin-only cancellation by `id`; JSON response or 303 back to history for a browser form.
- `GET /admin/plugin-command-runs` — admin-only HTML history.

- [ ] **Step 1: Write red authorization tests**

Create an admin-owned and user-owned command row. Assert admin can list/read both; editor/user/guest receive 403 even for their own actor ID; ordinary `/downloads` never includes command records. For a queued row, assert only an admin can POST cancel, the row becomes `cancelled` without ever appearing in `DownloadManager`, and a repeat/terminal cancel returns 409; missing IDs return 404. Add every exact path to `isSystemPath` tests.

- [ ] **Step 2: Implement bounded history queries and cancellation**

Order newest first, paginate with a fixed/default limit, omit output from list, and join output/imports only for detail. If output was pruned, return `outputAvailable=false` with the run intact. Add `CancelPluginCommandRun(id)` to the handler/context seam; it delegates to `Dispatcher.Cancel(id, "operator cancelled")`, maps `ErrRunNotFound` to 404 and `ErrRunNotCancellable`/state conflicts to 409, and returns JSON or a 303 history redirect without duplicating queue-state logic in the HTTP layer.

- [ ] **Step 3: Render escaped output and warning data**

The detail page renders redacted params/argv, status, timestamps, exit code, imports and `<pre>` output. Pongo2 escaping stays on; control characters were stripped before storage. Add a prominent note that program output can echo secrets despite parameter redaction. In both a queued list row and queued detail, render a keyboard-operable **Cancel queued run** form carrying the CSRF token and run ID; it submits to the dedicated admin endpoint. If dispatch wins the render/submit race, the same dispatcher latch cancels before fork or kills the verified group.

- [ ] **Step 4: Make live command jobs display durable authority**

For dispatched `source=plugin-command` jobs, cockpit status/label uses `authoritativeStatus`; offer Cancel only while running, never Pause/Resume/Retry. Pending dispatcher-owned rows do not appear in the live cockpit; they remain visible through `mah.fs.runs()` and cancellable by an administrator from command history. The manager returns 409 for forbidden controls even if a crafted request bypasses the UI. Link terminal command rows to the admin history detail only for admins.

- [ ] **Step 5: Add accessibility/browser assertions**

Verify warning headings, command table captions, live-region terminal announcement, keyboard-operable running-job cancel, keyboard-operable queued-history cancel, focus/announcement after the 303 result, no dead pause/retry controls and escaped output containing `<script>`/ANSI bytes. Run axe on the new admin page.

- [ ] **Step 6: Rebuild generated surfaces and commit**

```bash
npm run build-js
go run ./cmd/openapi-gen
go test --tags 'json1 fts5' ./server/... -run 'PluginCommand|SystemPath' -count=1
cd e2e && npm run test:with-server -- --project=default tests/plugins/plugin-command-history.spec.ts
cd ..
git diff --check
git add server templates src public/dist openapi.yaml e2e/tests/plugins/plugin-command-history.spec.ts
git commit -m "feat: show administrator plugin command history"
```

---

### Task 13: Prove the complete host interface and document plugin authorship

**Files:**
- Create: `plugin_system/command_integration_test.go`.
- Create: `server/api_tests/plugin_command_integration_test.go`.
- Modify: `docs-site/docs/features/plugin-system.md`, `docs-site/docs/features/plugin-lua-api.md`, `docs-site/docs/features/plugin-permissions.md`, `docs-site/docs/api/plugins.md`, `docs-site/docs/configuration/advanced.md`.
- Modify: `cmd/mr/commands/plugins_help/plugin_enable.md`; regenerate CLI docs.
- Modify: `CLAUDE.md` architecture/configuration sections with final invariant summary.
- Update: `docs/todo.md` review section.

**Interfaces:** This task changes no public shape. It proves the interfaces from Tasks 1–12 and records the contract the separate yt-dlp plugin repository will use.

- [ ] **Step 1: Add a real end-to-end plugin fixture**

A test plugin declares `commands` + `db:write`; its helper command writes one importable file and one discardable file. Drive `mah.commands.run` → completion callback → `mah.fs.list` → two queued operations → import callback. Assert the resource exists with the acting creator, the discarded file is gone, callbacks receive documented tables and the import ran while a plugin page answered without waiting for the VM lock.

- [ ] **Step 2: Add restart and callback-loss end-to-end cases**

Stop between command completion/import start, reconstruct dispatcher against the same DB/staging root, assert runs show interrupted claims, and call `create_resource` again to re-drive the same import ID. Disable the VM before terminal delivery and assert callback is absent while durable `runs()`/admin history remains correct.

- [ ] **Step 3: Document the author/operator contract**

Document manifest schema, capability interactions, exact Lua signatures/result tables, status vocabularies, callback cheapness, restart reconciliation, actor ownership, flat-file rules, limits, consent flow and no-sandbox/no-egress guarantees. Include the safe yt-dlp declaration (`--ignore-config`, `--paths`, `--no-playlist`, `--no-directories`, `--`, URL) as an example but do not ship the external plugin here.

- [ ] **Step 4: Regenerate docs and run freshness checks**

```bash
npm run docs-gen
npm run skills-gen
npm run build-js
./mr docs lint
./mr docs check-examples
./scripts/css-scan-test.sh
go run ./cmd/openapi-gen
```

- [ ] **Step 5: Run focused race, SQLite and PostgreSQL gates**

```bash
go test -race ./plugin_commands ./plugin_system ./download_queue
go test --tags 'json1 fts5' ./...
go test --tags 'json1 fts5 postgres' ./application_context/... ./server/api_tests/... -count=1
```

- [ ] **Step 6: Run complete browser and CLI E2E gates**

```bash
cd e2e && npm run test:with-server:all
cd e2e && npm run test:with-server:postgres
```

Read full outputs and `e2e/test-results/.last-run.json`; do not pipe test commands through `tail`/`tee` without preserving the producer's status.

- [ ] **Step 7: Review spec coverage and mutation-check load-bearing tests**

Re-read each spec section. Mutate, one at a time: shell-free argv; configured-path resolution (fall back to ambient `PATH`); legacy short-circuit order; intentional-actorless versus deleted-actor access; import claim idempotency; pgid identity check; `O_NOFOLLOW`; committed-row-before-repair; dispatcher/live-registry/download-capacity separation; replace derived managed occupancy with a counter and omit cleanup bookkeeping; and remove queued-history cancellation. Confirm the named test fails for each mutation, then restore and rerun it.

- [ ] **Step 8: Record final evidence and commit docs/tests**

Add a `docs/todo.md` review naming commands, counts, platform-specific gaps and any residual accepted pgid signal race. Commit only after fresh verification:

```bash
git status --short
git diff --check
git add docs-site cmd/mr/commands/plugins_help CLAUDE.md docs/todo.md plugin_system/command_integration_test.go server/api_tests/plugin_command_integration_test.go public/dist openapi.yaml
git commit -m "docs: complete plugin command host integration"
```

---

## Execution handoff to the separate yt-dlp plugin repository

After all core gates pass, create a separate design/plan in `yt-dlp-plugin-for-mahresources`. It consumes only the documented host interface. Its required tests are: fake `yt-dlp` end to end; extension filtering; `output_template` rejection of absolute paths, separators and `..`; callback that only lists/queues/discards; page-load reconciliation of interrupted imports independently of command outcome; explicit retry controls for failed/cancelled imports; and disposal of `output_unverified` runs.

## Final self-review checklist

- [ ] Every spec §1 manifest rule maps to Tasks 2–3.
- [ ] Every spec §2 consent/UI/CLI rule maps to Task 3.
- [ ] Every spec §3 dispatch/process/recovery/quota rule maps to Tasks 5–7 and 11.
- [ ] Every spec §4 history/redaction/authorization rule maps to Tasks 4, 6 and 12.
- [ ] Every spec §5 exchange/import/sweep rule maps to Tasks 1, 4 and 8–11.
- [ ] Spec §6 is explicitly handed to the separate repository, with the host fixture in Task 13.
- [ ] Spec §7 capability/actor rules map to Tasks 9–10.
- [ ] Every core test named in spec §8 appears in a task above.
- [ ] Spec §9 non-goals remain absent.
