# Plugin-declared server commands and exchange folders

Status: final — approved by gpt-6-astra review; operator findings 1-16 applied
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
  no leading dash, no placeholders — resolved through the host's explicit
  `-plugin-command-path`. A plugin cannot point the command at an arbitrary
  executable. The setting defaults to the server process's startup `PATH`, so
  that inherited value is a documented trust boundary until the operator pins
  it; empty and relative entries are refused.
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
  error, so a template can never name an arbitrary executable or path. The
  search uses `-plugin-command-path` (default: the server process's startup
  `PATH`), whose entries must be nonempty absolute directories.
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

> Run the commands this plugin declares from the operator-configured command
> path on the server machine with the service account's full privileges
> (filesystem and network, without sandboxing), and read the files those
> commands write into its private exchange folders. Importing those files into
> the library additionally
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
  server, with the service account's privileges; executable basenames are
  resolved only from the operator-configured plugin-command path:"* followed
  by each command verbatim (shell-joined argv, timeout). The enable flow
  surfaces the same panel before the acknowledgement is recorded.
- **The CLI enable path carries the acknowledgement explicitly.**
  `mr plugin enable` POSTs `/v1/plugin/enable` with only the name today
  (`cmd/mr/commands/plugins.go:52-58`; the handler reads only the name,
  `plugin_api_handlers.go:125-133`), so under the two-step rule it would be
  refused for command-bearing plugins with no way to satisfy the check —
  or worse, a bare flag would silently turn the two-step gesture back into
  one. v1 therefore adds **`--confirm-commands`**: without the flag, the
  enable request is refused and the CLI prints the command list — argv
  rendered verbatim, exactly as the panel does — plus the instruction to
  re-run with `--confirm-commands`; with the flag, the CLI sends the
  acknowledgement and the printed argv is the record of what was confirmed.
  The CLI is a deliberate second step, not a bypass.

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
- **Every VM load receives a monotonically increasing plugin generation before
  command submission is registered.** Runs/import claims capture it; disable
  revokes it, and re-enable produces a different generation even for an
  unchanged manifest. Worker-start revalidation therefore has an identity to
  compare from the first durable claim onward, not a primitive introduced by
  the later Lua adapter.

### Scheduling

Command runs use a **dedicated dispatcher** over the download-queue job
machinery — not the generic semaphore-goroutine submission path — with these
properties made explicit rather than inherited:

- **FIFO per plugin**; the dispatcher's own pending command queue is capped at
  `MaxQueueSize` **per plugin**, so one plugin can fill only its own admission
  budget. Pending imports use the same per-plugin cap on their separate queue.
  A queued durable row is visible through `mah.fs.runs()` and command history,
  but does not enter `download_queue`'s live-job registry until dispatch.
- **The live registry has a bounded managed lane.** `download_queue` retains
  its 100-job ordinary admission budget; dispatched commands/imports use a
  separate managed-live allowance capped at the six workers below and are not
  counted by `makeRoomForNewJob`. Managed occupancy is **derived under
  `dm.mu` by scanning `m.jobs` for the managed marker at admission**. `m.jobs`
  is the sole owner/source of truth, not mirrored in a counter, so both
  independent removal implementations free capacity automatically:
  managed-lane admission evicts a terminal entry through `evictJob`, while the
  existing `cleanupOldJobs` retention sweep directly `delete`s from `dm.jobs`
  and rebuilds `jobOrder` without calling `evictJob`. Terminal managed entries
  are evicted from that lane before another dispatch. Thus 100 queued commands consume no
  download registry slots, and the maximum live command/import footprint is
  four plus two rather than an unbounded share of ordinary downloads. A live
  command job's managed Cancel callback delegates to the same dispatcher
  cancellation primitive the history endpoint uses; `DownloadManager.Cancel`
  invokes it outside manager/job locks, so both controls share the durable
  request latch rather than implementing two cancellation paths.
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

The persistence model is **three tables plus one managed map**, named here
because later sections reference their contents:

- **`plugin_command_runs`** — the run row: id, plugin name, command name,
  redacted parameter view, actor user id, `actorless_at_submission`,
  created/started/finished timestamps, status, exit code, error text, spawned
  process group id, and nullable `exchange_removed_at`. The removal timestamp
  is set only after the retained exchange directory has been deleted or found
  absent; it keeps later retention passes from revisiting historical runs
  forever. The boolean distinguishes a run intentionally submitted
  without an actor from one whose actor was later deleted; `NULL` alone never
  grants actorless access.
  Small; survives restarts; independent of the queue's 1-hour terminal
  retention.
- **`plugin_command_run_output`** — one row per run holding the **argv as
  actually executed** (sensitive elements redacted, per §4) and the **64 KB
  combined stdout/stderr tail**. This is the large column; splitting it out
  is what makes retention split possible: run rows and import maps must
  survive indefinitely (§5's idempotency depends on them), the output tail
  needn't — it is prunable after a configurable age (default 30 days) with
  no effect on recovery. 640 MB per 10,000 runs is exactly the growth term
  a single exhaustive table would make unprunable on SQLite.
- **`plugin_command_imports`** — the import claims: import id, run id, file
  name, submitting plugin generation, actor user id, created/started/
  finished timestamps, status, error text, and `source_delete_pending` (§5).
  Cleanup trouble never overloads `error`: a succeeded import remains a
  success, while the separate boolean says retained source bytes still await
  run sweep. Small; retained with the run.
- **The import map** (`name → { import_id, resource_id, status, error,
  source_delete_pending }`, §5) — persisted as a table keyed by run id, the piece idempotent
  re-import depends on. Small; retained indefinitely.

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
in-memory dispatcher queue does not survive a restart — with no live dispatch
is resolved as follows. A durable `cancel_requested` takes precedence over
crash classification: a queued row is stamped `cancelled` with its stored
reason; a running row still performs the identity checks below, kills a verified
surviving group, and then stamps `cancelled`. If a recorded group may still be
alive but recovery cannot settle it safely, recovery leaves the row nonterminal,
retains the staging-root lease, withholds `mah.commands`/`mah.fs`, and allows the
rest of the server to boot. Rows without that latch use the other outcomes
below. The detailed containment rules are in
`2026-09-20-plugin-command-recovery-containment-design.md`.

1. **Boot identity check.** The same conditional statement that persists a new
   pgid also pairs it with the host-unique boot-session UUID (Linux `boot_id`,
   Darwin `kern.bootsessionuuid`). When both the recorded and current values are
   known and differ, the old local process cannot still be a writer: do not
   inspect or signal its numeric pgid; stamp `interrupted + output_unverified`
   (or latched `cancelled + output_unverified`). The identity is host-internal
   and absent from Lua/admin views. Empty legacy values and platforms without a
   reliable host-unique session UUID retain the fail-closed checks below;
   clock-derived `kern.boottime` is never used.
2. **Process identity check.** Every spawned process carries
   `MAHR_COMMAND_RUN_ID=<run id>` in its environment (§3, host hardening).
   Recovery enumerates the recorded pgid's group and verifies at least one
   member's environment carries the matching run id (via `/proc/<pid>/environ`
   on Linux, the process args interface on macOS).
3. **Verified dead** (no group, or empty group): the processes are gone;
   stamp `interrupted` with a finished timestamp. Nothing is writing.
4. **Verified alive**: descendants survived the crash; the identity check
   is repeated immediately before signalling, then the group is SIGKILLed
   and observed until dead before the run is stamped `interrupted`. If the
   group cannot be signalled, inspected, or proven dead inside the recovery
   deadline, quarantine the command runtime as described above. A narrow
   residual race remains — between the pre-signal check and signal delivery,
   the group can exit and the pgid be reused by an unrelated process, so the
   SIGKILL could land on it. It is **accepted and documented** rather than
   hidden: the window is sub-millisecond and exploitation requires the pid
   counter to wrap past every intervening allocation within that window — the
   same risk class the dispatcher's own runtime `kill(-pgid)` accepts against
   groups it created moments earlier.
5. **Unverifiable live group** (a recorded group exists but no member's
   environment matches — either same-boot pgid reuse or an Apple platform
   executable whose environment macOS does not disclose): **do not publish a
   terminal row**. Quarantine only the command runtime while retaining its
   staging lease; the HTTP server and unrelated work continue. Recovery retries
   on the command sweep cadence and publishes the atomic command/filesystem
   hosts without a plugin reload once every blocker is safe. It settles other
   independently safe rows and reports the complete blocker set to stdout and
   `/logs`. An operator may terminate a group out of band or restart with
   `-plugins-disabled` to bypass all plugin startup. Marking
   `interrupted + output_unverified` while the group
   remains alive is prohibited: refusing import protects data integrity but
   does not stop an unbounded writer or make terminal output final.
6. **No pgid persisted**: the fork/persist crash window provides no numeric
   group to inspect or terminate. This remains the one unaddressable launch
   shape: stamp `interrupted + output_unverified`, refuse every file operation
   except `discard_run`, and name the missing pgid in the durable reason.

The staging-root runtime lease is governed by the same containment boundary. A
busy lease during a rolling replacement quarantines command surfaces and retries
acquisition while the rest of the server boots; invalid roots, permissions
failures and malformed lease files remain fatal. An acquired lease spans
recovery, quarantine, activation and complete dispatcher quiescence.

Exchange folders of resolved runs are retained until the sweep reaches them.

The status vocabulary is exactly: `queued`, `running`, `succeeded`, `failed`,
`cancelled`, `interrupted`. Nothing else. One transition mapping defines
which terminal state each path produces:

| Path | Status | Note |
|---|---|---|
| Command exits 0 | `succeeded` | exit code recorded |
| Command exits non-zero / cannot start / quota exceeded | `failed` | error text names timeout or quota |
| Operator cancels via the live job UI or admin command history | `cancelled` | queued run: removed from the dispatcher and no process exists; dispatched/running run: cancellation latch wins before fork or the process group is killed |
| Plugin disabled | `cancelled` | queued runs are refused at dispatch; running process groups killed — both marked `cancelled` with reason `plugin disabled` |
| Server crash / restart / shutdown / dispatch lost | `interrupted` | queued rows and running rows whose group is dead or identity-verified (survivors killed) are stamped; different known boot or no persisted pgid → `interrupted + output_unverified`; a recorded live but unverifiable group remains nonterminal and quarantines command surfaces while the server boots |

`failed` (timeout/quota) and `cancelled` (operator/disable) are therefore
distinct statuses, as required.

**Disable coordinates with the pre-fork `running` window.** The row can be
`running` with no process group yet (§3, launch ordering), and the disable
rule above only names queued rejection and group kills. Disable therefore
takes the per-run dispatch lock: a disable arriving before the fork
prevents the fork (the row is cancelled without a spawn); a disable
arriving after the fork but before pgid persistence leaves cancellation
**latched** on the row, so the just-spawned group — whose pgid the worker
writes next — is killed and reaped before any terminal status is
published. No window exists in which a run is both disabled and spawning
unobserved. Tests cover disable before the fork and disable between fork
and pgid persistence.

**Retention:** run rows, import claims and import maps are retained
indefinitely in v1 — pruning any of them destroys the identity or the map
that idempotent re-import (§5) depends on, and run counts are bounded by
usage. The **output tail is the one prunable piece**: `plugin_command_run_output`
rows older than the configurable output retention (default 30 days) are
deleted by the sweep, with the run row itself untouched. Future pruning
tooling must preserve run rows, claims and maps; only output tails are
fair game.

### Host hardening

- argv[0] resolved by walking `-plugin-command-path` at run time; no
  plugin-configurable binary path. The setting defaults once at startup to the
  inherited process `PATH`, which is explicitly documented as a trust boundary,
  and every entry must be a nonempty absolute directory. Operators pin the flag
  or `PLUGIN_COMMAND_PATH` to trusted directories in production.
- Parameters are passed as exec arguments directly. The host never re-parses
  them.
- Per-parameter cap 8 KB; aggregate argv cap 64 KB; at most 32 parameters.
- The spawned process runs with its working directory set to the run's
  exchange folder and **stdin connected to `os.DevNull`** — a tool that
  prompts blocks only until its timeout, never indefinitely.
- **Environment is an explicit allowlist**, not inherited:
  `PATH` (the validated `-plugin-command-path` value), `HOME`, `TMPDIR` (private
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

- **Per-run exchange quota** (default **8 GiB**): what one run may write
  into its exchange folder. The default is sized for **merge peak**, not
  final size: yt-dlp's default `bestvideo*+bestaudio/best` selector
  downloads video and audio as separate files and then muxes a third, so a
  2.2 GiB result transiently occupies ~4.4 GiB — a 4 GiB quota kills such a
  run during its merge, at the end of a 2-hour download. §6's operator note
  repeats this: size the quota for the merge peak of your largest expected
  download. The host samples the folder's size periodically; when the
  sample exceeds the quota, the process group is killed and the run is
  marked `failed` with error text naming the quota. Enforcement is
  approximate (sampling, not a syscall-level rlimit) and documented as such;
  a tool that writes fast between samples can overshoot the quota.
- **Global staging quota** (default 50 GiB): what all exchange folders may
  hold combined. Usage is sampled at command-runtime startup and after each
  background sweep, then cached; command submission reads that cache and
  never walks the staging tree while holding the plugin VM or command
  admission lock. This is an approximate admission bound rather than a
  syscall-level hard cap: a run may grow beyond the last sample before the
  next sweep refresh. When the sampled value is above the limit, **new command
  runs are refused** until a refresh observes enough space. **The drain path
  is exempt**: imports of already-admitted runs proceed even above the global
  quota — draining the exchange folders by importing and deleting their files
  is exactly what brings the total down, so refusing an import for want of
  space that only that import can free would deadlock. Import temps count
  toward the next sample but never trigger import admission refusal.

### Process-tree termination

- v1 command runs are **Unix-only**. The child is started with its own
  process group (`Setpgid`). The live runner has creation-time authority over
  the exact group it just created and uses that authority directly for
  timeout, cancellation, quota-kill and disable (`kill(-pgid)`); it does not
  make local cleanup depend on whether the OS exposes a member's environment.
  The narrow exit/reuse race is the already accepted runtime risk described
  above. Restart recovery has no such creation-time authority and therefore
  retains the stronger environment identity check. Windows is out of scope for v1
  (see Non-goals): Go's `os.OpenFile` does not expose
  `FILE_FLAG_OPEN_REPARSE_POINT`, so §5's symlink defence has no Windows
  equivalent to name, and a Job Object alone would not close that gap. A
  command-bearing manifest loads on Windows but refuses to run.
- Output pipes are drained with a size cap. **Output is final only when the
  process group is dead and pipes are closed** — descendant writers (e.g. an
  ffmpeg spawned by yt-dlp) cannot keep writing into an exchange folder
  declared finished. A cleanup deadline may classify the attempted outcome,
  but it never licenses terminal publication while a group remains alive.
  Process-group inspection is dormant during ordinary execution and begins
  only after the direct parent exits or termination starts; polling the whole
  process table every 20 ms for a multi-hour command is prohibited. Recovery
  likewise publishes only after verified death; an alive unverifiable group
  quarantines command-runtime startup as described above. A live worker that
  passes its first cleanup deadline backs polling off to one second, logs once
  after one minute that the named run/pgid permanently holds a global command
  slot, and may make at most one final signal after a fresh observation that the
  group is alive (owned or environment-unverified).
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

Because dispatcher-queued rows deliberately do not occupy the live job
registry, the administrator history is also their control surface: queued list
and detail rows have a **Cancel** action backed by `Store.RequestRunCancel` and
the dispatcher's per-run cancellation latch. Every button's accessible name
includes its run id, including when the same queued run appears in both list
and detail, so administrators never face indistinguishable repeated actions.
The store atomically persists the
request and reason; recovery honors that durable request as `cancelled` if the
process dies before in-memory removal completes. The dispatcher removes it from
its private queue, stamps `cancelled`, and delivers the terminal callback
without ever registering a live job. The same endpoint is race-safe if dispatch begins between render
and submit: it latches cancellation before fork or kills the verified running
process group. Thus moving admission out of the cockpit does not make a
100-deep queue cancellable only by disabling its whole plugin.

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
  import map (`name → { import_id, resource_id, status, error,
  source_delete_pending }`, idempotent on completion), and then deletes the
  source file using descriptor-relative `unlinkat` after
  checking that the current name still identifies the admitted regular file.
  Unix cannot bind unlink atomically to an open descriptor, so the narrow
  check-to-unlink pathname race is accepted under the documented
  same-service-account threat model, just as `mah.fs.discard` accepts it.
  Durable success is first recorded conservatively with
  `source_delete_pending = true`; successful unlink clears the flag before
  callback delivery, while a crash or genuine delete failure leaves it set.
  The import remains `succeeded` and `error` remains empty either way; the
  import map, not the bytes, is what makes re-import idempotent. `on_import`, if given, fires
  at-most-once when the import job reaches a terminal state (contract
  below). Import results remain readable through `mah.fs.runs()` after any
  callback loss. Refused inside DB transactions, like
  `create_resource_from_data`.
- `mah.fs.discard(run_id, name)` → deletes one file.
- `mah.fs.runs()` → the calling plugin's durable run records: `{ id, command,
  status, started_at, finished_at, exit_code, error, output_unverified,
  imports }` — including `interrupted` runs after restart, the
  `output_unverified` flag for the no-pgid crash shape (§3), and each run's
  import map with `source_delete_pending` — so recovery does not depend on a
  live callback.

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
- **Crash-safe destination handling is a prerequisite for replay, and it
  must live inside the hash lock — not in an external preflight.**
  `AddResource` copies its input into a unique temp
  (`resource_upload_context.go:1084-1094`), resolves the destination from
  `mimetype.DetectFile` + `hash + extension` (`1099`, `1251`), and trusts
  the destination if it exists (`1251-1269`) — so a crash during the
  destination copy leaves a truncated file a replay would reuse. An
  external "delete the destination before calling AddResource" preflight is
  **rejected as the mechanism**: two runs may contain identical content,
  per-file claims do not serialize them, and worker B's preflight could
  delete worker A's destination mid-copy (A then commits a resource whose
  backing file was unlinked; B deduplicates on A's row and never restores
  the bytes — `1178-1179` acquires the hash lock inside `AddResource`,
  `1201-1203` short-circuits on an existing row).

  **This is a standing data-integrity fix, not an import-scoped one.** The
  truncation bug is pre-existing in `AddResource` and affects uploads and
  queue downloads **today** (`download_queue/manager.go:103`,
  `contracts/resource_interfaces.go:19` are among its callers); commands
  only make it likelier by moving multi-GB files. The validation therefore
  lands in the shared `AddResource` path and benefits every caller, with a
  **test outside the import path** (an upload-flow crash/replay asserting a
  complete resource). It is recommended that this be landed as its **own
  change** before (or alongside) the commands work: a standing fix buried
  in a commands PR gets reverted by accident when commands is rolled back.

  The import path uses that shared fix — with the committed-row lookup
  strictly first.
  Under the hash lock, the existing committed-resource lookup and merge
  (`1201-1203`) remains the first branch: destination validation/replacement
  happens **only after that lookup returns not-found**, and before the
  file-existence reuse branch. There the destination file is validated
  against the **immutable input snapshot** — if it exists and its size
  differs from that snapshot's, it is replaced with a full copy. Importing
  already-committed identical content therefore never touches its backing
  file: the row lookup short-circuits first (asserted by test). A size
  check suffices to catch copy-truncation (a crash mid-copy leaves a
  strict prefix, which cannot have the snapshot's size); matching sizes is
  the ordinary post-copy-crash case and is correctly reused. Each attempt
  derives hash, MIME and size **freshly from its own snapshot** — the size
  of the
  snapshot being imported is compared against the destination that the
  same snapshot resolves to via the mime-detected extension; comparing a
  newly measured source against a cached prior-attempt destination is
  prohibited (changed content selects a different destination, so a
  changed source can never be mistaken for a truncated copy). A
  **mid-destination-copy crash/replay test** (interrupting the copy under
  the hash lock) and a **concurrent same-content import test** (two workers,
  identical content) assert replay publishes complete resources and
  no worker loses its backing file.
- **Import temp files are bounded and reclaimed, including AddResource's
  snapshot.** Import does not make an outer full-file copy before calling
  AddResource. Instead an exact-size, cancellation-aware reader streams the
  admitted exchange descriptor into AddResource's own immutable scratch
  file. Import-owned invocations route that scratch into the **claim's managed
  temp directory** (`<staging root>/import_tmp/<import-id>/`) via an
  options-aware internal helper — the public three-argument `AddResource`
  method on `contracts.ResourceCreator` and the download queue's usage stay
  unchanged, and ordinary uploads keep their `HLSTempDir` behaviour. The
  exchange source plus one scratch copy makes the single-file admission bound
  approximately `2 × source size`, matching the documented merge-peak sizing;
  the previous outer-plus-inner scheme incorrectly required `3 ×`. Every
  import temp is inside `import_tmp`, never appears in `mah.fs.list`, and is
  covered by one accounting rule: the per-run exchange quota covers the run's
  exchange folder **plus** its import temps, and the global staging quota
  covers the whole staging root including `import_tmp`. Cleanup is immediate on the import's terminal
  state (success or failure); startup recovery deletes any temps whose
  claim is terminal or `interrupted` — partial temps are unusable by
  design, since re-import always copies fresh. Tests cover quota accounting
  including temps, **orphan-temp cleanup after a crash following the inner
  upload temp having been populated**, and reclamation after restart.
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
    resource_id = n|nil, error = string|nil,
    source_delete_pending = bool }
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
     and (for runs with a submitter) belong to the acting user. Only a row
     created with `actorless_at_submission = true` is accessible to any
     principal acting for the plugin. When deletion nulls an ordinary run's
     actor, that flag remains false and Lua access fails closed; it does not
     turn the row into an intentionally actor-less schedule run.
  2. **Finished state**: the run must be terminal; file operations against
     a running or queued run are refused. Recovery never makes a recorded
     live-but-unverifiable group terminal. The no-pgid crash shape is flagged
     `output_unverified`; its file operations are refused with an `output
     unverified` error except `discard_run`.
  3. **Name validation** (lexical): the name must match the plugin-visible
     character set (no `/`, `\`, null bytes, no `.` or `..`, length-capped).
  4. **Import claim short-circuit** (`create_resource` only): if the durable
     import map already records the name — `succeeded` → return the existing
     resource id; `pending`/`running` → return the existing import id;
     `interrupted` → re-enqueue the **same** claim id (bindings refreshed);
     `failed`/`cancelled` → a new claim replaces it — **before** any
     file-existence check. This is what makes re-import idempotent after the
     source file was deleted (successful cleanup or later sweep); `read`
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

- A sweep pass processes a bounded batch of finished runs whose configurable
  retention has elapsed (default 7 days, measured from **completion**, not
  creation) and whose `exchange_removed_at` is still NULL. Successful removal
  or an already-absent directory stamps that field; later passes never revisit
  the historical row. A crash after deletion but before the stamp causes one
  harmless retry. A run is skipped by the sweep while it has an active
  file operation — import, read and discard each hold their lease — or a
  nonterminal run record **or any nonterminal import** (`pending`/`running`),
  because the import pin is taken at **claim/admission time**, not at worker
  start: an import accepted against a run whose retention has already
  elapsed must not be swept out from under the worker that will eventually
  run. Lease acquisition and the sweep's skip-check are coordinated under
  the same per-run lock, so an operation cannot slip between the check and
  the sweep. When retention elapses, **everything** in the exchange
  directory is deleted, including any `source_delete_pending` bytes; what
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
  refusing the run submission with an explanatory error. **What this
  validation does and does not defend:** it covers the literal operator
  string; the interpolated result is a different surface — `%(title)s`
  values come from remote, uploader-controlled metadata. yt-dlp sanitizes
  separators in field values, so interpolation is almost certainly safe in
  practice, but the spec does not rest on that: the **load-bearing defence
  is structural** — `--paths` pins the root, `--no-directories` forbids
  subdirectories, the folder is per-run and flat, and `mah.fs` addresses
  only top-level regular files (§5). The validation is defense-in-depth
  for the operator-supplied half, not the wall the remote half leans on.
- The default format selector `bestvideo*+bestaudio/best` makes yt-dlp
  **shell out to ffmpeg** for the merge: the operator note must say ffmpeg
  must be on the trusted `PATH` too (or the setting changed to a
  pre-merged single format).
- The operator note must also say: **size the per-run exchange quota for
  the merge peak, not the final size** — with the default selector, peak
  disk use is roughly 2× the final size because video and audio download
  separately before muxing (§3).

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
- terminal `output_unverified` runs (the no-pgid crash shape) → offered for
  `discard_run`; a recorded live-but-unverifiable group is not terminal and
  keeps command surfaces closed while the staging lease remains held.

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
  stale grant). Intentionally actor-less runs (`actorless_at_submission=true`)
  require the current acting principal to hold `db:write`, and a claim is
  attributed to that principal. User deletion nulls ordinary run/import actor
  columns through `stampedModels`, but ordinary runs retain
  `actorless_at_submission=false`: Lua access fails closed and pending claims
  cannot start or be re-driven through that run. The invariant and its reason
  are documented beside the other fail-closed models in
  `application_context/user_admin_guard.go`.

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
  rejects paths and dashes. Command-path tests put a rogue same-named binary
  earlier in the ambient process `PATH` and prove an explicit
  `-plugin-command-path` wins; empty/relative configured entries fail startup.
- Refused in transaction (both `run` and `create_resource`); parameter count
  and size caps; per-plugin and global concurrency **on the command pool
  (2 per plugin, 4 global), separate from the download pool (3)**; timeout
  kills the **whole process group** — a regression test uses a child that
  spawns a long-lived descendant and asserts the descendant dies too. A
  Darwin regression launches an Apple platform executable whose environment
  is unavailable and proves local cancellation still kills its descendant
  before terminal publication. A fake-unverified inspector gives the same
  proof on every Unix test host. Process inspection call counts prove an
  ordinary long-running parent is not polled every 20 ms.
- Quotas: a run whose writes exceed the per-run exchange quota is killed and
  marked `failed` (error names the quota); while the sampled global staging
  usage is above the limit, new runs are refused. An import at the `2 × source`
  boundary succeeds and one beyond it fails before copying. Submission tests
  prove `Prepare` reads the cached sample without walking the staging tree;
  startup and sweep refresh failures remain explicit.
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
  durable run record readable after the 1-hour queue retention expires;
  **disable in the pre-fork window** — disable before the fork prevents the
  spawn, disable between fork and pgid persistence latches cancellation so
  the just-spawned group is killed and reaped before terminal publication.
- Crash recovery: restart with a **surviving orphaned process group** (a
  grandchild writer still alive) — an identity-verified group is killed before
  the record is stamped `interrupted`; a recorded live but unverifiable group
  is not killed or stamped terminal, quarantines command surfaces while the
  server boots, and heals automatically after later recovery observes death.
  The guarantee is that output **cannot be imported while writers remain
  alive**, not that killed writers' files are complete: an `interrupted` run's
  files may be partial, and its status says why. A **spawn/persist crash** with
  no pgid remains `interrupted + output_unverified`, file operations refused,
  `discard_run` allowed. A **pgid reuse** test proves no unrelated process is
  killed and no terminal row is published while that group remains alive.
- stdin: the spawned process's stdin is `os.DevNull` (a prompting tool fails
  on timeout, never blocks forever).
- `mah.fs`: enforcement tests for name validation, symlink refusal
  (including a swap-after-lstat race), directory/special refusal, cross-plugin
  and cross-run access refusal, actor checks, and **deleted-actor
  fail-closed behavior**: nulling an ordinary submitter leaves
  `actorless_at_submission=false`, hides the run from every plugin principal
  and prevents its pending claim from starting, while a run born actorless
  remains plugin-accessible; idempotent re-import after a cleanup failure
  (`source_delete_pending`, successful import error remains empty) **and after
  the run has been swept** (import-claim
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
  download history cannot see command records); every queued-run cancel button
  has a distinct accessible name containing its run id.
- **Staging/import**: a **real subprocess** (not a stub) writes a file that
  `mah.fs.create_resource` imports through the configured afero filesystem —
  including a **MemoryFS target** (small files), proving the import path
  works through `MemMapFs`.
- **Destination-integrity fix, outside the import path**: the shared
  `AddResource` validation is tested from an ordinary **upload-flow
  crash/replay** (no commands involved), since the fix is a standing
  data-integrity change for uploads and queue downloads too.
- **Output-tail retention**: `plugin_command_run_output` rows older than
  the retention are pruned by the sweep while the run row, claims and map
  survive.
- Sweep: retention measured from completion; runs with active import, read
  or discard leases skipped (lease/sweep lock coordination); nonterminal runs
  skipped; expired runs' directories fully deleted including
  pending-delete bytes, with the import map surviving; fresh runs kept.
- Queue integration: FIFO per-plugin dispatch, independent per-plugin pending
  caps, and command/import pool caps; 100 queued commands create zero entries in
  the ordinary download registry and an ordinary download still admits; only
  dispatched work enters the six-entry managed-live lane; its occupancy is
  derived from `m.jobs`. Tests independently remove terminal managed entries
  through managed admission's `evictJob` path and through `cleanupOldJobs`'
  direct map deletion, and each immediately restores a slot without counter
  maintenance. An administrator can cancel a queued run
  from command history (no process group exists), while a dispatch race reaches
  the same latch/group-kill path; no pause and no retry are exposed for command
  jobs; terminal state reaches the
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
  permitted operation on terminal `output_unverified` runs; `on_import` receives the
  documented result table and is not fired by the submission-time
  short-circuit; **disable → re-enable → page reconciliation** re-drives
  interrupted claims (same id) and offers failed/cancelled claims for an
  explicit operator retry that creates a **new** claim id — nothing
  automatic; **import temp accounting**: per-run and global quotas include
  `import_tmp` (which holds the single `AddResource` snapshot scratch via the
  scratch-dir option), and startup recovery deletes orphaned temps of
  terminal or interrupted claims — including a crash after the inner
  upload temp was populated; **concurrent same-content import**: two
  workers importing identical content never lose a backing file (the
  destination validation runs under `AddResource`'s hash lock, not as an
  external preflight). **Retention scale**: sweeps fetch a bounded batch of
  expired rows with `exchange_removed_at IS NULL`, stamp deleted or absent
  directories, skip leased/nonterminal-import runs without stamping, and never
  reconsider stamped historical rows.

Manage UI: warning panel renders verbatim (shell-quoted, escaped) commands;
enable flow shows it and requires the acknowledgement; a changed command set
forces re-consent; legacy-record upgrade paths covered by tests per §2.
**CLI**: `mr plugin enable` on a command-bearing manifest without
`--confirm-commands` is refused and prints the verbatim command list; with
the flag the acknowledgement is recorded (the CLI is a deliberate second
step, not a bypass — tested that the refusal text renders argv exactly as
the panel does).

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
