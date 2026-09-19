# Plugin-declared server commands and exchange folders

Status: draft 7 — revised after six gpt-6-astra design reviews
Date: 2026-09-19

## Goal

Let a plugin declare, in its manifest, a fixed set of server command templates
that the host may execute on its behalf, run them as asynchronous background
jobs, and give the plugin a mediated way to turn the files a command produces
into library resources.

Motivating use case: a yt-dlp plugin. yt-dlp is an external binary; the plugin
VM deliberately has no filesystem access and no process execution, so neither
half of "download media via yt-dlp" is expressible today. This spec adds the
two missing powers as one cohesive capability: **declared commands** (host
executes a fixed argv template) and **exchange folders** (host mediates the
files a command writes back into the plugin's reach).

## Trust model (stated plainly)

**This is trusted execution, not sandboxing.** A command runs with the full
privileges of the mahresources service account: its filesystem, its network,
its credentials. The host guarantees:

- The host never spawns a shell. The process is created from an argv vector,
  and argument boundaries established by the manifest are preserved.
- argv[0] is a literal, nonempty executable **basename** — no path separators,
  no leading dash, no placeholders — resolved via a trusted PATH. A plugin
  cannot point the command at an arbitrary executable.
- A parameter replaces an entire argv element. The host never re-parses,
  re-quotes or splits a parameter.

The host does **not** guarantee that the invoked program interprets those
parameters safely. A program consumes its arguments according to its own
grammar: a value supplied by a plugin can be interpreted as a flag (yt-dlp's
`--exec=…` is the canonical example), and the declared argv[0] may itself be an
interpreter. Parameter validation below (§3) rejects the degenerate cases the
host can see; it cannot make a plugin's use of a program safe. Enabling the
`commands` capability means trusting the plugin author with execution under the
service account. The capability label and the enable-time warning say this
explicitly.

Two consequences follow, stated in the consent label and docs rather than
papered over:

- The per-plugin **egress policy does not apply** to spawned processes. A
  command may reach private hosts and the loopback. (Process/network
  confinement is a possible future addition; v1 does not attempt it.)
- Operator-level configuration visible to the command can alter program
  behaviour in ways no manifest consent covers. The shipped yt-dlp template
  passes `--ignore-config` (§6) precisely so the warning panel's verbatim argv
  remains the whole story for *that* plugin; the environment allowlist (§3)
  bounds the residual surface to deliberate operator configuration.

The one structural guarantee retained from the shell-free design is that the
*host* cannot be made to misparse: whatever the program does with its
arguments, it receives exactly the argv the template and parameters produce.

## Non-goals

- Operator-initiated runs (UI forms supplying parameters). Commands are
  plugin-initiated from Lua only.
- Per-run operator approval. Consent is given once, at enable time, to the
  full declared command set.
- Automatic retries of failed command runs.
- Process or network confinement of spawned commands.
- Command stdout as a data channel for the plugin; files are the data channel.
- Windows support for command runs (§3); command-bearing manifests load but
  refuse runs on Windows in v1.
- Plugin distribution/installation (separate roadmap item).

## 1. Manifest

A new optional `commands` field on the `plugin` global, alongside
`capabilities`/`network`:

```lua
plugin = {
    api_version = 1,
    name = "yt-dlp",
    capabilities = { "db:write", "commands" },
    commands = {
        {
            name = "download",
            argv = { "yt-dlp",
                     "--paths", "{{exchange_dir}}",
                     "-o", "{{output_template}}",
                     "--format", "{{format}}",
                     "--no-playlist",
                     "--no-directories",
                     "--", "{{url}}" },
            timeout = 7200,
            sensitive_params = { "url" },
        },
    },
}
```

Parse-time rules (fail loudly, in the style of the existing manifest parser):

- `argv` is a **list of exec arguments, never a shell string**. The host
  creates the process from the argv vector directly. No shell is spawned
  anywhere in the pipeline.
- `{{placeholder}}` must be an **entire element** of `argv`; partial
  interpolation inside an element is a load error. Substitution replaces the
  whole element with the parameter string.
- Every `{{placeholder}}` must be supplied at run time or the run is refused.
  No empty-string substitution. `{{exchange_dir}}` (§5) is **host-filled and
  reserved**: a `params` key named like a host-filled placeholder refuses the
  run — it is not an override and not silently ignored. The set of
  host-filled names is part of the API surface and grows only deliberately.
- `argv[0]` must be a literal, nonempty **basename**: no `/`, no `..`, no
  `-` prefix, no placeholders, no path separators. It is resolved via `PATH`
  at run time from the host's trusted directories. Anything else is a load
  error, so a template can never name an arbitrary executable or path.
- Placeholders must **not** occupy a flag position. By convention and by
  review guidance, variable data follows a fixed flag (`"--format",
  "{{format}}"`), never forms one. The parser cannot understand each program's
  grammar, so this is a convention the manifest reviewer sees in the warning —
  not a host-enforced invariant — and the warning says parameters may alter
  the invoked program's behaviour.
- `name` is a unique slug per plugin; duplicates are a load error.
- `timeout` is optional, in seconds; default 3600, host-capped at 86400.
  The clock starts when the process is spawned, after the run leaves the queue.
- `sensitive_params` is an optional list of parameter names whose values are
  replaced with `[redacted]` in the job history (§4). It participates in
  consent comparison and manifest identity as a set (order-insensitive).
- At most 32 parameters per run and 64 KB of aggregate parameter bytes;
  exceeding either refuses the run.
- A manifest with `commands` but without the `commands` capability is a load
  error. (Note: unlike this spec's first draft claimed, `network` carries no
  such dependency on `http` today, and none is added here.)
- `name`, `argv`, `timeout` and `sensitive_params` participate in manifest
  identity (`Manifest.Equal` and the discovery/load integrity check; argv is
  compared positionally, `sensitive_params` as a set), so an edited template
  is detected the same way a changed `network` rule is.

The manifest never stores a shell-joined string; the manage UI derives a
human display by shell-quoting and joining argv at render time.

### New capability

`commands` joins `AllCapabilities` (surfaces note `mah.commands, mah.fs`) with
a consent label of the shape:

> Run the commands this plugin declares on the server machine with the
> service account's full privileges (filesystem and network, without
> sandboxing), and read the files those commands write into its private
> exchange folders. Importing those files into the library additionally
> requires the db:write capability.

## 2. Consent and the warning

- The grant is **all-or-nothing**: enabling a plugin with `commands` granted
  consents to every command the manifest declares.
- **Consent covers command declarations explicitly on every code path.** The
  persisted consent record gains a command list — for each command its `name`,
  `argv` (compared **positionally**, element by element, not via the
  order-insensitive `sameStrings`), `timeout`, and `sensitive_params`
  (compared as a set).
  - The legacy-consent short-circuit in `CompareGrants` runs **after** command
    comparison: a legacy consent record paired with a manifest that declares
    any command is a widening requiring re-consent.
  - A legacy manifest (`Declared == false`) cannot declare commands, so no
    upgrade path can introduce commands without a manifest change, which is
    itself caught.
  - **Grandfathering never grants commands.** The existing
    no-consent-record path (`plugin_system/consent.go:352-370`) records what
    the manifest declares and grants it — acceptable for reductions of a
    legacy full surface, but not for commands.
- **Enable must be a deliberate two-step gesture for command-bearing
  manifests.** Today, enabling records `GrantsFromManifest` unconditionally —
  enabling *is* the consent gesture (`plugin_state_context.go:224-230`), and a
  one-click toggle would silently make "explicit enable" and "rendered a
  panel" the same act. For a manifest declaring commands, the enable request
  is therefore **refused unless it carries an explicit acknowledgement field**
  (`confirm_commands`); a request without it is rejected with the warning data
  so the UI can render the panel and require a second, deliberate submission.
  The acknowledgement is recorded with the consent. This makes "explicit
  enable" a real confirm step, not a checkbox the flow happens to render.
- **No persistent consent store means no commands.** When the consent store is
  the in-memory default (`memoryConsentStore`, `consent.go:286-299`), the
  no-record path is the normal path on every boot, so under the
  never-grandfather rule command-bearing manifests are **refused at load
  entirely**. This is intended: consent that evaporates on restart cannot
  authorize service-account execution. Documented in the plugin docs.
- **Widening detection**: an added command, a changed argv (positionally), a
  changed timeout, or a changed `sensitive_params` list is a widening; the
  plugin refuses to load until the operator re-enables.
- **Manage UI.** The plugin list shows a warning panel for any plugin
  declaring commands: *"⚠ This plugin can run the following commands on the
  server, with the service account's privileges:"* followed by each command
  verbatim (shell-joined argv, timeout). The enable flow surfaces the same
  panel before the acknowledgement is recorded.

## 3. Execution model

### Lua surface

```
mah.commands.run(name, params [, callback]) -> run_id | nil, err
```

- `name` is the declared command slug; `params` is a Lua table of
  string→string values (each value capped at 8 KB; 32 values maximum).
- `callback`, if given, is a Lua function executed in a background VM
  goroutine when the run reaches a terminal state (same machinery as
  `mah.start_job`), receiving one result table:

  ```lua
  { ok = bool, exit_code = n|nil, error = string|nil, run_id = "..." }
  ```

- **The completion callback must stay cheap, and the spec says so.** It runs
  under the async callback budget (`asyncActionTimeout = 5 * time.Minute`,
  `manager.go:62`) while holding the plugin's **exclusive VM lock**
  (`action_jobs.go:407-413`) — the same lock pages, hooks and schedules
  contend for (`pages.go:49`). A callback that synchronously imported a
  7200-second download's files (multiple full passes over each file's bytes
  in `AddResource`) would blow the budget and block the plugin's own pages.
  Therefore the callback's contract is: **list, queue imports, discard —
  nothing that streams.** File→resource transfer never happens inside the VM
  (see `mah.fs.create_resource` in §5, which enqueues an *import job* and
  returns immediately); imports run on the dispatcher outside the VM lock, so
  a slow import cannot block a page or blow the callback budget. The host
  enforces the callback budget; exceeding it fails the callback but leaves
  the durable import results intact.
- **Callback delivery is at-most-once.** If the VM is unavailable at
  completion (plugin disabled, process restart), the callback is dropped and
  the terminal result remains readable through the durable run record (§3).
  A restart marks all unrecoverable nonterminal runs — running *and* queued —
  `interrupted`; callbacks are not replayed.
- Refused inside DB transactions, like `mah.download.submit`. Also refused
  when the invoking VM is revoked mid-flight, and when the plugin has been
  disabled since submission.
- Requires the `commands` capability. Ungranted, `mah.commands` is never
  installed.

### Scheduling

Command runs use a **dedicated dispatcher** over the download-queue job
machinery — not the generic semaphore-goroutine submission path — with these
properties made explicit rather than inherited:

- **FIFO per plugin**; queue admission bounded the same way the download
  queue is (`MaxQueueSize`).
- **Named concurrency caps on their own pool, off the download semaphore.**
  The download queue's `MaxConcurrentDownloads = 3` is not shared: two
  concurrent yt-dlp runs must not starve ordinary downloads. Command jobs
  run on their own pool: **at most 2 concurrently running commands per
  plugin, 4 globally**; **at most 2 concurrent import jobs globally** (also
  on their own pool — imports are disk-bound, not process-bound).
- **No pause and no manual retry for command jobs in v1.** Pausing a process
  tree and retrying under the same job ID both conflict with fresh-exchange-dir
  semantics; a rerun is a new `mah.commands.run` call with a new run id.
- **The dispatcher carries its own terminal classification.** The generic
  path classifies any `ctx.Err() != nil` as cancelled
  (`generic_job.go:167-171`), which would conflate timeout with cancellation.
  The command dispatcher classifies explicitly: timeout → `failed` (error
  text names it), operator cancel → `cancelled`, dispatch-loss →
  `interrupted`. Where the generic job record and the durable command record
  could disagree, **the durable command record is the authority the job UI
  displays** for command jobs.

### Durable run records

A new persisted table `plugin_command_runs` (id, plugin name, command name,
redacted parameter view, actor user id, created/started/finished timestamps,
status, exit code, error text, **spawned process group id**) survives
restarts and is independent of the queue's 1-hour terminal retention.

**Crash-safe launch registration.** Launch registration is tracked on the
row (the persisted pgid field) rather than as a public status, so the
six-status vocabulary below stays exhaustive. The ordering is explicit: a
row is created `queued` with no pgid; **at dispatch, the row is marked
`running` before the fork** (pgid still null); **after the fork, the pgid is
persisted**. This makes three row shapes distinguishable with no extra
status: `queued` (never launched), `running` with a pgid (live dispatch),
and `running` without a pgid — an **uncertain launch**, the spawn-crash
shape. Two crash windows remain, and both are handled by the recovery
identity check below rather than by assuming the pgid exists or is
trustworthy:

- A crash between fork and pgid persist leaves a nonterminal row with no
  recoverable group identity.
- A numeric pgid can be **reused**: after the original group exits, recovery's
  `kill(-pgid, 0)` can succeed against an unrelated group, and a blind
  SIGKILL would hit processes that never belonged to this run.

On startup, every nonterminal record — running **and** queued, since the
in-memory queue registry does not survive a restart
(`download_queue/manager.go:228-230`) — with no live dispatch is resolved as
follows:

1. **Identity check.** Every spawned process carries
   `MAHR_COMMAND_RUN_ID=<run id>` in its environment (§3, host hardening).
   Recovery enumerates the recorded pgid's group and verifies at least one
   member's environment carries the matching run id (via `/proc/<pid>/environ`
   on Linux, the process args interface on macOS).
2. **Verified dead** (no group, or empty group): the processes are gone;
   stamp `interrupted` with a finished timestamp. Nothing is writing.
3. **Verified alive**: descendants survived the crash; SIGKILL the group,
   wait for exit, then stamp `interrupted`. This closes the orphaned-writer
   case from the previous draft without touching unrelated groups.
4. **Unverifiable** (group exists but no member's environment matches —
   the reuse case — or no pgid was ever persisted, the spawn-crash case):
   stamp `interrupted` **with `output_unverified = true`**. The exchange
   folder's contents are **not importable**: `mah.fs` file operations on an
   `output_unverified` run are refused with an `output unverified` error,
   because ownership or death of the writers cannot be established. Only
   `mah.fs.discard_run` may remove such a run (deleting unverified output is
   safe; importing it is not). The plugin's page surfaces these runs for
   operator disposal.

Exchange folders of resolved runs are retained until the sweep reaches them.

The status vocabulary is exactly: `queued`, `running`, `succeeded`, `failed`,
`cancelled`, `interrupted`. Nothing else. One transition mapping defines
which terminal state each path produces:

| Path | Status | Note |
|---|---|---|
| Command exits 0 | `succeeded` | exit code recorded |
| Command exits non-zero / cannot start / quota exceeded | `failed` | error text names timeout or quota |
| Operator cancels via the job UI | `cancelled` | process group killed |
| Plugin disabled | `cancelled` | queued runs are refused at dispatch; running process groups killed — both marked `cancelled` with reason `plugin disabled` |
| Server crash / restart / shutdown / dispatch lost | `interrupted` | both running and queued records; pgid identity-verified (survivors killed) before stamping; unverifiable writer identity → `interrupted` + `output_unverified` |

`failed` (timeout/quota) and `cancelled` (operator/disable) are therefore
distinct statuses, as required.

**Retention:** `plugin_command_runs` rows are retained indefinitely in v1 —
pruning a row would destroy the import map (§5) that idempotent re-import
depends on, and run counts are bounded by usage. Future pruning tooling must
preserve import maps or accept losing idempotency for swept runs.

### Host hardening

- argv[0] resolved via `PATH` over operator-controlled directories at run
  time; no plugin-configurable binary path.
- Parameters are passed as exec arguments directly. The host never re-parses
  them.
- Per-parameter cap 8 KB; aggregate argv cap 64 KB; at most 32 parameters.
- The spawned process runs with its working directory set to the run's
  exchange folder and **stdin connected to `os.DevNull`** — a tool that
  prompts blocks only until its timeout, never indefinitely.
- **Environment is an explicit allowlist**, not inherited:
  `PATH` (operator-configured trusted directories), `HOME`, `TMPDIR` (private
  per-run temp), `LANG`, `TZ`, and run-context variables `MAHR_PLUGIN_NAME`,
  `MAHR_COMMAND_RUN_ID`, `MAHR_EXCHANGE_DIR`. Nothing else. Operator
  configuration reachable through these variables is deliberate operator
  action and is documented as such.
- Spawned processes have **no egress enforcement**: they do not pass through
  the plugin HTTP egress layer and may reach private hosts. Documented as part
  of the trust model, not hidden.

### Byte quotas

Timeout bounds time; it does not bound bytes. Two operator-configurable
quotas close that gap:

- **Per-run exchange quota** (default 4 GiB): what one run may write into its
  exchange folder. The host samples the folder's size periodically; when the
  sample exceeds the quota, the process group is killed and the run is
  marked `failed` with error text naming the quota. Enforcement is
  approximate (sampling, not a syscall-level rlimit) and documented as such;
  a tool that writes fast between samples can overshoot the quota.
- **Global staging quota** (default 50 GiB): what all exchange folders may
  hold combined. While above it, new command runs are refused until the
  sweep brings the total down.

### Process-tree termination

- v1 command runs are **Unix-only**. The child is started with its own
  process group (`Setpgid`); timeout, cancellation, quota-kill and disable
  kill the whole group (`kill(-pgid)`). Windows is out of scope for v1
  (see Non-goals): Go's `os.OpenFile` does not expose
  `FILE_FLAG_OPEN_REPARSE_POINT`, so §5's symlink defence has no Windows
  equivalent to name, and a Job Object alone would not close that gap. A
  command-bearing manifest loads on Windows but refuses to run.
- Output pipes are drained with a size cap; termination waits (bounded) for
  reap and pipe EOF. **Output is final only when the process group is dead
  and pipes are closed** — descendant writers (e.g. an ffmpeg spawned by
  yt-dlp) cannot keep writing into an exchange folder declared finished.
  The same pgid-verified-dead check the startup recovery performs (§3,
  durable run records) backs this claim for the crash case.
- Command workers are registered with the queue's shutdown tracking; on
  server shutdown, process groups are terminated and runs are recorded
  `interrupted` (exchange dirs retained for later inspection until swept).

## 4. History

Job history for command runs records: the command name, the **redacted
parameter view** (`sensitive_params` values replaced by `[redacted]`, all
other values shown), and the argv as actually executed with sensitive
parameter elements also redacted in the persisted record (the same elements,
so no path circumvents `sensitive_params`), plus exit code, duration and a
64 KB tail of combined stdout/stderr.

Two disclosures redaction cannot cover are documented rather than hidden:
captured program output (stdout/stderr) can itself contain secrets (a URL
the tool echoes, for example); it is HTML-escaped with terminal control
characters stripped.

**Authorization is administrator-only**, enforced by the dedicated
command-record path in both API and UI. It does **not** inherit the existing
download-history visibility (which permits non-admin users to view their own
history — `download_history_handlers.go:35-45`,
`download_template_context.go:140-153`), because command output is not a
per-user artifact: the process ran under the service account, whoever
submitted it. Generic jobs are excluded from the existing history by
deconstruction (`download_queue/history.go:95-101`), so command history
needs its own records and its own check.

## 5. Exchange folders and `mah.fs`

### Staging

Exchange directories are **OS-backed directories** — real paths under a
host-configurable staging root (default under the server's data directory) —
never an in-process afero view, which an external process cannot see.

**MemoryFS deployments are supported.** An earlier draft refused command runs
on MemoryFS on the premise that there is no OS-backed storage to import into;
that premise is wrong — `AddResource` writes through an `afero.File`
(`resource_upload_context.go:1235-1250`), which `MemMapFs` satisfies, and the
staging root is an OS path by construction, so import works. The real cost is
**RAM**: every imported byte passes through memory, and the same is already
true of ordinary uploads on such a deployment. v1 does not add a special
refusal where uploads have none; the byte quotas (§3) bound the exposure, and
operators running MemoryFS are already accepting that trade-off for their
whole library.

Layout: `<staging root>/plugin_exchange/<plugin-name>/<run-id>/`, created
private (0700) by the host before launch.

### The `{{exchange_dir}}` placeholder

Any template may use `{{exchange_dir}}`; the host substitutes the run's
actual OS path. Plugins never construct or see paths — file identity is a
`(run_id, name)` pair inside the plugin's own scratch space. Exchange folders
are flat: only top-level regular files are addressable; the yt-dlp template
passes flat-output flags so yt-dlp does not create subdirectories.

### `mah.fs` surface

Granted with the `commands` capability:

- `mah.fs.list(run_id)` → `{ entries = { { name, size, modified } ... },
  truncated = bool }`. The entry list is **capped at 10,000 entries and
  truncated, never refused** — a run that produced 10,001 files must not
  leave the plugin unable to even see the excess (per-name operations could
  not recover it and the run would be stuck until the sweep). The
  `truncated` flag tells the plugin names exist beyond the window.
- `mah.fs.discard_run(run_id)` → deletes the entire exchange folder of one
  finished run — the escape hatch when `truncated` hides uninteresting
  excess, or when a run is abandoned wholesale. Same run-level ownership
  checks as `list`; refuses while the run has **any nonterminal import**
  (see the import lifecycle below) and on runs whose output is unverified
  it is the *only* permitted operation.
- `mah.fs.read(run_id, name, max_bytes)` → content as a string; refuses beyond
  `max_bytes` (hard cap 4 MB).
- `mah.fs.create_resource(run_id, name, fields [, on_import])` →
  `import_id | nil, err` (or the existing resource id, when the import map
  short-circuits). Requires **both** `commands` and `db:write` (creating
  library content is a write power; see §7). **This call does not transfer
  bytes.** It validates, **atomically claims the import** (see lifecycle
  below), **enqueues the import job** on the import pool (§3) and returns
  the import id immediately — the same shape as `mah.download.submit`, for
  the same reason: the transfer runs outside the plugin VM, so the completion
  callback stays cheap and a slow import cannot hold the VM lock against
  pages and hooks. The import job streams the file from the staging
  directory through the `AddResource` path, records the result in the run's
  import map (`name → { import_id, resource_id, status, error }`, idempotent
  on completion), and then deletes the source file (on delete failure the
  file is marked `imported-pending-delete`; the import map, not the bytes,
  is what makes re-import idempotent). `on_import`, if given, fires
  at-most-once when the import job reaches a terminal state (contract
  below). Import results remain readable through `mah.fs.runs()` after any
  callback loss. Refused inside DB transactions, like
  `create_resource_from_data`.
- `mah.fs.discard(run_id, name)` → deletes one file.
- `mah.fs.runs()` → the calling plugin's durable run records: `{ id, command,
  status, started_at, finished_at, exit_code, error, output_unverified,
  imports }` — including `interrupted` runs after restart, the
  `output_unverified` flag for runs whose writer identity could not be
  verified (§3), and each run's import map — so recovery does not depend on
  a live callback.

### Import job lifecycle

Imports are durable work in their own right, with their own claims, states
and recovery — an enqueue that vanishes on restart is not acceptable:

- **Claims are atomic per `(run_id, name)`.** `create_resource` first checks
  the import map; if the name already has an import in a **nonterminal**
  state (`pending`, `running`) it returns the **existing import id** —
  idempotency holds at submission, not only at completion, so a page that
  reconciles while an import is queued does not double-import. Terminal
  entries resolve by state:

  | Existing entry | `create_resource` returns |
  |---|---|
  | `succeeded` | the existing resource id (short-circuit) |
  | `interrupted` | the **same import id**, re-enqueued as `pending` — an interrupted import keeps its identity; its plugin generation and actor binding are **refreshed to the re-submitting caller** (revalidated then, as below) |
  | `failed` / `cancelled` | a **new** claim with a new import id, replacing the terminal entry |
- **Import status vocabulary**: `pending`, `running`, `succeeded`, `failed`,
  `cancelled`, `interrupted`. Each claim record carries import id, run id,
  name, submitting plugin generation, actor user id, timestamps, and error
  text.
- **Enqueue failure is not limbo.** If pool admission fails or the process
  crashes between claim and enqueue, the claim is marked `failed` (enqueue
  failure) or `interrupted` (crash) — never silently pending.
- **Restart recovery.** Nonterminal imports (`pending`/`running`) found at
  startup are marked `interrupted`. They are recoverable: a subsequent
  `create_resource` for the same name **re-enqueues the same claim** (same
  import id, back to `pending`, bindings refreshed). This is the re-drive
  the §6 page reconciliation uses; until then the claim sits in `runs()`,
  visible and re-submittable — documented behaviour, not a silent drop.
- **Crash-safe destination handling is a prerequisite for replay, and the
  fresh-input temp alone does not provide it.** `AddResource` already copies
  its input into a unique temp (`resource_upload_context.go:1084-1094`), but
  then copies into a deterministic hash destination (`1251-1269`) and
  **trusts that destination if it exists** — so a crash during the
  destination copy leaves a truncated file that a replayed import would
  reuse as-is. The import path therefore validates the destination before
  invoking `AddResource`: the worker knows the staging file's exact size; if
  a destination file for the same content already exists **and its size
  differs from the source**, the destination is deleted first, so
  `AddResource` performs a full fresh copy. A size check suffices to catch
  copy-truncation (a crash mid-copy leaves a strict prefix, which cannot
  have the source's size); when destination and source sizes match, reuse is
  the ordinary post-copy-crash case and is correct. Additionally, each
  attempt still stages through its own uniquely named temp input so a
  partial *input* temp is never trusted. A **mid-destination-copy
  crash/replay test** (interrupting `AddResource`'s destination copy, not
  just the staging-to-temp copy) asserts the replayed import publishes a
  complete resource.
- **Import temp files are bounded and reclaimed.** Each attempt's input
  temp lives in a dedicated area of the staging root
  (`<staging root>/import_tmp/<import-id>/`), not inside the exchange
  folder, so it never appears in `mah.fs.list`. Both quotas count it: the
  per-run exchange quota covers the run's exchange folder **plus** its
  import temps, and the global staging quota covers the whole staging root
  including `import_tmp`. Cleanup is immediate on the import's terminal
  state (success or failure); startup recovery deletes any temps whose
  claim is terminal or `interrupted` — a partial temp is unusable by design,
  since re-import always copies fresh. Tests cover quota accounting
  including temps and orphan-temp cleanup after restart.
- **Plugin lifecycle binding.** Each claim is bound to the submitting
  plugin's generation. The import worker **revalidates at start**: plugin
  still enabled, `commands`/`db:write` grants still consented, actor's
  current role and scope permit the write. A revalidation failure marks the
  claim `cancelled` with the reason (the mirror of the Lua download entry
  point's revoked-VM refusal, `download_api.go:101-108`, for work that has
  left the VM).
- **Plugin disable.** Queued imports are cancelled (`cancelled`, reason
  `plugin disabled`). An import already running when the plugin is disabled
  **finishes its resource commit** — mid-copy cancellation would orphan the
  bytes just as a crash would — and is recorded `succeeded`; the disable
  boundary is at the start of the next job, not mid-stream. After a
  disable/re-enable cycle, cancelled imports stay cancelled — they are
  **not** auto-re-submitted; recovery is the explicit operator-controlled
  retry below (a new claim id, per the lifecycle table).
- **Server shutdown** marks nonterminal imports `interrupted` (recoverable
  by re-submission, same as restart).
- **`on_import` contract.** Fired at-most-once when the import job reaches a
  terminal state, in a background VM goroutine under the same exclusive-lock
  and `asyncActionTimeout` budget rules as the command completion callback,
  and equally required to stay cheap. Result table:

  ```lua
  { ok = bool, import_id = "...", run_id = "...", name = "...",
    resource_id = n|nil, error = string|nil }
  ```

  The submission-time short-circuit (already-imported) does **not** fire
  `on_import` — it returns synchronously at submission. Callback loss falls
  back to the durable map, as with command callbacks.

### Enforcement, per operation kind

- `mah.fs.runs()`: filtered by plugin and actor (its own runs, per the
  ownership rule below); no run id or name is supplied.
- `mah.fs.list(run_id)`: run-level checks — ownership, terminal state, and
  the `output_unverified` refusal (refused with `output unverified`, like
  every file operation; `discard_run` is the sole exception). No name is
  supplied. Entries are reported as `readdir` reports them, but only
  regular files are addressable by the operations below.
- `mah.fs.read/create_resource/discard(run_id, name, ...)`: the full file
  checks, in order:
  1. **Run ownership**: `run_id` must exist, belong to the calling plugin,
     and (for runs with a submitter) belong to the acting user; actor-less
     (schedule-submitted) runs are accessible to any principal acting for
     the plugin.
  2. **Finished state**: the run must be terminal; file operations against
     a running or queued run are refused. (Startup recovery has already
     resolved every `interrupted` run's writer status — §3 — and flagged
     `output_unverified` runs, whose file operations are refused with an
     `output unverified` error except `discard_run`.)
  3. **Name validation** (lexical): the name must match the plugin-visible
     character set (no `/`, `\`, null bytes, no `.` or `..`, length-capped).
  4. **Import claim short-circuit** (`create_resource` only): if the durable
     import map already records the name — `succeeded` → return the existing
     resource id; `pending`/`running` → return the existing import id;
     `interrupted` → re-enqueue the **same** claim id (bindings refreshed);
     `failed`/`cancelled` → a new claim replaces it — **before** any
     file-existence check. This is what makes re-import idempotent after the
     source file was deleted (failed post-import delete, or swept); `read`
     and `discard` of an imported-but-deleted name are plain `file not
     found`.
  5. **File checks**: the entry must be a **regular file** (`lstat`;
     directories, symlinks, devices and other specials are refused), and is
     opened **relative to the run directory with `O_NOFOLLOW`** (and
     re-verified after open), so a symlink swapped in after `readdir` cannot
     escape.
  6. **Size caps**: `read` enforces `max_bytes`; imports stream with a total
     quota.

Files are addressed only as `(run_id, name)` pairs inside the calling
plugin's own exchange tree; any other resolution is refused.

**Honest isolation statement.** This is *Lua-level* isolation: the plugin's
VM code can only reach its own runs. It is **not** process-level isolation —
every command runs under the same service account and can, by ordinary
filesystem permissions, read sibling exchange folders of other plugins and
runs. Operators running mutually distrusting plugins on one host should not
enable `commands` for all of them.

### Sweep and synchronization

- A sweep pass removes finished runs' exchange directories once a
  configurable retention (default 7 days, measured from **completion**, not
  creation) has elapsed. A run is skipped by the sweep while it has an active
  file operation — import, read and discard each hold their lease — or a
  nonterminal run record **or any nonterminal import** (`pending`/`running`),
  because the import pin is taken at **claim/admission time**, not at worker
  start: an import accepted against a run whose retention has already
  elapsed must not be swept out from under the worker that will eventually
  run. Lease acquisition and the sweep's skip-check are coordinated under
  the same per-run lock, so an operation cannot slip between the check and
  the sweep. When retention elapses, **everything** in the exchange
  directory is deleted, including `imported-pending-delete` bytes; what
  survives is the import map in the durable run record, which is what makes
  re-import idempotent after the bytes are gone — via the import-claim
  short-circuit in the operation order above.
- Import, read and discard take a per-file lease for their duration;
  `create_resource` inside a DB transaction is refused (same rule as
  `create_resource_from_data`), so a rollback can never silently lose the
  source file.
- Errors are explicit: `run not found`, `file not found`, `run swept`,
  `already imported` (which returns the existing resource id).

## 6. The yt-dlp plugin (separate repository)

Shipped after the core lands. Lives in its own repository
(`yt-dlp-plugin-for-mahresources`), installed into the configured plugin path.

```lua
plugin = {
    api_version = 1,
    name = "yt-dlp",
    capabilities = { "db:write", "commands", "pages" },
    commands = {
        {
            name = "download",
            argv = { "yt-dlp",
                     "--ignore-config",
                     "--paths", "{{exchange_dir}}",
                     "-o", "{{output_template}}",
                     "--format", "{{format}}",
                     "--no-playlist",
                     "--no-directories",
                     "--", "{{url}}" },
            timeout = 7200,
            sensitive_params = { "url" },
        },
    },
    settings = {
        { name = "output_template", type = "string", label = "Output filename template",
          default = "%(title)s [%(id)s].%(ext)s" },
        { name = "format", type = "string", label = "yt-dlp format selector",
          default = "bestvideo*+bestaudio/best" },
        { name = "import_extensions", type = "string",
          label = "File extensions to import (comma-separated)",
          default = "mp4,mkv,webm,mp3,m4a,opus" },
    },
}
```

Notes on the template and its settings:

- `--ignore-config` makes the warning panel's verbatim argv the whole story:
  no operator config file in the service account's `HOME` can silently alter
  behaviour the panel showed as consented.
- The URL is last, after `--`, so it can never be parsed as an option;
  `--paths` pins the download directory to the exchange folder.
- **The plugin validates `output_template` before passing it.** yt-dlp's
  `-o` accepts absolute paths and `../`, which would override `--paths` and
  escape the exchange folder — and this value comes from a *settings text
  field* typed by an operator, not from the consented template. The plugin
  rejects absolute paths, path separators and `..` in `output_template`,
  refusing the run submission with an explanatory error. (The trust model
  covers the host-vs-plugin boundary; this is the plugin holding its own
  operator-supplied input inside the sandbox the host built.)
- The default format selector `bestvideo*+bestaudio/best` makes yt-dlp
  **shell out to ffmpeg** for the merge: the operator note must say ffmpeg
  must be on the trusted `PATH` too (or the setting changed to a
  pre-merged single format).

Flow: a plugin page (capability `pages`) where the operator pastes a URL
triggers `mah.commands.run("download", { url = url, output_template = tpl,
format = format }, on_complete)`. The completion callback stays cheap per
§3: `mah.fs.list(run_id)`, filter to importable extensions (skip
`.part`/`.ytdl`/thumbnails), queue `mah.fs.create_resource` per remaining
file with the source URL recorded in meta and description, `mah.fs.discard`
the rest. The byte transfer happens in the import jobs, not the callback.

**Restart re-drive.** Callbacks are at-most-once, and nothing re-drives the
plugin after a restart unless the plugin itself looks. The plugin **has no
`schedule` capability** in v1; instead, its page handler **reconciles on
load**, and it reconciles **import states independently of the command
outcome** — a cancelled or interrupted command run can still have an
`on_complete` callback that queued imports, and those claims need re-driving
too. The walk over `mah.fs.runs()`:

- imports in `interrupted` state (stranded by a restart, from any terminal
  command run) → re-queued via a fresh `create_resource` (same claim id);
- `succeeded`/`failed`/`cancelled` command runs with unimported files and no
  claim → imports queued per the normal flow;
- imports in `failed`/`cancelled` state (e.g. cancelled by a disable, or
  failed at admission) → surfaced to the operator with an explicit **retry
  control** that creates a **new claim** (new import id, per the lifecycle
  table); nothing automatic, since a disable was a deliberate boundary;
- `interrupted` command runs with verified output → surfaced to the operator
  (their files may be partial; importing them is an explicit choice, the run
  status says why);
- `output_unverified` runs → offered for `discard_run`.

Until a human opens the page after a restart, stranded claims sit in
`runs()` — that is the documented behaviour, not a silent drop.

Resource creation uses the same field schema as the existing resource
creators (`mah.db.create_resource_from_url` / `create_resource_from_data`
options: name, description, tags, groups, meta).

## 7. Capability interactions

- `commands` alone grants: running commands, and `mah.fs.list/read/discard/
  discard_run/runs`.
- **`mah.fs.create_resource` additionally requires `db:write`.** A plugin
  with `commands` but no `db:write` can download and read files but cannot
  create resources. The consent label for `commands` says so explicitly.
- Actor propagation: the run record keeps the acting user; the import job is
  attributed to that actor and **revalidated at import time** against their
  current role and scope (a user demoted between run and import cannot use a
  stale grant). Actor-less runs (schedules) require the acting principal to
  hold `db:write` and are attributed to the acting user.

## 8. Testing

Core (mahresources):

- Manifest parsing: whole-element placeholders, argv[0] basename rule, timeout
  cap, `commands` without the capability is a load error.
- Consent: positional argv comparison; **upgrade-path tests** — legacy
  manifest + commands → refused until re-consent; legacy consent record +
  newly declared commands → widening detected despite the legacy short-circuit;
  missing consent record + command-bearing manifest → **load refused until
  explicit enable** (never grandfathered); changed `sensitive_params` or
  timeout → widening; **enable of a command-bearing manifest without the
  `confirm_commands` acknowledgement is refused**, and with it the
  acknowledgement is recorded; **command manifests on a memory consent store
  are refused at load**.
- Substitution: assert the **exec argv vector** the host builds — the process
  is created from the vector directly (the test asserts `exec.Cmd.Args`
  equals the built vector and no intermediate shell string exists in the
  launch path); whole-element replacement; `{{exchange_dir}}` host-filled; a
  `params` key colliding with a host-filled placeholder name refuses the run;
  missing parameter refuses the run; aggregate caps. argv[0] validation
  rejects paths and dashes.
- Refused in transaction (both `run` and `create_resource`); parameter count
  and size caps; per-plugin and global concurrency **on the command pool
  (2 per plugin, 4 global), separate from the download pool (3)**; timeout
  kills the **whole process group** — a regression test uses a child that
  spawns a long-lived descendant and asserts the descendant dies too.
- Quotas: a run whose writes exceed the per-run exchange quota is killed and
  marked `failed` (error names the quota); while above the global staging
  quota, new runs are refused; sampled accounting documented as approximate.
- Callback budget: a completion callback that only lists/queues/discards
  finishes inside `asyncActionTimeout`; an import of a large file runs on
  the import pool **while the plugin's page remains responsive** (no VM lock
  held); `on_import` fires at-most-once with the import map durable after
  callback loss.
- Terminal-state delivery: callback fires on completion, cancellation and
  timeout; disabled/disabled-then-re-enabled plugin semantics (disable →
  `cancelled` with reason, queued runs refused at dispatch); **status
  mapping test** — timeout → `failed` (named), operator cancel → `cancelled`,
  disable → `cancelled`, restart → `interrupted` for both running and queued
  (generic `ctx.Err() → cancelled` classification not used for commands);
  durable run record readable after the 1-hour queue retention expires.
- Crash recovery: restart with a **surviving orphaned process group** (a
  grandchild writer still alive) — the group is identity-verified and killed
  before the record is stamped `interrupted` — the guarantee is that output
  **cannot be imported while writers remain alive**, not that the killed
  writers' files are complete: an `interrupted` run's files may be partial,
  and its status is what tells the plugin that; a **spawn/persist crash**
  (row `running`, no pgid persisted →
  `interrupted` + `output_unverified`, file operations refused, `discard_run`
  allowed) and **pgid reuse** (the recorded group id belongs to unrelated
  processes after the original group exited → identity check fails, no
  unrelated process is killed, output marked unverified).
- stdin: the spawned process's stdin is `os.DevNull` (a prompting tool fails
  on timeout, never blocks forever).
- `mah.fs`: enforcement tests for name validation, symlink refusal
  (including a swap-after-lstat race), directory/special refusal, cross-plugin
  and cross-run access refusal, actor checks, idempotent re-import after a
  failed delete **and after the run has been swept** (import-claim
  short-circuit, no source file present), refusal inside transactions,
  **listing truncates with a `truncated` flag instead of refusing**,
  `discard_run` on a truncated run, read cap, **`output_unverified` runs:
  `runs()` exposes the flag, `list`/`read`/`create_resource`/`discard` are
  refused, `discard_run` is permitted**. A **mid-destination-copy
  crash/replay test** — interrupting `AddResource`'s hash-destination copy
  itself, not merely the staging-to-temp input copy — asserts a replayed
  import yields a complete resource: the size-validated destination is
  replaced, never a truncated file reused.
- History: sensitive-parameter redaction in both the parameter view and the
  persisted argv; output HTML-escaped and control-stripped; **command history
  access is administrator-only** in API and UI (a non-admin viewing their own
  download history cannot see command records).
- **Staging/import**: a **real subprocess** (not a stub) writes a file that
  `mah.fs.create_resource` imports through the configured afero filesystem —
  including a **MemoryFS target** (small files), proving the import path
  works through `MemMapFs`.
- Sweep: retention measured from completion; runs with active import, read
  or discard leases skipped (lease/sweep lock coordination); nonterminal runs
  skipped; expired runs' directories fully deleted including
  pending-delete bytes, with the import map surviving; fresh runs kept.
- Queue integration: FIFO per-plugin dispatch, command/import pool caps, no
  pause and no retry exposed for command jobs, terminal state reaches the
  callback and the durable record even when the generic queue's event
  machinery does not fire, and the durable record's status — not the generic
  classification — is what the job UI shows.
- Import lifecycle: atomic per-`(run_id, name)` claims — a repeated
  submission while `pending`/`running` returns the existing import id, not
  a duplicate; **enqueue failure** marks the claim `failed` (never silently
  pending); **restart** marks nonterminal claims `interrupted` and a fresh
  `create_resource` re-enqueues the **same claim id** (bindings refreshed),
  while `failed`/`cancelled` claims are replaced by a new id on re-submit; **plugin disable**
  cancels queued claims and lets a running import's commit finish (recorded
  `succeeded`); **worker-start revalidation** (disabled plugin, changed
  grants, demoted actor → claim `cancelled` with reason); **sweep
  protection from admission time** — an import accepted against a
  retention-expired run is not swept before its worker starts;
  **`discard_run`** refuses while nonterminal imports exist and is the only
  permitted operation on `output_unverified` runs; `on_import` receives the
  documented result table and is not fired by the submission-time
  short-circuit; **disable → re-enable → page reconciliation** re-drives
  interrupted claims (same id) and offers failed/cancelled claims for an
  explicit operator retry that creates a **new** claim id — nothing
  automatic; **import temp accounting**: per-run and global quotas include
  `import_tmp`, and startup recovery deletes orphaned temps of terminal or
  interrupted claims.

Manage UI: warning panel renders verbatim (shell-quoted, escaped) commands;
enable flow shows it and requires the acknowledgement; a changed command set
forces re-consent; legacy-record upgrade paths covered by tests per §2.

Plugin repository: integration tests through the mahresources plugin test
harness — the command template pointed at a fake `yt-dlp` stub script,
exercising run → callback → list → queued import → `on_import` end to end;
standalone Lua tests for extension filtering and **`output_template`
validation (absolute paths, separators, `..` refused)**.

## 9. Explicitly out of scope

- Operator-initiated command runs from the UI.
- Automatic retry policies.
- Process/network confinement of spawned commands (the trust model section
  is the honest statement for v1).
- Windows support for command runs (symlink defence has no Go-expressible
  equivalent; a Job Object alone does not close the gap).
- Automatic pruning of `plugin_command_runs` (import maps must survive;
  future tooling decides otherwise deliberately).
- Streaming progress from the command into the job progress sink.
- Command stdout as structured plugin data.
- Plugin-package distribution format.
