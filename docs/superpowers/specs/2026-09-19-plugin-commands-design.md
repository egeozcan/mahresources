# Plugin-declared server commands and exchange folders

Status: draft 2 — revised after gpt-6-astra design review
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
- Operator-level configuration visible to the command (e.g. a `yt-dlp` config
  file in the service account's home) can alter program behaviour in ways no
  manifest consent covers. The environment allowlist (§3) bounds this to
  deliberate operator configuration.

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
  No empty-string substitution. `{{exchange_dir}}` (§5) is host-filled.
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
    legacy full surface, but not for commands. A command-bearing manifest
    without a recorded command consent is refused at load until the operator
    explicitly enables it through the consent flow (which shows the warning
    panel). Once recorded, later changes are caught widenings. The load-time
    integrity check between discovery and load also covers commands via
    `Manifest.Equal`.
- **Widening detection**: an added command, a changed argv (positionally), a
  changed timeout, or a changed `sensitive_params` list is a widening; the
  plugin refuses to load until the operator re-enables.
- **Manage UI.** The plugin list shows a warning panel for any plugin
  declaring commands: *"⚠ This plugin can run the following commands on the
  server, with the service account's privileges:"* followed by each command
  verbatim (shell-joined argv, timeout). The enable flow surfaces the same
  panel before consent is recorded.

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

Command runs use a dedicated dispatcher over the download-queue job
machinery — not the generic semaphore-goroutine submission path — with these
properties made explicit rather than inherited:

- **FIFO per plugin**, at most 2 concurrently running command jobs per plugin
  and a bounded global cap; queue admission bounded the same way the download
  queue is (`MaxQueueSize`).
- **No pause and no manual retry for command jobs in v1.** Pausing a process
  tree and retrying under the same job ID both conflict with fresh-exchange-dir
  semantics; a rerun is a new `mah.commands.run` call with a new run id.
- **Timeout vs cancellation are distinct terminal states.** Timeout begins at
  process launch (not submission) and kills the process group; cancellation is
  operator-initiated.

### Durable run records

A new persisted table `plugin_command_runs` (id, plugin name, command name,
redacted parameter view, actor user id, created/started/finished timestamps,
status, exit code, error text) survives restarts and is independent of the
queue's 1-hour terminal retention. On startup, every nonterminal record —
running **and** queued, since the in-memory queue registry does not survive a
restart (`download_queue/manager.go:228-230`) — with no live dispatch is
marked `interrupted` with a finished timestamp; its exchange folders are
retained until the sweep reaches them.

The status vocabulary is exactly: `queued`, `running`, `succeeded`, `failed`,
`cancelled` (operator), `interrupted` (crash/shutdown/disabled). Nothing
else.

Plugin disable semantics: a disabled plugin's queued runs are refused at
dispatch; running processes are killed (process group) and marked cancelled.

### Host hardening

- argv[0] resolved via `PATH` over operator-controlled directories at run
  time; no plugin-configurable binary path.
- Parameters are passed as exec arguments directly. The host never re-parses
  them.
- Per-parameter cap 8 KB; aggregate argv cap 64 KB; at most 32 parameters.
- The spawned process runs with its working directory set to the run's
  exchange folder.
- **Environment is an explicit allowlist**, not inherited:
  `PATH` (operator-configured trusted directories), `HOME`, `TMPDIR` (private
  per-run temp), `LANG`, `TZ`, and run-context variables `MAHR_PLUGIN_NAME`,
  `MAHR_COMMAND_RUN_ID`, `MAHR_EXCHANGE_DIR`. Nothing else. Operator
  configuration reachable through these variables (e.g. a yt-dlp config file
  in `HOME`) is deliberate operator action and is documented as such.
- Spawned processes have **no egress enforcement**: they do not pass through
  the plugin HTTP egress layer and may reach private hosts. Documented as part
  of the trust model, not hidden.

### Process-tree termination

- On Unix the child is started with its own process group (`Setpgid`);
  timeout or cancellation kills the whole group (`kill(-pgid)`). On Windows,
  a Job Object with kill-on-close.
- Output pipes are drained with a size cap; termination waits (bounded) for
  reap and pipe EOF. **Output is final only when the process group is dead
  and pipes are closed** — descendant writers (e.g. an ffmpeg spawned by
  yt-dlp) cannot keep writing into an exchange folder declared finished.
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
the tool echoes, for example); it is shown only to administrators in the
job-history UI, HTML-escaped with terminal control characters stripped.
History access follows the existing job-history authorization (administrative
UI).

## 5. Exchange folders and `mah.fs`

### Staging

Exchange directories are **OS-backed directories** — real paths under a
host-configurable staging root (default under the server's data directory) —
never an in-process afero view, which an external process cannot see. Import
copies the file through the configured resource filesystem via the same
`AddResource` path as uploads. On a MemoryFS deployment, command runs are
refused at submission with an explanatory error (there is no OS-backed
resource storage to import into).

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

- `mah.fs.list(run_id)` → array of `{ name, size, modified }`, capped at
  10,000 entries (beyond which the call refuses). Names come from the host's
  `readdir`.
- `mah.fs.read(run_id, name, max_bytes)` → content as a string; refuses beyond
  `max_bytes` (hard cap 4 MB).
- `mah.fs.create_resource(run_id, name, fields)` → resource id. Requires
  **both** `commands` and `db:write` (creating library content is a write
  power; see §7). Streams the file from the staging directory into the
  configured resource filesystem through the `AddResource` path. Refused
  inside DB transactions, like `create_resource_from_data`. Import consumes
  the file: on success the file is marked `imported` and deleted; if deletion
  fails the file is marked `imported-pending-delete` and re-import is
  idempotent (returns the already-created resource id, recorded in the run's
  import map). Pending-delete bytes do not survive the sweep; the import
  map, not the bytes, is what makes re-import idempotent.
- `mah.fs.discard(run_id, name)` → deletes one file.
- `mah.fs.runs()` → the calling plugin's durable run records: `{ id, command,
  status, started_at, finished_at, exit_code, error }`, oldest first —
  including `interrupted` runs after restart — so recovery does not depend on
  a live callback.

### Enforcement, per operation kind

- `mah.fs.runs()`: filtered by plugin and actor (its own runs, per the
  ownership rule below); no run id or name is supplied.
- `mah.fs.list(run_id)`: run-level checks only — ownership and terminal
  state; no name is supplied. Entries are reported as `readdir` reports
  them, but only regular files are addressable by the operations below.
- `mah.fs.read/create_resource/discard(run_id, name, ...)`: the full file
  checks, in order:
  1. **Run ownership**: `run_id` must exist, belong to the calling plugin,
     and (for runs with a submitter) belong to the acting user; actor-less
     (schedule-submitted) runs are accessible to any principal acting for
     the plugin.
  2. **Finished state**: the run must be terminal; file operations against
     a running or queued run are refused.
  3. **Name validation**: the name must match the plugin-visible character
     set (no `/`, `\`, null bytes, no `.` or `..`, length-capped), must be
     a **regular file** (`lstat`; directories, symlinks, devices and other
     specials are refused), and is opened **relative to the run directory
     with `O_NOFOLLOW`** (and re-verified after open), so a symlink swapped
     in after `readdir` cannot escape. On platforms without `O_NOFOLLOW`
     the open is refused unless the platform provides an equivalent.
  4. **Size caps**: `read` enforces `max_bytes`; import streams with a total
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
  nonterminal run record; lease acquisition and the sweep's skip-check are
  coordinated under the same per-run lock, so an operation cannot slip
  between the check and the sweep. When retention elapses, **everything** in
  the exchange directory is deleted, including `imported-pending-delete`
  bytes; what survives is the import map (`run_id, name → resource id`) in
  the durable run record, which is what makes re-import idempotent after the
  bytes are gone.
- Import, read and discard take a per-file lease for their duration;
  `create_resource` inside a DB transaction is refused (same rule as
  `create_resource_from_data`), so a rollback can never silently lose the
  source file.
- Errors are explicit: `run not found`, `file not found`, `run swept`,
  `already imported` (which returns the existing resource id).
- Successful imports are recorded per run (`run_id, name → resource id`);
  the sweep leaves no live bytes behind; only the import map survives.

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

Notes on the template: variable data always travels as a **value after a
fixed flag** (`--format {{format}}`), never as a flag-valued element; the URL
is last, after `--`, so it can never be parsed as an option; `--paths` pins
the download directory to the exchange folder. A plugin page (capability
`pages`) where the operator pastes a URL triggers
`mah.commands.run("download", { url = url, output_template = tpl,
format = format }, on_complete)`; in the callback: `mah.fs.list(run_id)`,
filter to importable extensions (skip `.part`/`.ytdl`/thumbnails), create a
resource per remaining file with the source URL recorded in meta and
description, `mah.fs.discard` the rest. `yt-dlp` is resolved from the host's
trusted `PATH`; the operator installs it.

Resource creation uses the same field schema as the existing resource
creators (`mah.db.create_resource_from_url` / `create_resource_from_data`
options: name, description, tags, groups, meta).

## 7. Capability interactions

- `commands` alone grants: running commands, and `mah.fs.list/read/discard/
  runs`.
- **`mah.fs.create_resource` additionally requires `db:write`.** A plugin
  with `commands` but no `db:write` can download and read files but cannot
  create resources. The consent label for `commands` says so explicitly.
- Actor propagation: the run record keeps the acting user; the import is
  attributed to that actor and revalidated against their **current** role and
  scope at import time (a user demoted between run and import cannot use a
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
  timeout → widening.
- Substitution: assert the **exec argv** the host builds (whole-element
  replacement; `{{exchange_dir}}` host-filled; missing parameter refuses the
  run; aggregate caps). A regression test asserts no shell is constructed
  anywhere and that argv[0] validation rejects paths and dashes.
- Refused in transaction (both `run` and `create_resource`); parameter count
  and size caps; per-plugin and global concurrency; timeout kills the **whole
  process group** — a regression test uses a child that spawns a long-lived
  descendant and asserts the descendant dies too.
- Terminal-state delivery: callback fires on completion, cancellation and
  timeout; disabled/disabled-then-re-enabled plugin semantics; restart marks
  both running **and** queued runs `interrupted`; durable run record readable
  after the 1-hour queue retention expires.
- `mah.fs`: enforcement tests for name validation, symlink refusal
  (including a swap-after-lstat race), directory/special refusal, cross-plugin
  and cross-run access refusal, actor checks, idempotent re-import after a
  failed delete, refusal inside transactions, listing cap, read cap.
- **MemoryFS/staging**: with MemoryFS resource storage, command runs are
  refused; with OS-backed storage, a **real subprocess** (not a stub)
  writes a file that `mah.fs.create_resource` imports through the configured
  afero filesystem.
- Sweep: retention measured from completion; runs with active import, read
  or discard leases skipped (lease/sweep lock coordination); nonterminal runs
  skipped; expired runs' directories fully deleted including
  pending-delete bytes, with the import map surviving; fresh runs kept.
- Queue integration: FIFO per-plugin dispatch, per-plugin/global caps, no
  pause and no retry exposed for command jobs, terminal state reaches the
  callback and the durable record even when the generic queue's event
  machinery does not fire.

Manage UI: warning panel renders verbatim (shell-quoted, escaped) commands;
enable flow shows it; a changed command set forces re-consent; legacy-record
upgrade paths covered by tests per §2.

Plugin repository: integration tests through the mahresources plugin test
harness — the command template pointed at a fake `yt-dlp` stub script,
exercising run → callback → list → import end to end; standalone Lua tests
for extension filtering.

## 9. Explicitly out of scope

- Operator-initiated command runs from the UI.
- Automatic retry policies.
- Process/network confinement of spawned commands (the trust model section
  is the honest statement for v1).
- Streaming progress from the command into the job progress sink.
- Command stdout as structured plugin data.
- Plugin-package distribution format.
