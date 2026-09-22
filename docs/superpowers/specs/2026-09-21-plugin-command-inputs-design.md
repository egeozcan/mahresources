# Plugin-command input files

Status: requirements, not yet implemented. Requested 2026-09-21. Reviewed and
decided 2026-09-21; the decisions taken in that review are recorded here and in
`docs/adr/0005-plugin-command-inputs-are-declared-names.md`. This file is the
single source of truth for the feature.

## Why

A declared command can write files into its exchange folder, and a plugin can
read them back. Nothing goes the other way: `mah.fs` installs `runs`, `list`,
`read`, `discard`, `discard_run` (plus `create_resource` and
`set_resource_thumbnail` under `db:write`), and a run parameter can only replace
one whole argv element. So a plugin cannot hand a program a file it wrote, and
cannot hand it text that only makes sense in a file.

The motivating case is yt-dlp cookies. `--cookies FILE` is the supported way to
authenticate a download, and the file it wants is the only thing a plugin cannot
produce. The workaround yt-dlp still accepts, `--add-header "Cookie: ..."`, is
answered with `Deprecated Feature: Passing cookies as a header is a potential
security risk`, so building a plugin on it trades a small host feature for a
dead end.

This spec adds the missing direction: the plugin may supply named *input files*
to one command run, and the host writes them into that run's exchange folder
before the process starts.

## Goal

`mah.commands.run` may be given file contents. The names are validated
synchronously against the command's manifest declaration, in the same call stack
as the Lua call, so a refusal is `nil, error` with nothing durable created and
nothing written. The host then writes the contents into the run's own exchange
folder in the runner's start path, immediately before the program is spawned, and
keeps their contents out of every record it stores. The program reads them
exactly as it would read any file in its working directory, which is already the
run's exchange folder (`plugin_commands/runner_unix.go`, `Dir: run.ExchangeDir`).

## Trust model

This does not grant a new *kind* of power: `commands` already means "this plugin
may run these argv templates as the service account", and a parameter already
lets it shape one argument of each. What is new is that the folder the program
runs in starts with contents the plugin chose, so a program that reads a file
from its working directory without being told to — `.netrc`, `.gitconfig`,
`config`, whatever its own rules look for — can be fed one. Two consequences,
both deliberate:

- The manifest declares the names, so the operator consents to a list, not to a
  capability. A manifest that supplies a name its declaration does not list is
  refused.
- A leading dot is refused outright, so the names the common tools look for
  without being asked (`.tmp`, `.netrc`, `.env`, `.gitconfig`) cannot be
  supplied at all. This is hygiene for the operator's mental model, not a
  boundary: paths, stdin and environment channels are the boundaries, and all
  three are refused.

The host still does not interpret the program: it cannot enforce that a declared
input is used, only that it was declared and is writable. A declaration is not
required to reference a declared input in its `argv`; this is a convention the
operator sees in the consent panel, the same status the existing rule gives to
"a placeholder must not occupy a flag position".

## Non-goals

- No arbitrary paths. A supplied file is a name in the run's own folder, never a
  directory, an absolute path, or anything containing `/`, `\`, `..` or NUL.
- No manifest-carried static input files. Contents are supplied per run and
  nothing else; a fixed file an author needs can be passed as a Lua string on
  every run, and per-run contents subsume the static case.
- No binary or streaming inputs. Bounded bytes only: no encoding is validated,
  and the size bound is the whole contract.
- No stdin channel (the child's stdin stays the null device) and no environment
  channel (the environment allowlist is unchanged).
- No new read authority: Lua still reads exchange files only through
  `mah.fs.read`, and `.tmp` stays invisible to `mah.fs.list`.
- No encryption at rest, no per-run guaranteed deletion, no Windows support
  (declared command runs remain Unix-only).
- No symmetry requirement: the host does not stage *output* files anywhere new.

## 1. Manifest

`commands[].inputs` is a new optional list of file names on a command
declaration, alongside `argv`, `timeout` and `sensitive_params`.

```lua
commands = {
    {
        name = "fetch_video",
        argv = {
            "yt-dlp", "--ignore-config",
            "--paths", "{{exchange_dir}}",
            "-o", "%(title)s [%(id)s].%(ext)s",
            "--cookies", "cookies.txt",
            "--", "{{url}}",
        },
        inputs = { "cookies.txt" },
        timeout = 7200,
        sensitive_params = { "url" },
    },
}
```

Parse-time rules, failing loudly like the rest of the manifest parser:

- Every name must satisfy the exchange file-name grammar the read path already
  uses (`plugin_commands/exchange.go`: non-empty, not `.` or `..`, no `/`, `\`
  or NUL, at most `MaxFileNameBytes` = 255 bytes) **and** must not begin with a
  dot.
- Names must be unique within a declaration, at most `MaxInputFiles` = 4.
- `inputs` without the `commands` capability is a load error, like `commands`
  itself.
- `inputs` participates in manifest identity and in the consent record exactly as
  `argv` does: adding, removing or renaming a declared input is a widening that
  refuses to load until the operator re-enables the plugin.
- Order is display order, not identity. `SameDeclarations` compares the declared
  names as a set, exactly as it compares `sensitive_params`, so reordering the
  list must not force re-consent.
- A declaration is not required to reference a declared input in its `argv`. The
  host cannot parse an arbitrary program's argument grammar, so this is a
  convention the operator sees in the panel — the same status the existing rule
  gives to "a placeholder must not occupy a flag position".

## 2. Lua surface

```lua
local run_id, err = mah.commands.run("fetch_video", params, callback, {
    inputs = {
        ["cookies.txt"] = "# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tSID\t...\n",
    },
})
```

- The signature is `mah.commands.run(name, params [, callback] [, options])`.
  The third argument stays the callback or `nil` — it is never overloaded — and
  the options table is the fourth. Supplying inputs without a callback therefore
  reads `mah.commands.run(name, params, nil, { inputs = ... })`. Every existing
  two- and three-argument call behaves exactly as today.
- The options table is checked strictly: an unknown key is refused by name, so
  `input = { ... }` cannot silently produce a run with no file.
- `inputs` is a string→string table. Keys are declared names; values are the
  contents to write. Tables, numbers, functions and nil values are refused.
- No key may be present that the named command's declaration does not list. The
  error names both the file and the command.
- Supplying nothing (`inputs` absent, or an empty table) is legal and means the
  run's folder simply starts empty.
- A zero-byte value is legal and writes a zero-byte file. Absent and empty are
  different things, and the host does not interpret the program: §10's yt-dlp
  guidance ("no cookies" must be an absent file) is the plugin's business.
- A name may be supplied at most once (it is a table key).
- The call is refused inside `mah.db.transaction`, exactly as `run` already is,
  and refused when the command runtime is unavailable or the plugin is disabled
  — no file is written in those cases.

Refusal wording, checked synchronously with no I/O:

| Condition | Message |
|---|---|
| Undeclared name | `command %q does not declare input file %q` |
| Leading dot | `input file %q must not begin with a dot` |
| Bad grammar, over `MaxFileNameBytes` | `input file %q must be a plain file name` |
| Over `MaxInputFiles` | `command %q received %d input files; maximum is %d` |
| Over `MaxInputBytes` | `input file %q is %d bytes; maximum is %d` |
| Over `MaxAggregateInputBytes`, or the per-run quota | `input file contents total %d bytes; maximum is %d` |
| Non-string value | `input file %q contents must be a string` |
| Unknown options key | `command input options have unknown field %q` |
| Options not a table | `command input options must be a table, got %s` |
| Options field not a table | `command input options field %q must be a table, got %s` |

## 3. Ordering and atomicity

The feature is two phases, in two different places, and the split is what makes
both the synchronous refusal and crash safety true at once.

**Phase 1 — validation, synchronous, no I/O.** `ValidateInputs(Declaration,
map[string]string) ([]InputFile, error)` lives in `plugin_commands` and is called
from the same two places `BuildInvocation` is: the Lua layer (`plugin_system`)
as a pre-check, and `dispatcher.Submit` before its `Prepare`/`CreateRun`
sequence. It applies every rule in §2 and §4. A refusal returns `nil, error` to
Lua, creates no durable row, and writes nothing.

**Phase 2 — the write, in the runner's start path.**
`commandExecutor.Execute` already does, in order, `MarkRunRunning` →
`stopBeforeStart` → `normalizeCommandPath` → `resolveExecutable` → open
`/dev/null`. The write goes immediately after that open and before any pipe is
created, so every cheap detectable failure — a cancelled-before-start run, an
unset `PLUGIN_COMMAND_PATH`, an unresolvable executable — happens before a secret
reaches disk.

- Each input is written as a top-level regular file in the run's exchange
  folder, mode 0600, in declaration order.
- The write is atomic with respect to a later reader: stage the bytes at a
  host-chosen random name inside the run folder's existing private `.tmp`
  directory (0700, and already the child's `TMPDIR`), then `rename` into the run
  folder. Rename within one directory tree is atomic, so a name the plugin
  supplied is never partially visible, never an empty file still being filled,
  and never missing once the spawn happens. Scratch debris left by a crash is
  inside `.tmp`, which `mah.fs.list` already skips (it lists regular files only)
  and `mah.fs.read` cannot reach.
- After all the writes, each target is verified by `Lstat` before the spawn:
  regular file, exactly the expected size, mode 0600. A mismatch refuses the
  spawn.
- If any write or verification fails, **no process is spawned**: the run is
  finished through the existing failure path with `write input file %q: <error>`
  (or the verification message), naming the offending file. The scratch file is
  removed; the exchange folder is left to the retention sweep, as every other
  failed run's is. The run is durable and terminal, so it never claims a live
  dispatch.
- Nothing is written outside the run's own exchange folder, and no path
  component comes from Lua. Symlink and regular-file rules match the read path
  (top-level, regular file, no following links).

Consequences, all of them wanted:

- A run cancelled while queued writes nothing at all, because
  `stopBeforeStart` runs first.
- A crash between the write and the spawn leaves a durable nonterminal row that
  `Recover` already settles and the retention sweep already reaps. It does not
  leave an orphan folder: `Prepare` runs in `Submit` before `CreateRun`, so an
  early crash window remains only where it already exists today and is not
  widened by up to 512 KiB of secret bytes. The declared name is absent rather
  than partial.
- Recovery never re-dispatches a queued or running command, so there is no
  re-write path to design and no second write of the same run.

## 4. Limits and quota

- `MaxInputFiles` = 4 per run.
- `MaxInputBytes` = 256 KiB per file. `mah.fs.read`'s `MaxReadBytes` (4 MiB) is
  larger, so any supplied file can be read back in one call.
- `MaxAggregateInputBytes` = 512 KiB per run.
- Exceeding any of these refuses the run synchronously: `nil, error`, no durable
  row, nothing spawned.
- The parameter and input budgets are separate (64 KiB + 512 KiB), so the
  worst-case in-flight payload of one run is documented as 576 KiB.
- Supplied bytes count against the run's exchange quota
  (`-plugin-command-run-quota`, 8 GiB by default). The per-run quota is otherwise
  sampled only *during* execution, so admission gains one explicit check:
  supplied bytes plus the measured run-directory usage must not exceed
  `PerRunQuota`. A run whose inputs alone exceed the quota is refused there.
- The bytes are visible to the global staging sample, because they occupy the
  staging root exactly like command output does. The sample is refreshed at
  startup and after each retention sweep (`dispatcher.go`), so a test asserting
  it after a run must refresh it first.
- Constants live with the other `plugin_commands` limits so a single place
  documents them.

## 5. Records and redaction

- File contents must never appear in: the durable run row, the output row, the
  argv (executed or redacted), the parameter view, the admin history, plugin
  logs, or `-v`-style debug output the host emits. Contents exist only as the
  file in the exchange folder.
- The run row gains `InputsJSON`, an ordered JSON array
  `[{"name":"cookies.txt","bytes":1234}]`, empty when nothing was supplied. It
  is written with the row at `CreateRun`, and the sizes are the **submitted**
  byte counts, never a live `stat`, so a swept folder or a program that rewrote
  its input cannot make the record lie.
- It must never be folded into `ParamsJSON`. That column is the parameter view:
  it is what `runViewToLua` hands Lua, and it is the namespace `BuildInvocation`
  substitutes in, so an `inputs` key there would look like a parameter and could
  collide with a declared one.
- `plugin_commands.RunRecord` exposes `Inputs []SuppliedInput{Name, Bytes}`,
  decoded once in the store with a tolerant decode (a malformed column logs and
  reads as empty; history must never fail over metadata). Both the Lua run view
  and the template provider read that one decode.
- The Lua run view (`mah.fs.runs`) exposes the names and sizes for parity, so a
  completion callback can discard precisely. Contents are never exposed.
- The admin history page renders a small table beside the existing Imports
  table: file name and bytes supplied, with an accessible caption and a note
  that contents are never recorded. No contents, and no download link.
- `plugin_system.CommandDisplay` gains `Inputs []string` (JSON `inputs`), the
  declared names, so the enable panel, the `--confirm-commands` refusal body and
  the CLI all read one list.
- `sensitive_params` keeps its current meaning for parameters. It has no role
  here: input contents are not parameters, and a name is not a secret.

## 6. Consent

- The enable-time panel (both render sites in `templates/managePlugins.tpl`) and
  the `mr plugin enable --confirm-commands` output must show each command's
  declared input names beside its argv, in wording that says what they mean.
  Suggested copy: *"This command may be given the contents of these files,
  written into the private folder it runs in: cookies.txt. Their contents are
  not shown here and are not reviewable; you consent to the names only. They
  stay in that folder until the exchange folder is swept (7 days by default)
  unless the plugin discards them."*
- The capability label for `commands` gains the same clause. The five phrases
  pinned by `TestCommandCapabilityCataloguesDescribeTheHostPrivilege` must keep
  passing; the test is extended, not loosened.
- `Grants.Commands[].inputs` carries the declared names, so an unchanged
  manifest is recognised as unchanged. A stored record from before this feature
  has no `inputs` field at all, which makes declaring one a widening — exactly
  what is wanted, and it needs no migration.
- Changing a declaration's `inputs` requires re-consent, and the widening is
  reported by `CompareGrants` (`sameCommandGrant`, the `ChangedCommands` list)
  the way an argv change is.
- Documentation and CLI help state the same thing in the same words:
  `docs-site/docs/features/plugin-system.md`,
  `docs-site/docs/features/plugin-permissions.md`,
  `docs-site/docs/features/plugin-lua-api.md`,
  `docs-site/docs/api/plugins.md`, and
  `cmd/mr/commands/plugins_help/plugin_enable.md`.

## 7. Retention and secrets

- Input files are retained with the run's exchange folder under
  `-plugin-command-exchange-retention` (168 h by default). Nothing deletes them
  earlier.
- The plugin can read a supplied file back with `mah.fs.read` and remove it with
  `mah.fs.discard` or `mah.fs.discard_run`. This is deliberate: a program that
  rewrites what it was given (yt-dlp refreshes its cookie jar in place) produces
  a new value the plugin may want, and a plugin that does not want it can delete
  it as soon as the run is terminal.
- Documentation must state the worst case plainly: a supplied file can outlive
  the plugin's completion callback, a plugin disable (which unloads the VM and
  so may drop the callback), and a cancellation, up to the retention window. A
  run cancelled while it is still queued leaves no file, because the runner
  observes that cancellation before it writes anything; a cancellation arriving
  once the run has begun preparing cannot un-write what is already staged, so
  cancelling is not a way to remove a supplied credential — the plugin discards
  it, or the sweep removes it.
- No per-run guaranteed deletion, no encryption at rest: the retention window is
  the price of the plugin being able to read a rewritten file back.

## 8. What must not change

- Runs without `inputs` behave exactly as today, including every refusal path.
- `mah.fs` gains no write function; the exchange folder stays write-only for the
  command, not for Lua.
- `commandExecutor.Prepare` keeps its current sequence and meaning (quota →
  plugin dir → exchange dir → `.tmp`), and `Submit`'s ordering — `Prepare`, then
  queue admission, then `CreateRun` — is unchanged.
- Parameters, redaction, timeouts, the per-plugin FIFO queues, the managed-job
  lane, recovery, the staging lease and the Windows refusal are untouched.
- `.tmp` remains invisible to `mah.fs.list`, and a supplied name can never
  collide with it because a leading dot is refused.
- `BuildInvocation`, `RedactedArgv` and `ParamView` are unchanged; inputs travel
  in their own field and their own record.

## 9. Acceptance tests

Against the real runner and store, not a mock:

1. **Present before spawn.** A fixture command reads `cookies.txt` from its
   working directory and writes what it read to another file. The run's listing
   contains that file with the exact supplied bytes, and the input's mode is
   0600.
2. **Undeclared name refused.** `inputs = { ["other.txt"] = "x" }` against a
   declaration listing only `cookies.txt` returns `nil, err`, creates no durable
   run row, and spawns nothing.
3. **Name grammar refused.** `../x`, `a/b`, `a\\b`, `""`, `.`, `..`, `.tmp`,
   `.netrc`, and a 256-byte name are each refused.
4. **Limits.** Five files, one 257 KiB file, and 513 KiB total are refused; four
   files, 256 KiB, 512 KiB are accepted.
5. **Quota.** Inputs alone over `-plugin-command-run-quota` refuse the run; the
   global staging sample reflects written inputs after a completed run and a
   `Usage.Refresh`.
6. **Nothing leaks.** After a run with a distinctive secret in an input, the run
   row, output row, argv, parameter view, history JSON, and captured logs
   contain the name and size, and none of them contains the secret. The
   parameter view contains no input key at all.
7. **Consent.** Adding a name to `inputs` makes the plugin refuse to load until
   re-enabled; the panel and the CLI both print the name; an unchanged manifest
   with a stored consent record loads without re-consent; reordering a
   declaration's names also loads without re-consent.
8. **Retention and discard.** The input survives the run; `mah.fs.discard`
   removes it and `mah.fs.read` then fails; the retention sweep removes the whole
   folder on schedule.
9. **Crash safety.** Killing the host between the write and the spawn leaves a
   state the existing recovery path resolves without new stuck states; a kill
   between the scratch create and the rename leaves the declared name absent
   rather than partial, and leaves no listable or readable debris. The state
   such a kill must leave is exactly: a durable nonterminal row, the exchange
   folder, and either a completed input file or a partial file inside `.tmp`
   with no declared name. That artifact set is what the tests pin, at the state
   level; the window itself is sub-millisecond, so it is not reproduced by a
   real signal from another process.
10. **Failure path.** An injected write error finishes the run as failed, names
    the file in the error, spawns nothing, and leaves the exchange folder to the
    existing retention sweep.
11. **Options shape.** `run(name, params, nil, opts)` works; a non-table opts,
    an unknown key (`input`), a non-string value, and a table where the callback
    belongs are each refused with the §2 wording.
12. **Zero bytes.** A zero-byte input is written as a zero-byte file and reads
    back as empty; omitting the name writes no file at all, so absent and empty
    are distinguishable.
13. **Queued cancellation writes nothing.** Cancelling a run while it is queued
    leaves the exchange folder without any supplied file, and the run is
    `cancelled`.
14. **Record shape.** `InputsJSON` is the ordered `[{name, bytes}]` array; the
    admin history renders the names and sizes and never the contents; the Lua run
    view exposes the same names and sizes.
15. **Nothing written before a doomed spawn.** With an unresolvable
    `PLUGIN_COMMAND_PATH` entry (or an unknown executable), the run fails and the
    exchange folder contains no supplied file.
16. **Verification catches a short write.** An injected short write or a
    size-changing rename target refuses the spawn and finishes the run failed
    with the verification message.
17. **Scratch isolation.** While a write is in flight, the declared name never
    appears in `mah.fs.list` (which cannot see it either way before terminal
    status) and the `.tmp` scratch is never listable or readable.
18. **In-memory residency.** After the write, the in-flight request held by the
    dispatcher carries names and sizes only; a test asserts the queued
    `QueuedRun` no longer holds the contents.

## 10. Consumer acceptance (yt-dlp plugin)

The plugin that asked for this will declare `inputs = { "cookies.txt" }` on its
four fetch commands and add `--cookies cookies.txt` to their argv. Then:

- With cookie text supplied, the download authenticates and the plugin discards
  `cookies.txt` in its completion callback; the admin history shows the name and
  size, never the text.
- With no cookie text supplied, the same argv is used and no file is written;
  yt-dlp creates an empty jar in the run folder, which the plugin discards with
  the rest of the run's leftovers. (An empty file is *not* valid input to
  `--cookies` — yt-dlp rejects a zero-byte file as "does not look like a
  Netscape format cookies file" — so "no cookies" must be an absent file, not an
  empty one. The host permits a zero-byte input; it is the plugin's business not
  to supply one here.)
