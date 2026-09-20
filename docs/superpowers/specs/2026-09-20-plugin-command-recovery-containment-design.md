# Plugin command recovery containment design

**Date:** 2026-09-20  
**Status:** Revised after written-spec review; awaiting approval
**Scope:** Follow-up to `2026-09-19-plugin-commands-design.md` and `2026-09-20-plugin-command-runtime-remediation.md`

## Problem

Plugin-command recovery currently treats a safety refusal as a process-startup
failure. A persisted PGID that is alive but cannot be tied to its run makes
`Dispatcher.Recover` return an error; `main` then exits before serving HTTP. The
same happens when a verified owned group ignores termination for ten seconds,
and when a rolling replacement cannot acquire the old process's staging-root
lease. The safety invariant is correct — uncertain rows must remain
nonterminal, one runtime must own a staging root, and command surfaces must stay
unavailable — but disabling the entire application is not required to preserve
it.

PGIDs are scoped to one host boot. After a reboot, a persisted integer may name
an unrelated process group even though the original command cannot still be
running. Recovery may use a host-unique, boot-invariant session UUID to prove
that case, but the proof has a dangerous direction: a false mismatch publishes
terminal state while the original writer may still exist. A clock-derived boot
time is therefore not an acceptable identity.

Finally, a live worker whose group never becomes observably dead polls the whole
process table every 20 ms forever. Its refusal to publish terminal state is
correct, but the steady-state inspection cost and lack of operator diagnostics
are not.

## Goals

1. Lease contention or a command-recovery safety blockage disables only the
   plugin-command runtime, not HTTP, downloads, resources, or unrelated plugin
   surfaces.
2. Quarantine heals automatically when lease ownership or process-group safety
   becomes resolvable; zombie-only groups are considered dead on platforms where
   process state can be enumerated.
3. An acquired staging-root lease remains held throughout recovery, quarantine,
   activation, and complete dispatcher quiescence.
4. A PGID paired with a different known host-unique boot-session UUID is never
   inspected or signalled.
5. Infrastructure and consistency errors remain fatal during the initial
   synchronous startup attempt.
6. A stuck live worker remains nonterminal but backs polling off, identifies the
   capacity it pins, and makes at most one bounded re-signal after observing the
   group alive.
7. Boot identity remains host-internal and is never exposed to plugin Lua or the
   administrator command-history view.

## Non-goals

- Publishing partial `mah.commands` or `mah.fs` functionality while recovery is
  blocked.
- Automatically discarding, importing, or marking terminal output that may
  still have a writer.
- Building a cross-host process supervisor or distributed staging lease.
- Adding an administrator repair UI in this follow-up.
- Implementing zombie-aware process enumeration on AIX, DragonFly BSD, FreeBSD,
  NetBSD, OpenBSD, or Solaris. Their existing probe-only inspector cannot prove a
  zombie-only group dead; quarantine there may require the parent to reap it or
  a server restart. Linux and Darwin, including the shipped container path, are
  covered by zombie-aware enumeration.
- Changing the documented approximate global staging quota or retention batch.

## Design

### 1. Use only host-unique boot-session identities

The platform provider returns one immutable string for the current process:

- Linux reads and trims `/proc/sys/kernel/random/boot_id`.
- Darwin reads `kern.bootsessionuuid` with `unix.Sysctl`.
- Other Unix targets report identity unavailable in this version.

`kern.boottime` is deliberately not used. It is derived from realtime and
uptime, can move after clock adjustment or sleep/wake, and is not host-unique.
A false difference would take the unsafe branch. BSD targets without a reliable
host-unique session UUID retain the existing fail-closed process inspection
rather than approximating identity with a timestamp.

Identity lookup is hardening, not a new startup dependency. An unavailable or
failed lookup is logged once and represented as unknown; recovery then uses the
existing environment-based ownership inspection. Equality is exact because the
only comparable values are boot-session UUIDs.

The runtime resolves its current identity once. It is never read per run or per
poll.

### 2. Pair boot identity atomically with the PGID

A queued row has neither a PGID nor a boot identity. After fork,
`SetRunProcessGroup` writes the positive PGID and current boot-session UUID in
the same conditional database statement. The invariant is structural: the
process that produced the persisted PGID also supplied the identity that
qualifies it.

The database model gains a private boot-identity column, but ordinary
`RunRecord`, `RunView`, administrator history, templates, JSON, and
`runViewToLua` do not. Recovery uses a dedicated internal `RecoveryRun` shape
that combines the public run fields with the private identity. Tests must prove
that `mah.fs.runs()` and admin rendering never expose it.

Rows from older releases have an empty identity and use process inspection.

### 3. Classify a proven prior-boot row without touching its PGID

For a running row with a positive PGID:

1. If both boot-session UUIDs are nonempty and differ, recovery does not call
   `InspectGroup` or `KillGroup`.
2. The row becomes `interrupted` (or `cancelled` when its durable cancellation
   latch already won) with `output_unverified=true` and a reason recording the
   prior-boot classification.
3. If either UUID is unavailable, or they are equal, recovery follows the
   existing ownership inspection.

A different host-unique session UUID proves that the persisted local PGID was
not created in this boot. It does not prove output completeness, so output
remains unverified and only discard is allowed.

### 4. Preserve zombie exclusion while tolerating Linux scan races

Linux process enumeration distinguishes three observations instead of treating
all signal-0-visible groups as writers:

- any parsed non-zombie target-group member proves the group alive;
- one or more parsed target-group members, all zombies, with no unresolved
  target candidate proves `GroupDead`; zombies cannot write, so
  `kill(-pgid, 0)` is deliberately not consulted;
- no target member observed after the bounded scan/retry leaves a race in which
  a member may have appeared or disappeared, so only then may
  `kill(-pgid, 0)` distinguish absent from still present.

A successful or `EPERM` signal-0 probe proves only that a process-table entry
holds the PGID; it does not prove a writer exists. It therefore produces
`GroupAliveUnverified`, never overrides the definitive zombie-only result.
`ESRCH` permits `GroupDead`.

An individual `/proc/<pid>/stat` read or parse failure is retried or classified
through a secondary process-state read. A sample proven outside the target group
is ignored. If a sample still could belong to the target group, inspection stays
fail-closed rather than declaring a zombie-only result. Failure of `/proc`
enumeration or an unexpected liveness-probe error remains an inspection error.

Darwin's existing `kern.proc.all` scan already excludes `SZOMB` before deciding
group liveness and must not add a signal-0 override. Probe-only Unix inspectors
cannot make this distinction; their limitation is explicit in the non-goals and
operator documentation.

### 5. Represent all blockers, separately from recovery failure

`plugin_commands` adds a typed `RecoveryBlockedError` containing the complete
list of `{run ID, PGID, reason}` blockers. `errors.As` distinguishes it from database,
filesystem, configuration, malformed-record, migration, and context errors.

A blocker is collected whenever recovery cannot safely settle a running row
because a group may still exist:

- ownership inspection fails;
- the group is alive but unverified;
- ownership changes before recovered-group termination;
- signalling an owned recovered group fails;
- post-signal inspection fails; or
- the recovered group remains alive past `groupDrainTimeout`.

A cancelled recovery context remains an ordinary context error. Unknown enum
states and malformed durable values remain fatal consistency errors.

Recovery continues after a blocker so it can settle every independently safe
row and report the full blocker set. Blocked rows are never passed to
`FinishRun`. Recovery keeps an in-memory per-run signal-attempt latch: periodic
healing scans may observe death, but do not repeatedly signal the same recovered
group within one server process. This one-attempt recovery bound is deliberate:
recovery has only persisted identity, not the live runner's creation authority;
the runner's separately bounded two-attempt rule is defined in §10.

### 6. Own startup and healing through one runtime controller

`application_context` owns one command-runtime controller with four externally
meaningful states: acquiring, quarantined, active, and stopping. It is the sole
owner of the retry goroutine, runtime lease, not-yet-started dispatcher, active
dispatcher, exchange mediator, and publication state.

The initial synchronous attempt is:

1. Validate settings and plugin-manager dependencies.
2. Try to acquire the staging-root runtime lease.
3. If the lease is busy, enter acquiring quarantine, log it, start the single
   retry loop, and let the server boot. `ErrRuntimeLeaseBusy` is exported and the
   public `AcquireRuntimeLease` error wraps it with `%w`, so `errors.Is` survives
   the staging-root context added at that boundary. An invalid root, permissions
   failure, unsupported lock (`ENOLCK`/`EOPNOTSUPP`), or malformed lease file
   remains fatal and names `-plugins-disabled` in its startup error.
4. Once the lease is held, construct the shared usage cache, executor,
   dispatcher, and exchange dependencies, then run recovery.
5. If recovery reports blockers, retain the lease and unstarted dispatcher,
   enter recovery quarantine, log the complete blocker set, start the retry
   loop, and let the server boot.
6. If recovery succeeds, start the dispatcher, atomically publish the active
   application runtime, then publish the command submitter and exchange mediator.
7. Any other error during this initial attempt remains fatal and releases an
   acquired lease through the existing deferred cleanup.

The controller uses state-specific retry timing:

- acquiring quarantine retries lease acquisition after 1s, 2s, 5s, 10s, 30s,
  1m, then caps at the five-minute sweep interval so ordinary rolling overlap
  heals in seconds;
- recovery quarantine reruns recovery on the retained unstarted dispatcher at
  the flat five-minute sweep cadence;
- once recovery succeeds, it starts the dispatcher and publishes both host
  surfaces without reloading plugins; and
- after the server has accepted quarantine, a later transient infrastructure
  failure cannot retroactively fail process startup. It is logged to stdout and
  `/logs`, remains quarantined, and is retried. Deterministic validation happens
  before quarantine is accepted.

`mah.commands` and `mah.fs` Lua modules are registered from capability grants,
while their atomic host providers are resolved per call. Plugins loaded during
quarantine therefore begin working after publication without reload. Until then,
per-call errors say commands are quarantined until automatic recovery succeeds
and direct the operator to `/logs`; they do not imply an in-progress startup
that can only be fixed by waiting blindly.

`StopPluginCommands` first prevents any later publication, cancels and joins the
retry loop, atomically removes the active application runtime, drains an active
dispatcher when present, and releases the lease only after complete quiescence.
A lease-busy controller owns no lease; a recovery-quarantined controller does.
A second start attempt while any controller state exists remains refused.

With `-plugins-disabled`, `StartPluginCommandsIfEnabled` bypasses controller,
recovery, and lease acquisition entirely. This remains the operator escape hatch
when the plugin subsystem itself should stay offline.

### 7. Make quarantine visible without log spam

Entering acquiring or recovery quarantine writes the same warning to stdout and
the application log (`/logs`, warning level, entity type `plugin_command`). A
recovery warning contains every blocked run ID, PGID, and reason. It explains
that an unverified Darwin orphan may heal only when it exits naturally or when
an operator who has decided the run is abandoned terminates the named process
group (for example, `kill -KILL -- -<pgid>`). A lease warning names the staging
root and explains that automatic retry is active. Both name
`-plugins-disabled` as the restart-time escape hatch.

Every fatal lease error also names `-plugins-disabled`, even though only the busy
sentinel takes the quarantine branch. Retry failures are deduplicated by
state/reason rather than written every interval. Healing emits one informational
application-log/stdout entry. Failure to persist a log entry never changes
runtime safety and remains visible on stdout.

### 8. Make administrator cancellation truthful during quarantine

The durable history page remains available while command execution is
quarantined, but it must not offer a cancellation mutation that this process
cannot enforce. The application exposes runtime availability and its reason to
the administrator history context. While unavailable, queued-run Cancel controls
render disabled with a visible explanation and no duplicate accessible action.

`CancelPluginCommandRun` returns a distinct `ErrCommandRuntimeQuarantined`, not
`ErrRunNotFound`; the API maps it to HTTP 503 (and `Retry-After` when acquisition
retry timing is known). It never writes `cancel_requested` while quarantined: in
lease-busy quarantine another process may own the live runner, and changing only
the durable label would not stop its process group.

### 9. Back off live cleanup polling and log the pinned capacity

Ordinary execution remains free of group polling. Polling begins only after the
direct parent exits or termination starts.

After the first cleanup deadline expires while the group is still alive:

- reset the group ticker from 20 ms to 1 second;
- continue waiting indefinitely for `GroupDead` and closed pipes;
- keep the run nonterminal and retain its command slot; and
- after the cleanup phase has lasted one minute, emit one stdout and `/logs`
  warning containing the run ID and PGID and stating that the wedged group is
  permanently holding one global command-concurrency slot. The warning payload
  derives total capacity from the same `maxActiveCommands` constant rather than
  hardcoding four in application text.

No terminal publication, slot release, or output verification rule changes.
The warning is one-shot; no periodic log spam is added. A dedicated warning
callback carries this event from `plugin_commands` to the application logger so
unrelated diagnostic `Logf` traffic is not silently promoted into durable audit
rows.

### 10. Allow one alive-observed re-signal

The first signal retains creation-time authority and does not depend on member
environment visibility. If that signal fails or a member joins the group during
the termination edge, the first cleanup deadline may leave the group alive.

At that deadline, the worker may issue exactly one final signal when the
immediately preceding inspection reported either `GroupAliveOwned` or
`GroupAliveUnverified`. A nonempty group retains its PGID; environment visibility
does not affect reuse. An inspection error supplies no alive observation and
therefore permits no re-signal. After the second attempt, cleanup only polls with
backoff.

The group may empty between observation and delivery, so the already accepted
narrow exit/reuse race remains. The attempt bound of two keeps that risk bounded
and does not restore signalling on every deadline.

## State and lifecycle summary

| Situation | Durable row | Command surfaces | Staging lease | Server |
|---|---|---|---|---|
| Different known boot | `interrupted`/latched `cancelled`, output unverified | Published after recovery | Held normally | Boots |
| Same/unknown boot, group dead | `interrupted`/`cancelled` | Published after recovery | Held normally | Boots |
| Live/uninspectable group | Remains nonterminal | Quarantined, auto-retrying; probe-only Unix may require reap/restart | Retained until heal/exit | Boots |
| Owned group ignores termination | Remains nonterminal | Quarantined, auto-retrying | Retained until heal/exit | Boots |
| Runtime lease busy | Unchanged | Quarantined, auto-retrying | Held by old runtime | Boots |
| Initial DB/filesystem/config/non-busy lease error | Unchanged where transactionality requires | Withheld | Released by failed startup | Startup fails |
| Retry-time infrastructure error after quarantine | Unchanged where transactionality requires | Quarantined, auto-retrying | Retained if acquired | Keeps serving |
| `-plugins-disabled` | Unchanged | Withheld with every plugin | Not acquired | Boots |

## Testing

### Boot identity and visibility

- Linux trims and returns `boot_id`; Darwin returns
  `kern.bootsessionuuid`, never `kern.boottime`.
- A different nonempty session UUID settles a running row as interrupted (or
  latched-cancelled) and output-unverified without any inspector or signal call.
- Matching UUIDs always take process inspection: the explicitly unsafe direction
  is pinned so same-boot work can never be classified as prior-boot work.
- Empty legacy/current identities take process inspection.
- PGID and boot identity are persisted by one transition; queued rows have
  neither.
- Lua run views, JSON/admin history, and rendered history contain no boot ID.
- Linux classifies an all-zombie target group dead without consulting signal 0,
  including under a PID-1 parent that does not reap adopted descendants.
- A hidden/unknown target candidate stays alive-unverified; unrelated stat races
  do not fail the whole scan.
- Darwin's all-`SZOMB` group remains dead even when `kill(-pgid, 0)` would return
  success or `EPERM`.

### Quarantine and healing

- Lease contention makes startup succeed with command/filesystem calls refused,
  retries on the 1s→2s→5s capped schedule, then activates automatically after
  the old lease is released.
- `errors.Is(AcquireRuntimeLease(...), ErrRuntimeLeaseBusy)` survives the public
  wrapper; non-contention lease errors remain fatal and name
  `-plugins-disabled`.
- A live unverified recovery group makes startup succeed, leaves host calls
  unavailable, retains the lease, and later activates automatically after the
  group dies.
- A verified group that misses the recovery deadline takes the same path.
- Recovery settles safe rows after an earlier blocker and reports the complete
  blocker set.
- A healing scan never re-signals a recovered group already attempted by this
  process.
- Database, filesystem, malformed-record, and context failures remain fatal on
  the initial synchronous path; retry-time failures remain quarantined.
- Stop races with acquisition, recovery, and activation cannot publish after
  stop or release a lease before dispatcher quiescence.
- Quarantine, retry failure, and healing records appear in `/logs` with deduped
  warnings, the process-group repair action, and actionable wording.
- Admin history disables queued cancellation while quarantined; the API returns
  the quarantine sentinel as HTTP 503 and never writes the cancellation latch.

### Live worker hardening

- Inspection frequency is 20 ms before the first cleanup deadline and no faster
  than the 1-second backoff afterward.
- A one-minute stuck group emits exactly one warning naming run, PGID, and the
  occupied global command slot; reported total capacity comes from
  `maxActiveCommands`.
- Either alive identity state permits at most one re-signal; an inspection error
  permits none; total signalling attempts never exceed two.
- An Apple-platform executable with unreadable environment receives the bounded
  second signal when it remains alive.
- Terminal publication still requires parent exit, closed pipes, and observed
  group death.

### Verification

Run focused race tests repeatedly, the affected SQLite and PostgreSQL command
suites, platform cross-compiles, command-history browser tests, and the existing
full verification matrix. Preserve the two accepted unrelated baseline failures
without masking any new one.

## Documentation changes

After implementation, amend the main plugin-command design, remediation plan,
`CLAUDE.md`, operator documentation, and `docs/todo.md` to record:

- only host-unique boot-session UUIDs permit the prior-boot shortcut;
- boot identity is paired atomically with PGID and never exposed;
- lease contention uses short capped backoff, while recovery blockage retries on
  the five-minute cadence;
- Linux/Darwin zombie-only groups are dead; probe-only Unix healing is limited;
- warnings are visible at `/logs`, with the named process-group kill as the
  immediate recovery action and `-plugins-disabled` as the restart escape hatch;
- administrator cancellation is disabled and returns an accurate quarantine
  error while this process cannot enforce it; and
- stuck live cleanup uses bounded alive-observed re-signalling, backed-off
  polling, and a one-shot capacity warning.
