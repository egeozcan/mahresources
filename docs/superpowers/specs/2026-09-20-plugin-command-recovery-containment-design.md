# Plugin command recovery containment design

**Date:** 2026-09-20  
**Status:** Approved in chat; awaiting written-spec review  
**Scope:** Follow-up to `2026-09-19-plugin-commands-design.md` and `2026-09-20-plugin-command-runtime-remediation.md`

## Problem

Plugin-command recovery currently treats a safety refusal as a process-startup
failure. A persisted PGID that is alive but cannot be tied to its run makes
`Dispatcher.Recover` return an error; `main` then exits before serving HTTP. The
same happens when a verified owned group ignores termination for ten seconds.
The safety invariant is correct — the row must remain nonterminal and command
surfaces must stay unpublished — but disabling the entire application is not
required to preserve it.

PGIDs are also scoped to one host boot. After a reboot, a persisted integer may
name an unrelated process group even though the original command cannot still be
running. Recovery should distinguish that case before inspecting or signalling
the number.

Finally, a live worker whose group never becomes observably dead polls the whole
process table every 20 ms forever. Its refusal to publish terminal state is
correct, but the steady-state inspection cost and lack of operator diagnostics
are not.

## Goals

1. A command-recovery safety blockage disables only the plugin-command runtime,
   not HTTP, downloads, resources, or unrelated plugin surfaces.
2. The staging-root lease remains held while command recovery is quarantined.
3. A PGID persisted on a different known host boot is never inspected or
   signalled.
4. Infrastructure and consistency errors remain fatal to application startup.
5. A stuck live worker remains nonterminal but backs polling off and identifies
   the run pinning capacity.
6. A live worker may make one bounded, ownership-verified re-signal after its
   creation-authority signal did not drain the group.

## Non-goals

- Publishing partial `mah.commands` or `mah.fs` functionality while recovery is
  blocked.
- Automatically discarding, importing, or marking terminal output that may
  still have a writer.
- Building a cross-host process supervisor or distributed staging lease.
- Adding an administrator repair UI in this follow-up.
- Changing the documented approximate global staging quota.

## Design

### 1. Persist a host-boot identity

`PluginCommandRun` and `RunRecord` gain a nullable/string `BootID`. The command
dispatcher captures its process's boot identity when it durably admits a run.
Queued rows created by older releases have an empty value and keep the existing
recovery path.

The identity provider is platform-specific:

- Linux reads and trims `/proc/sys/kernel/random/boot_id`.
- Darwin and the supported BSD targets encode `kern.boottime` from
  `unix.SysctlTimeval`.
- AIX, Solaris, or a failed platform lookup report identity unavailable.

Identity lookup is an optimization of a fail-closed recovery path, not a new
startup dependency. An unavailable identity is logged once and stored as empty;
recovery then uses the existing environment-based ownership inspection.

The runtime resolves the current identity once and supplies the same immutable
value to admission and recovery. It is not read per run or per poll.

### 2. Classify different-boot rows without touching their PGID

For a running row with a positive PGID:

1. If both the recorded and current boot identities are nonempty and differ,
   recovery does not call `InspectGroup` or `KillGroup`.
2. The row becomes `interrupted` (or `cancelled` when its durable cancellation
   latch already won) with `output_unverified=true` and a reason that records the
   different-boot classification.
3. If either identity is unavailable, or they match, recovery follows the
   existing ownership inspection.

A different boot proves the original local process group cannot still be a
writer. It does not prove output completeness, so output remains unverified and
only discard is allowed.

### 3. Represent safety blockage separately from recovery failure

`plugin_commands` adds a typed `RecoveryBlockedError` containing the run ID,
PGID, and reason. `errors.As` must distinguish it from database, filesystem,
configuration, malformed-record, migration, and context errors.

The typed blockage is returned whenever recovery cannot safely settle a running
row because a group may still exist:

- ownership inspection fails;
- the group is alive but unverified;
- ownership changes before termination;
- signalling an owned group fails;
- post-signal inspection fails; or
- the owned group remains alive past `groupDrainTimeout`.

A cancelled recovery context remains an ordinary context error. Unknown enum
states and malformed durable values remain fatal consistency errors.

The blocked row is not passed to `FinishRun`; later rows are not recovered; no
command or exchange surface is published.

### 4. Quarantine the command runtime, retain its lease, boot the server

`application_context.StartPluginCommands` handles only
`RecoveryBlockedError` specially:

- retain `RuntimeLease` in `MahresourcesContext` for the process lifetime;
- do not retain or start the dispatcher;
- do not construct/publish the exchange mediator;
- do not call `SetCommandSubmitter` or `SetExchangeMediator`;
- log one prominent warning containing run ID, PGID, reason, and the operator
  escape hatch `-plugins-disabled`; and
- return success so `main` continues normal startup.

The retained lease prevents this process or a rolling peer from entering the
same staging root while the suspected writer exists. `StopPluginCommands`
already handles a nil dispatcher plus a live lease and closes the lease during
normal shutdown.

Every non-blockage error keeps current behavior and remains fatal. This prevents
an unavailable database, corrupt identifier, failed temp cleanup, migration
problem, or invalid configuration from being mislabeled as safe degradation.

With `-plugins-disabled`, `StartPluginCommandsIfEnabled` continues to bypass
recovery and lease acquisition entirely. All plugins are disabled, but the
operator can bring up the rest of the application to inspect or repair durable
state.

### 5. Back off live cleanup polling and log the pinned run

Ordinary execution remains free of group polling. Polling begins only after the
direct parent exits or termination starts.

After the first cleanup deadline expires while the group is still alive:

- reset the group ticker from 20 ms to 1 second;
- continue waiting indefinitely for `GroupDead` and closed pipes;
- keep the run nonterminal and retain its command slot; and
- after the cleanup phase has lasted one minute, log one warning containing the
  run ID and PGID.

No terminal publication, slot release, or output verification rule changes.
The warning is one-shot; no periodic log spam is added.

### 6. Allow one ownership-verified re-signal

The first signal retains creation-time authority and does not depend on member
environment visibility. If that signal fails or a member joins the group during
the termination edge, the first cleanup deadline may leave the group alive.

At that deadline, the worker may issue exactly one final signal only when the
immediately preceding inspection reports `GroupAliveOwned`. `GroupAliveUnverified`
and inspection errors are never re-signalled. After the second attempt, cleanup
only polls with backoff.

This changes the attempt bound from one to two while keeping the accepted narrow
inspect/signal exit-reuse race bounded. It does not restore repeated signalling
on every deadline.

## State and lifecycle summary

| Situation | Durable row | Command surfaces | Staging lease | Server |
|---|---|---|---|---|
| Different known boot | `interrupted`/latched `cancelled`, output unverified | Published after recovery | Held normally | Boots |
| Same/unknown boot, group dead | `interrupted`/`cancelled` | Published after recovery | Held normally | Boots |
| Live unverified group | Remains nonterminal | Withheld | Retained until exit | Boots |
| Owned group ignores termination | Remains nonterminal | Withheld | Retained until exit | Boots |
| DB/filesystem/config error | Unchanged where transactionality requires | Withheld | Released by failed startup | Startup fails |
| `-plugins-disabled` | Unchanged | Withheld with every plugin | Not acquired | Boots |

## Testing

### Boot identity

- A different nonempty boot identity settles a running row as interrupted (or
  latched-cancelled) and output-unverified without any inspector or signal call.
- Matching identities take the current verification/termination path.
- Empty legacy/current identities take the current path.
- Linux parsing trims the boot ID; Darwin/BSD identity is stable and
  cross-compiles on every supported build-tag path.

### Quarantine boundary

- A live unverified recovery group makes `StartPluginCommands` return success,
  leaves command submission and exchange access unavailable, and retains the
  staging lease.
- A second runtime cannot acquire that staging root while quarantined.
- `StopPluginCommands` releases a quarantined lease.
- A verified group that misses the recovery deadline takes the same quarantine
  path.
- Database, filesystem, malformed-record, and context failures remain errors.
- The warning names run ID, PGID, reason, and `-plugins-disabled`.

### Live worker hardening

- Inspection frequency is 20 ms before the first cleanup deadline and no faster
  than the 1-second backoff afterward.
- A one-minute stuck group emits exactly one warning naming run and PGID.
- A currently owned group gets at most one re-signal; an unverified group gets
  none; total signalling attempts never exceed two.
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

- boot identity is advisory hardening with a fail-closed fallback;
- recovery blockage quarantines only command surfaces while retaining the
  staging lease;
- `-plugins-disabled` is the operator escape hatch; and
- stuck live cleanup uses bounded re-signalling, backed-off polling, and a
  one-shot warning.
