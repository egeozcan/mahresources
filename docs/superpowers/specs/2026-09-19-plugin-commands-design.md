# Plugin-declared server commands and exchange folders

Status: draft for user review
Date: 2026-09-19

## Goal

Let a plugin declare, in its manifest, a fixed set of server command templates
that the host may execute on its behalf, run them as asynchronous background
jobs triggered from Lua, and give the plugin a mediated way to turn the files a
command produces into library resources.

Motivating use case: a yt-dlp plugin. yt-dlp is an external binary; the plugin
VM deliberately has no filesystem access and no process execution, so neither
half of "download media via yt-dlp" is expressible today. This spec adds the
two missing powers as one cohesive capability: **declared commands** (host
executes a fixed argv template) and **exchange folders** (host mediates the
files a command writes back into the plugin's reach).

The design philosophy follows the one `mah.media` already established: the
plugin names *what* should happen, never a path, a binary flag it invented, or
a shell string. The host owns every path and every argument boundary.

## Non-goals

- Operator-initiated runs (UI forms supplying parameters). Commands are
  plugin-initiated from Lua only. A UI trigger can be layered on later without
  changing anything here.
- Per-run operator approval. Consent is given once, at enable time, to the full
  declared command set.
- Automatic retries of failed command runs. The plugin decides whether to rerun.
- Command stdout as a data channel for the plugin. Output is captured to the
  job history for debugging only; files are the data channel.
- Distribution/installation of plugins (separate roadmap item).

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
            argv = { "yt-dlp", "{{url}}", "--paths", "{{exchange_dir}}",
                     "-o", "%(title)s [%(id)s].%(ext)s" },
            timeout = 7200,
        },
    },
}
```

Parse-time rules (fail loudly, in the style of the existing `network` parser):

- `argv` is a **list of exec arguments, never a shell string**. At run time the
  host `exec`s argv directly with the substituted elements. No shell is
  involved anywhere in the pipeline, so quoting/escaping as a failure class
  does not exist.
- A `{{placeholder}}` must be an **entire element** of `argv`. Partial
  interpolation inside an element (e.g. `"--paths={{dir}}"`) is a load error.
  Substitution replaces the whole element with the parameter string.
- Every `{{placeholder}}` appearing in `argv` must be supplied at run time or
  the run is refused. There is no empty-string substitution. The one exception
  is `{{exchange_dir}}` (§4), which the host fills itself.
- `name` is a unique slug within the plugin; duplicates are a load error.
- `timeout` is optional, in seconds; default 3600, host-capped maximum 86400
  (24h), like other bounded powers.
- A manifest with `commands` but without the `commands` capability in
  `capabilities` is a load error, in the same way `network` requires `http`.

The manifest never stores a shell-joined string. The manage UI derives a
human-readable display by shell-quoting and joining argv at render time.

### New capability

`commands` joins `AllCapabilities` (slug `commands`, surfaces note `mah.commands, mah.fs`)
with a consent label of the shape:

> Run the commands this plugin declares on the server machine, and read the
> files those commands write into its private exchange folders.

## 2. Consent and the warning

- The grant is **all-or-nothing**: enabling a plugin with `commands` granted
  consents to every command the manifest declares.
- **Widening detection extends to commands.** The persisted consent record
  stores the consented command list — for each command, its `name`, `argv` and
  `timeout`. `CompareGrants` treats an added command, a changed argv template,
  or a changed timeout as a widening: the plugin refuses to load until the
  operator re-enables. Editing plugin code can never smuggle in a new command;
  only a fresh consent can.
- **Manage UI.** The plugin list shows a warning panel for any plugin
  declaring commands: *"⚠ This plugin can run the following commands on the
  server:"* followed by each command verbatim (shell-joined argv, timeout). The
  enable/consent flow surfaces the same panel before consent is recorded.

## 3. Execution model

### Lua surface

```
mah.commands.run(name, params [, callback]) -> run_id | nil, err
```

- `name` is the declared command slug. `params` is a Lua table of
  string→string values. `callback`, if given, is a Lua function executed in a
  background VM goroutine when the run reaches a terminal state (same
  machinery as `mah.start_job`), receiving one result table:

  ```lua
  { ok = bool, exit_code = n|nil, error = string|nil, run_id = "..." }
  ```

- The call is refused inside DB transactions, exactly like
  `mah.download.submit`: the command runs for minutes and transactions do not
  wait.
- Requires the `commands` capability. Ungranted, `mah.commands` is never
  installed (the standard withheld-module behaviour).

### Job machinery

A command run is a new job kind on the download-queue machinery (Approach A):
attempt structure, history capture, job events, progress sink, sweep/pausing
and actor attribution all come from the existing infrastructure. The command
runner is a `JobRunFn`.

- History captures: argv as run (post-substitution), exit code, duration, and
  a size-capped tail of combined stdout/stderr (64 KB). The tail is for
  debugging in the job history UI, not a plugin data channel.
- **No automatic retry.** A failed run terminates as failed; the callback and
  history report it. A plugin that wants a retry calls `run` again.
- Concurrency: at most 2 concurrently running command jobs per plugin, and a
  global cap across plugins. Beyond that, runs queue FIFO.
- Cancellation: an operator can cancel a queued or running command job through
  the existing job UI (process killed, run marked cancelled, callback fires
  with `ok = false`).

### Host hardening

- argv[0] is resolved with `PATH` lookup at run time. No plugin-configurable
  binary path.
- Parameters are passed as exec arguments directly. The host never re-parses
  or re-quotes them.
- Each parameter value is capped at 8 KB; longer values refuse the run.
- The working directory of the spawned process is the run's exchange folder.
- Environment: a minimal environment (PATH, HOME, TMPDIR, TZ plus whatever the
  host already exports to ffmpeg for `mah.media`). yt-dlp's own cookie/config
  needs are served by files the operator places in the service account's home,
  not by plugin-supplied values.

## 4. Exchange folders and `mah.fs`

### Layout

The host creates one scratch directory per run before launching the command,
on the same afero filesystem resources live on (so MemoryFS and alternate
filesystems keep working):

```
<file save path>/plugin_exchange/<plugin-name>/<run-id>/
```

### The `{{exchange_dir}}` placeholder

Any command template may use `{{exchange_dir}}`; the host substitutes the
run's actual directory. Plugins never construct or see paths — file identity is
always a `(run_id, name)` pair inside the plugin's own scratch space.

### `mah.fs` surface

Granted with the `commands` capability (a plugin can only ever reach its own
runs' folders; a separate capability name would be grant sprawl without a
separable power to withhold):

- `mah.fs.list(run_id)` → array of `{ name, size, modified }` tables.
  Names come from the host's `readdir` of the real directory, never from
  plugin-supplied strings, so path traversal is structurally impossible.
  `run_id` must belong to a finished run of the *calling* plugin; anything else
  is an error.
- `mah.fs.read(run_id, name, max_bytes)` → file content as a string,
  refusing beyond `max_bytes` (hard cap 4 MB). For small sidecars such as a
  yt-dlp metadata JSON destined for resource meta.
- `mah.fs.create_resource(run_id, name, fields)` → resource id. The host
  streams the file into resource storage through the same code path as an
  upload; `fields` carries name, description, tags, groups and meta exactly as
  `mah.db.create_resource` does. On success the file is deleted from the
  exchange folder (import consumes the file).
- `mah.fs.discard(run_id, name)` → deletes one file from the exchange folder
  (e.g. `.part` leftovers the plugin chose not to import).

`runs` accessor: `mah.fs.runs()` → array of the calling plugin's run ids that
still have exchange folders, oldest first, so a callback-less plugin can
enumerate after the fact.

### Cleanup sweep

A sweep pass (same pattern as the existing export sweep) removes exchange
directories of finished runs older than 7 days (host-configurable). The sweep
logs what it removed. Anything a plugin wants kept must be imported or read
before the sweep reaches it; the 7-day default is generous for the intended
"import in the completion callback" flow.

## 5. The yt-dlp plugin (separate repository)

Shipped after the core lands. Lives in its own repository
(`yt-dlp-plugin-for-mahresources`), installed into the configured plugin path.

```lua
plugin = {
    api_version = 1,
    name = "yt-dlp",
    capabilities = { "db:write", "commands" },
    commands = {
        {
            name = "download",
            argv = { "yt-dlp", "{{url}}", "--paths", "{{exchange_dir}}",
                     "-o", "%(title)s [%(id)s].%(ext)s",
                     "{{format_arg}}", "--no-playlist" },
            timeout = 7200,
        },
    },
    settings = {
        { name = "format", type = "string", label = "yt-dlp format selector",
          default = "bestvideo*+bestaudio/best" },
        { name = "import_extensions", type = "string",
          label = "File extensions to import (comma-separated)",
          default = "mp4,mkv,webm,mp3,m4a,opus" },
    },
}
```

Flow: a plugin page (or action) where the operator pastes a URL →
`mah.commands.run("download", { url = url, format_arg = "--format " .. format },
on_complete)` → in the callback: `mah.fs.list(run_id)`, filter to importable
extensions (skip `.part`/`.ytdl`/thumbnail/sidecar files), `create_resource`
each remaining file with the source URL recorded in meta and description,
`mah.fs.discard` the rest.

`yt-dlp` is resolved from the server's `PATH`; the operator installs it on the
host. No binary-path setting exists to spoof.

## 6. Testing

Core (mahresources):

- Manifest parsing: placeholder-must-be-whole-element, unknown/duplicate
  command names, timeout cap, `commands` without the capability is a load
  error.
- Consent: command list persisted; added/changed command argv or timeout is a
  widening that blocks load until re-consent.
- Substitution: assert the *exec argv* the host builds (parameter replaces the
  whole element; `{{exchange_dir}}` filled by host; missing parameter refuses
  the run). No shell is ever constructed — a regression test asserts the
  runner does not use a shell anywhere.
- Refused in transaction; parameter size cap; per-plugin and global
  concurrency caps; timeout kills the process and records the failure.
- History capture (argv, exit code, stderr tail cap).
- `mah.fs`: list/read/create_resource/discard; path-traversal regression
  (names can only come from readdir; `(run_id, name)` must resolve inside the
  plugin's own exchange tree); cross-plugin run access refused; import
  consumes the file; MemoryFS path works.
- Sweep: finished runs older than the retention are removed, fresh ones kept.

Manage UI: warning panel renders verbatim commands; enable flow shows it;
granted-but-changed manifest forces re-consent.

Plugin repository: integration-style tests through the mahresources plugin test
harness — the command template pointed at a fake `yt-dlp` stub script,
exercising run → callback → list → import end to end; standalone Lua tests for
extension filtering where they can run without the host.

## 7. Out of scope (future)

- Operator-initiated command runs from the UI.
- Automatic retry policies.
- Command stdout/stderr as structured plugin data.
- Streaming progress from the command into the job progress sink (yt-dlp
  `--progress-template` could map later).
- Plugin-package distribution format.
