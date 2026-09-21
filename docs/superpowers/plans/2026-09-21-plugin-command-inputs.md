# Plugin-Command Input Files Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a consented plugin supply named file contents to one declared command run; validate the names synchronously with nothing durable created, write the bytes into that run's private exchange folder immediately before the spawn, and record names and sizes while never recording contents.

**Architecture:** Add no new module. `plugin_commands` owns declaration parsing, the `InputFile` type, synchronous validation, the atomic write step and the `InputsJSON` record; `plugin_system` owns the manifest field, the consent record and the Lua options table; `application_context` persists one new column; `server` and the templates render names and sizes. The write goes into `commandExecutor.Execute`'s existing start path, after every cheap failure point and before any pipe exists.

**Tech Stack:** Go, GORM, SQLite/PostgreSQL, gopher-lua, Pongo2, Cobra, and the existing `e2e/test-plugins/test-commands` fixture.

**Spec:** `docs/superpowers/specs/2026-09-21-plugin-command-inputs-design.md`
**ADR:** `docs/adr/0005-plugin-command-inputs-are-declared-names.md`

## Global Constraints

- A supplied file is a **name in the run's own folder**. No paths: no `/`, `\`, `..`, NUL, `.` or `..`, no leading dot, at most 255 bytes each. The grammar is `plugin_commands`' existing exchange-name grammar plus the dot rule.
- At most 4 declared names per command; at most 4 supplied per run.
- At most 256 KiB per input, 512 KiB aggregate. Constant names: `MaxInputFiles`, `MaxInputBytes`, `MaxAggregateInputBytes`.
- Validation is synchronous and does no I/O: `nil, error` to Lua, no durable row, nothing written. It runs from the Lua pre-check and from `dispatcher.Submit`, the same two places `BuildInvocation` runs.
- The write happens in `commandExecutor.Execute`, after `MarkRunRunning`, `stopBeforeStart`, `normalizeCommandPath`, `resolveExecutable` and the `/dev/null` open, and **before the first pipe is created**. A queued cancellation therefore writes nothing at all, and a doomed spawn never puts a secret on disk.
- The write is stage-then-rename inside the run folder's existing `.tmp`: scratch at a host-chosen random name (0600), then `rename` to the declared name (0600). Rename is atomic within the tree, so the declared name is never partially visible.
- After all writes and before the spawn, `Lstat` each target: regular file, exactly the expected size, mode 0600. A mismatch refuses the spawn.
- A write or verification failure finishes the run **failed** through the existing outcome path with `write input file %q: <error>`, names the file, spawns nothing, removes that file's scratch, and leaves the folder to the retention sweep. No new cleanup path, no new stuck state.
- Contents never reach any record: not `ParamsJSON`, not `ArgvJSON`, not the output row, not logs. The run row gains `InputsJSON`, an ordered `[{"name":"cookies.txt","bytes":1234}]` array (empty string when nothing was supplied), holding **submitted** byte counts.
- Zero-byte contents are legal and write a zero-byte file. Absent ≠ empty.
- Contents are dropped from memory once they are on disk: the raw `map[string]string` is `clear`ed, the validated slice's contents are zeroed in place (the dispatcher's copies share the map and the slice's backing array), and the map is deep-cloned in `cloneCommandRequest`.
- Declaration identity compares input names as a **set**, like `sensitive_params`; order is display order only. Reordering must not force re-consent; adding, removing or renaming must.
- Consent copy must say the contents are not shown and not reviewable, and the capability label must keep the five phrases pinned by `TestCommandCapabilityCataloguesDescribeTheHostPrivilege`.
- `commandExecutor.Prepare`, `Submit`'s ordering (`Prepare` → queue admission → `CreateRun`), `BuildInvocation`, `RedactedArgv`, `ParamView`, `mah.fs`'s read surface and the Windows refusal are unchanged.
- Planning baseline: `1f949c18`. Work in an isolated worktree (`.worktrees/plugin-command-inputs`); one writer owns the checkout. Run `gh`/git only inside that worktree.

---

## Execution and file ownership map

| Module | Files and responsibility |
| --- | --- |
| Declaration | `plugin_commands/declaration.go`: `Declaration.Inputs`, the three limits, name validation, `InputFile`, `ValidateInputs`, set-based `SameDeclarations` |
| Manifest | `plugin_system/manifest.go`: `commands[].inputs` parsing, allowed-key set, capability requirement, identity through `Manifest.Equal` |
| Consent/review | `plugin_system/consent.go`, `capability_grants_test.go`, `templates/managePlugins.tpl`, `cmd/mr/commands/plugins.go`, `plugins_help/plugin_enable.md`: `CommandGrant.Inputs`, `CommandDisplay.Inputs`, label clause, both render sites, CLI print |
| Lua + plumbing | `plugin_system/commands_api.go`, `plugin_commands/{types,dispatcher}.go`: 2–4 arg signature, strict options table, `CommandRequest.Inputs`, `cloneCommandRequest` deep copy, `Submit` validation and quota check |
| Write step | `plugin_commands/runner_unix.go`: `writeSuppliedInputs`, stage/rename, post-write verification, content drop |
| Record/render | `models/plugin_command_run_model.go`, `plugin_commands/types.go`, `application_context/plugin_command_store.go`, `plugin_system/fs_api.go`, `server/template_handlers/template_context_providers/plugin_command_history_context.go`, `templates/pluginCommandHistory.tpl` |
| Proof/docs | `e2e/test-plugins/test-commands/*`, `e2e/tests/plugins/plugin-command-history.spec.ts`, four docs-site pages |

Tasks 1–2 establish the declared contract and informed consent. Tasks 3–4 establish the synchronous refusal and the atomic write. Tasks 5–6 make it visible to operators and prove it end to end.

---

### Task 1: Declare and compare input files

**Files:**
- Modify: `plugin_commands/declaration.go`, `plugin_commands/declaration_test.go`.
- Modify: `plugin_system/manifest.go`, `plugin_system/manifest_test.go`.

**Interfaces:**

```go
// plugin_commands/declaration.go
const (
    MaxInputFiles          = 4
    MaxInputBytes          = 256 << 10
    MaxAggregateInputBytes = 512 << 10
)

type Declaration struct {
    Name            string        `json:"name"`
    Argv            []string      `json:"argv"`
    Timeout         time.Duration `json:"timeout"`
    SensitiveParams []string      `json:"sensitive_params,omitempty"`
    Inputs          []string      `json:"inputs,omitempty"` // declared names, display order
}

// InputFile is one validated, supplied input. Content is the only place the
// bytes ever live outside the exchange folder.
type InputFile struct {
    Name    string
    Content []byte
}

func ValidateInputFileName(name string) error
func ValidateInputs(declaration Declaration, supplied map[string]string) ([]InputFile, error)
```

- [x] **Step 1: Add red tests for the declaration rules**

Cover: a valid declaration with two inputs; a duplicate name; `.tmp`, `.netrc` and a bare `.`; a 256-byte name; five names; `a/b`, `a\\b`, `""` and `..`; and `SameDeclarations` returning true when only the order changed and false when a name was added, removed or renamed. Assert `Declaration.Inputs` is copied, not aliased, by `SameDeclarations`-adjacent paths.

- [x] **Step 2: Run them and watch them fail**

Run: `go test ./plugin_commands -run 'TestInputFileName|TestInputDeclarationIdentity' -count=1`

Expected: FAIL — `Declaration.Inputs` and the limits do not exist.

- [x] **Step 3: Implement name validation and the limits**

`ValidateInputFileName` reuses `validExchangeComponent` plus `len(name) > MaxFileNameBytes` and adds `strings.HasPrefix(name, ".")`. `ValidateDeclaration` validates every declared name in order, refuses a duplicate, and refuses more than `MaxInputFiles`. `SameDeclarations` compares `Inputs` with `sameStringSet`.

- [x] **Step 4: Parse the manifest field**

Add `"inputs"` to `parseManifestCommand`'s `allowed` set and read it with `exactStringArray(value, label+".inputs")`; declaration validation does the rest. `Manifest.Equal` already delegates to `SameDeclarations`, so identity follows with no further change. The `commands` capability requirement already covers `inputs`, since the field lives on a `commands[]` entry.

- [x] **Step 5: Add red manifest tests**

Valid list parses; a dot name, a duplicate, a fifth name, a non-array and a non-string element each fail with a message naming `commands[1].inputs` or `commands[1]`; `SameDeclarations` reorder-equality is asserted through `Manifest.Equal`.

- [x] **Step 6: Implement and verify**

Run: `go test ./plugin_commands ./plugin_system ./internal/arch -count=1`
Run: `go test ./plugin_system -run 'TestManifestCommandInputs|TestCommand' -count=1` (the label test in `capability_grants_test.go` is still untouched and must pass)

- [x] **Step 7: Commit**

```bash
git add plugin_commands/declaration.go plugin_commands/declaration_test.go plugin_system/manifest.go plugin_system/manifest_test.go
git commit -m "feat: declare plugin command input file names"
```

---

### Task 2: Extend consent, the review panel and the CLI

**Files:**
- Modify: `plugin_system/consent.go`, `plugin_system/consent_test.go`, `plugin_system/consent_load_test.go`, `plugin_system/capability_grants_test.go`, `plugin_system/manifest.go` (the `CapCommands` label).
- Modify: `templates/managePlugins.tpl` (both render sites), `server/template_handlers/template_context_providers/plugin_manage_context.go` if it filters fields.
- Modify: `cmd/mr/commands/plugins.go`, `cmd/mr/commands/plugins_test.go`, `cmd/mr/commands/plugins_help/plugin_enable.md`.
- Modify: `server/api_handlers/plugin_command_consent_test.go` (the JSON shape assertion).

**Interfaces:**

```go
type CommandGrant struct {
    Name            string   `json:"name"`
    Argv            []string `json:"argv"`
    TimeoutSeconds  int64    `json:"timeout_seconds"`
    SensitiveParams []string `json:"sensitive_params,omitempty"`
    Inputs          []string `json:"inputs,omitempty"`
}

type CommandDisplay struct {
    Name           string   `json:"name"`
    DisplayArgv    string   `json:"argv"`
    TimeoutSeconds int64    `json:"timeoutSeconds"`
    Inputs         []string `json:"inputs,omitempty"`
}
```

- [x] **Step 1: Add red consent tests**

A stored record with no `inputs` and a declaration that has one produces `ChangedCommands`; adding a name to an existing record produces it; reordering produces nothing; an unchanged record loads. Assert the stored record round-trips through `Marshal`/`ParseGrants`. This is the compatibility proof that no migration is needed.

- [x] **Step 2: Implement the grant and display fields**

`GrantsFromManifest` copies `command.Inputs`; `sameCommandGrant` compares it with `sameStringSet`; `CommandDisplays` copies it.

- [x] **Step 3: Extend the capability label and its test**

Append the inputs clause to `CapabilityLabels[CapCommands]` and extend `TestCommandCapabilityCataloguesDescribeTheHostPrivilege` with the new phrases (`not shown`, `not reviewable`) while keeping all five existing phrases.

- [x] **Step 4: Render it in both panel sites and the CLI**

`templates/managePlugins.tpl`: both command lists (the warning panel and the confirm step) gain a line such as `Inputs (contents are not shown and are not reviewable): cookies.txt` when `command.Inputs` is non-empty, plus the retention sentence. `cmd/mr/commands/plugins.go`: the refusal struct gains `Inputs []string json:"inputs"` and the print loop gains one indented line per command. `plugins_help/plugin_enable.md`: one paragraph and, if the existing examples make it welcome, one example line.

- [x] **Step 5: Verify**

Run: `go test ./plugin_system ./server/api_handlers ./cmd/mr -count=1`
Run: `go test ./plugin_system -run 'TestConsent|TestGrant|TestCommandCapability' -count=1`

- [x] **Step 6: Commit**

```bash
git add plugin_system/consent.go plugin_system/consent_test.go plugin_system/consent_load_test.go plugin_system/capability_grants_test.go plugin_system/manifest.go templates/managePlugins.tpl cmd/mr/commands/plugins.go cmd/mr/commands/plugins_test.go cmd/mr/commands/plugins_help/plugin_enable.md server/api_handlers/plugin_command_consent_test.go
git commit -m "feat: consent to plugin command input file names"
```

---

### Task 3: The Lua options table and synchronous validation

**Files:**
- Modify: `plugin_commands/declaration.go`, `plugin_commands/declaration_test.go` (the `ValidateInputs` cases).
- Modify: `plugin_commands/types.go`, `plugin_commands/dispatcher.go`, `plugin_commands/dispatcher_test.go`.
- Modify: `plugin_system/commands_api.go`, `plugin_system/commands_api_test.go`.

**Interfaces:**

```go
type CommandRequest struct {
    // ...existing fields...
    Inputs map[string]string // raw from Lua; cleared once written
}

type QueuedRun struct {
    // ...existing fields...
    Inputs []InputFile // validated, declaration order, source of truth for the write
}
```

- [x] **Step 1: Add red `ValidateInputs` tests**

Undeclared name (message names file and command), non-string impossible at this layer but an undeclared one possible, over `MaxInputFiles`, over `MaxInputBytes`, over `MaxAggregateInputBytes`, and the returned order equal to declaration order regardless of map iteration. Also: a declaration with no `inputs` refuses every supplied key.

- [x] **Step 2: Implement `ValidateInputs`**

Return `[]InputFile` in declaration order, skipping names not supplied. Message strings exactly as the spec's §2 table. Never build a string from the contents.

- [x] **Step 3: Add red Lua-surface tests**

`run(name, params)` and `run(name, params, cb)` still work; `run(name, params, nil, {inputs={...}})` works; a 5th argument raises; `input = {...}` raises with `unknown field`; a non-table options argument raises; a number value raises; a string callback in the options slot still works as today; supplying a declared name writes nothing yet and returns a run id; an undeclared name returns `nil, err` and creates no run.

- [x] **Step 4: Implement the Lua surface**

`commands_api.go`: widen the arity guard to 2–4, keep argument 3 as callback-or-nil, read argument 4 as the options table with an allowed-key set of exactly `inputs`, and convert the sub-table to `map[string]string` with the existing `checkCommandParams` shape of checks (string keys and values). Call `plugin_commands.ValidateInputs` next to the existing sentinel `BuildInvocation` pre-check so both are proven before the host sees the request.

- [x] **Step 5: Plumb it through `Submit`**

`cloneCommandRequest` deep-copies `Inputs` (`cloneStringMap`) and copies `Declaration.Inputs`. `Submit` calls `ValidateInputs`, refuses when the aggregate exceeds `d.deps.Settings.PerRunQuota()` with the spec's aggregate message, sets `run.Inputs`, and only then runs the existing `Prepare` → queue → `CreateRun` sequence. Nothing is written to disk here.

- [x] **Step 6: Verify**

Run: `go test ./plugin_commands ./plugin_system -run 'TestInput|TestCommandsAPI|TestSubmit' -count=1`
Run: `go test ./plugin_commands ./plugin_system -count=1`

- [x] **Step 7: Commit**

```bash
git add plugin_commands/declaration.go plugin_commands/declaration_test.go plugin_commands/types.go plugin_commands/dispatcher.go plugin_commands/dispatcher_test.go plugin_system/commands_api.go plugin_system/commands_api_test.go
git commit -m "feat: validate supplied command inputs synchronously"
```

---

### Task 4: Write the inputs immediately before the spawn

**Files:**
- Modify: `plugin_commands/runner_unix.go`.
- Create: `plugin_commands/inputs_runner_test.go` (`//go:build !windows`, like the other unix runner tests).
- Modify: `plugin_commands/helper_process_test.go` if the fixture needs a mode that echoes a file.

**Interfaces:**

```go
// Test seams, nil in production, in the shape of exchangeService.afterLstat.
type commandExecutor struct {
    // ...existing fields...
    writeInputFileFn    func(dir, name string, content []byte) error // test injection
    afterInputScratchFn func()                                       // test barrier
}

func (e *commandExecutor) writeSuppliedInputs(ctx context.Context, run QueuedRun) (Outcome, bool)
func writeInputFile(exchangeDir, name string, content []byte) error
func verifyInputFile(exchangeDir, name string, size int64) error
func dropInputContents(run *QueuedRun)
```

- [x] **Step 1: Red test — present before spawn, mode 0600 (spec §9.1)**

A fixture command reads `notes.txt` from its working directory, writes what it read to `seen.txt`, and prints a fixed token. Assert `seen.txt` holds the exact supplied bytes, `notes.txt` is 0600 and exactly the supplied size, and the output tail contains only the token.

- [x] **Step 2: Red tests — the failure and crash paths (spec §9.9, §9.10, §9.15, §9.16, §9.17)**

With `writeInputFileFn` returning an error: the run finishes `failed`, `Error` names the file and its command name is on the same row, nothing was spawned, and no declared name exists in the folder. With a short write injected: verification refuses the spawn and the run fails with the verification message. With an unresolvable executable: the folder contains no supplied file at all. With `afterInputScratchFn` recording the directory: at that moment the declared name does not exist and the scratch is inside `.tmp`.

- [x] **Step 3: Red test — queued cancellation writes nothing (spec §9.13)**

Cancel a queued run, drain, then assert the exchange folder has no supplied file and the run is `cancelled`.

- [x] **Step 4: Red test — contents are dropped after the write (spec §9.18)**

Hold the validated slice and the raw map the caller passed; after `writeSuppliedInputs` succeeds, assert the map is empty and every `Content` in the caller's slice is zeroed.

- [x] **Step 5: Implement the write step and wire it in**

In `Execute`, after `stdin` is opened and before `stdoutR, stdoutW, err := os.Pipe()`, call `writeSuppliedInputs`. It iterates `run.Inputs` in order, stages each file inside `filepath.Join(run.ExchangeDir, ".tmp")` at a random host name opened `O_CREAT|O_EXCL|O_WRONLY, 0o600`, writes the bytes, closes, `os.Rename`s onto the declared name, and removes the scratch on any error. After the loop it verifies each target with `Lstat`. On the first failure it returns `Outcome{Status: RunStatusFailed, Error: fmt.Sprintf("write input file %q: %v", name, err)}` (or the verification wording) with `stop=true`. On success it calls `dropInputContents(&run)`.

- [x] **Step 6: Verify**

Run: `go test ./plugin_commands -run 'TestSuppliedInput|TestCommandExecutor' -count=1`
Run: `go test ./plugin_commands -count=1`
Run: `go test -race ./plugin_commands -run 'TestSuppliedInput' -count=1`

- [x] **Step 7: Commit**

```bash
git add plugin_commands/runner_unix.go plugin_commands/inputs_runner_test.go plugin_commands/helper_process_test.go
git commit -m "feat: write supplied command inputs before the spawn"
```

---

### Task 5: Record the names and sizes, and render them

**Files:**
- Modify: `models/plugin_command_run_model.go`, `plugin_commands/types.go`.
- Modify: `application_context/plugin_command_store.go`, `application_context/plugin_command_store_test.go`.
- Modify: `plugin_system/fs_api.go`, `plugin_system/fs_api_test.go`.
- Modify: `server/template_handlers/template_context_providers/plugin_command_history_context.go` and its test, `templates/pluginCommandHistory.tpl`.

**Interfaces:**

```go
type SuppliedInput struct {
    Name  string `json:"name"`
    Bytes int64  `json:"bytes"`
}

type RunRecord struct {
    // ...existing fields...
    Inputs []SuppliedInput
}

// decoded into RunView.Inputs (RunView embeds RunRecord today)
```

- [x] **Step 1: Red store test**

`CreateRun` with two `Inputs` writes `InputsJSON` as `[{"name":"cookies.txt","bytes":1234},...]` in order; reading it back yields the same slice; a hand-corrupted column reads as empty with a log and does not fail the read; a run with no inputs stores `""`.

- [x] **Step 2: Implement persistence**

Add `InputsJSON string gorm:"type:text"` to `PluginCommandRun` (AutoMigrate covers SQLite and Postgres). Marshal on write, unmarshal tolerantly on read in the store's single row→record path. `dispatcher.acceptCommand` passes `run.Inputs` through in the `RunRecord` it already builds — no interface change.

- [x] **Step 3: Red Lua-view test**

`mah.fs.runs` returns `inputs = { {name=…, bytes=…}, … }` and never any contents; a run with none yields an empty table.

- [x] **Step 4: Implement `runViewToLua`**

Append the `inputs` key built from `RunView.Inputs`.

- [x] **Step 5: Red template test and render it**

Template-context test: names and sizes are present in the rendered page; a distinctive secret never appears. Add an accessible table above the existing Imports table — `<caption class="sr-only">Input files supplied to this run</caption>`, `<th scope="col">File</th>`, `<th scope="col">Bytes supplied</th>` — with the "contents are never recorded" note.

- [x] **Step 6: Verify**

Run: `go test ./application_context ./plugin_system ./server/template_handlers/... -run 'TestPluginCommand' -count=1`
Run: `go test --tags 'json1 fts5' ./application_context ./plugin_commands ./plugin_system -count=1`

- [x] **Step 7: Commit**

```bash
git add models/plugin_command_run_model.go plugin_commands/types.go plugin_commands/dispatcher.go application_context/plugin_command_store.go application_context/plugin_command_store_test.go plugin_system/fs_api.go plugin_system/fs_api_test.go server/template_handlers/template_context_providers/plugin_command_history_context.go server/template_handlers/template_context_providers/plugin_command_history_context_test.go templates/pluginCommandHistory.tpl
git commit -m "feat: record supplied command input names and sizes"
```

---

### Task 6: Prove it end to end and document it

**Files:**
- Modify: `e2e/test-plugins/test-commands/plugin.lua`, `e2e/test-plugins/test-commands/bin/test-command-helper`.
- Modify: `e2e/tests/plugins/plugin-command-history.spec.ts`.
- Modify: `docs-site/docs/features/plugin-system.md`, `docs-site/docs/features/plugin-permissions.md`, `docs-site/docs/features/plugin-lua-api.md`, `docs-site/docs/api/plugins.md`.

- [x] **Step 1: Extend the fixture**

Add a second command declaration: `name = "read_input"`, `argv = { "test-command-helper", "read-input" }`, `inputs = { "notes.txt" }`, timeout 60. Extend the helper's `read-input` mode to copy `notes.txt` to `seen.txt` and print `read-ok` only (never the contents). Add an API mode that supplies `inputs = { ["notes.txt"] = body.secret }` and another that supplies none.

- [x] **Step 2: Extend the browser proof**

In `plugin-command-history.spec.ts`: a run with a secret input shows `notes.txt` and its byte count on the run detail page and nowhere in the page HTML contains the secret; the `inputs`-less run shows no input row and succeeds; the declared-name refusal returns a non-2xx with the message. Run the a11y subset that covers the page (the new table must not regress it).

- [x] **Step 3: Update the four docs pages**

`plugin-system.md`: `commands[].inputs` in the manifest field table, in the declaration field table (with "order does not affect identity"), and one sentence in "Declared server commands". `plugin-permissions.md#trusted-server-commands`: the names, the contents-are-unreviewable sentence, the retention worst case and the dot rule. `plugin-lua-api.md`: the `mah.commands.run` signature line becomes `run(name, params [, callback] [, options])` plus an `inputs` example and the limits. `api/plugins.md`: the enable-response command objects gain `inputs`.

- [x] **Step 4: Verify every gate**

```bash
go build --tags 'json1 fts5' ./...
go test --tags 'json1 fts5' ./plugin_commands/... ./plugin_system/... ./application_context/... ./server/... ./internal/arch/... -count=1
go run ./cmd/mr docs lint
cd e2e && npm run test:with-server:all
```

Postgres (Docker): `go test --tags 'json1 fts5 postgres' ./server/api_tests/... -count=1`

- [x] **Step 5: Commit**

```bash
git add e2e/test-plugins/test-commands e2e/tests/plugins/plugin-command-history.spec.ts docs-site/docs/features/plugin-system.md docs-site/docs/features/plugin-permissions.md docs-site/docs/features/plugin-lua-api.md docs-site/docs/api/plugins.md
git commit -m "test: prove plugin command inputs end to end"
```

---

## Final self-review checklist

- [x] Spec §1 manifest rules map to Task 1 (grammar, dot rule, count, capability, identity-as-set, no argv-reference requirement).
- [x] Spec §2 Lua surface maps to Tasks 3 and 4 (signature, strict keys, zero-byte, wording, transaction/disable refusals).
- [x] Spec §3 ordering and atomicity maps to Task 4 (phase split, slot in `Execute`, stage/rename, verification, failure path, three consequences).
- [x] Spec §4 limits and quota maps to Tasks 1, 3 and 4 (constants, synchronous aggregate, per-run quota admission, global sample refresh note).
- [x] Spec §5 records and redaction maps to Tasks 4 and 5 (`InputsJSON` shape, no `ParamsJSON` key, tolerant decode, Lua view, admin table, `CommandDisplay.Inputs`, submit-time sizes).
- [x] Spec §6 consent maps to Task 2 (grant, display, label phrases, both panel sites, CLI, docs).
- [x] Spec §7 retention and secrets maps to Tasks 4 and 6 (queued cancellation writes nothing, worst-case docs sentence, discard/read-back).
- [x] Spec §8 what must not change has no task that touches `Prepare`, `BuildInvocation`, `mah.fs` writes or the Windows refusal.
- [x] Every test in spec §9 with a §9 number is named in Task 4 or Task 6; §9.7 and §9.14 are in Tasks 2, 3 and 5.
- [x] `CONTEXT.md` terms (Command Declaration, Declared Input File, Run Parameter, Command Import, Exchange Folder) appear in the new docs prose, and "input file" is never used for a Command Import.
