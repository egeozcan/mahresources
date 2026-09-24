---
sidebar_position: 14
title: Job System
---

# Job System

A Job is the durable record of accepted background work. It stores its owner and
actor, Kind and input version, state, events, outputs, lineage, and the controls
currently permitted for the requesting account. Job IDs are canonical UUIDs;
retrying or repeating work creates a new Job linked to its source instead of
rewriting the source outcome.

The Job Service is installed before plugin activation and is the lifecycle
authority for work that publishes through an adapter. Download queues and
specialized exporters/importers still execute the work. The Job Service records
and fences acceptance, execution, recovery, visibility, commands, and retention.

## Release status and compatibility

The Job Center is available at `/jobs`. Its canonical APIs, panel, and CLI are
enabled together. Startup verifies the complete Kind inventory and the
plaintext-retirement barrier before serving traffic. Older queue, download,
import, export, and plugin routes remain available for compatibility. An older
server without the canonical list route still works with an unfiltered
`mr jobs list` through its legacy queue fallback.

Legacy Job routes and handles remain supported for at least one documented
release and six months after canonical cutover. A legacy download handle follows
its latest Retry leaf during that window; a canonical UUID continues to identify
one immutable Job. See [Download Queue](./download-queue.md) for the older
download routes and [Backup and Restore](../deployment/backups.md) for the
restore barrier.

## Job Kinds

Each Kind owns its replay input, execution and recovery rules, presentation, and
available commands. Current adapters include:

| Kind | Work | Recovery and visibility |
|------|------|------------------------|
| `remote-download@1` | Fetch one remote URL into a Resource | Replayable; owner-visible |
| `deferred-download@1` | A remote download accepted for a future time | Replayable; owner-visible |
| `group-export@1` | Build a group archive | Replayable; owner-visible; artifact retention is separate from Job history |
| `group-import-parse@1` | Parse an uploaded archive into a review plan | Replayable from durable staged input; owner-visible |
| `group-import-apply@1` | Apply a reviewed plan | Replayable only when import evidence proves it safe; owner-visible |
| `resource-reduction-compute@1` | Compute clusters for a Resource Reduction | Replayable; owner-visible |
| `similarity-recompute@1` | Recompute image similarity data | Replayable; administrator-visible |
| `plugin-action@1` | Run an asynchronous plugin action or `mah.start_job` closure | Owner-visible; process-local closures are not blindly re-run after restart |
| `job-summary-export@1` | Export a filtered Job summary as CSV or JSON | Replayable; owner-visible; artifact expires by export retention |
| `plugin-command@1` | Run a plugin command | Non-restorable; administrator-visible; protected by the command runtime fence |
| `plugin-command-import@1` | Import an admitted plugin command output | Non-restorable; administrator-visible; retry requires current importer and file proof |

Plugin command runs and imports use their separate fenced command runtime.

## State and visibility

Canonical state is one of `scheduled`, `queued`, `running`, `paused`, `blocked`,
`succeeded`, `failed`, `cancelled`, or `interrupted`. A Kind may publish a finer
phase such as parsing, downloading, or assembling without changing the Job
state. The old queue endpoints may continue to use their established status
names during compatibility.

An owner may inspect their Jobs, subject to the Kind's visibility rule. Admin
visibility and resource scope are checked on every read. Ownership grants
visibility, not permanent authority: each command and output access rechecks
current role, scope, plugin permission, and Kind policy. A filter for another
owner or actor never grants access to that person's Jobs.

## CLI

The plural `mr jobs` command is the canonical browsing and analytics surface:

```bash
mr jobs list --state failed --kind remote-download --limit 50
mr jobs get 018f4db1-9b40-7f54-8f16-37a449bcf01d --json
mr jobs timeline 018f4db1-9b40-7f54-8f16-37a449bcf01d --after-sequence 20
mr jobs summary --window 30d --json
```

`jobs list` returns a bounded page with an opaque `nextCursor`. Filters include
state, Kind, origin, owner, actor, accepted time, lineage relationship, text,
advertised command, and the viewer's pin and dismissal preferences. `get`
returns the current command and output declarations. `timeline` reads ordered
durable events by per-Job sequence. `summary` uses the same visibility and
filters as listing and accepts windows up to 90 days.

The CLI does not infer command eligibility from Kind or state. `mr job command`
reads detail, requires the server to advertise the key, checks the advertised
Job version and endpoint, and sends an idempotency key. Destructive commands or
commands with an advertised confirmation require `--confirm`. `mr job
bulk-command` checks that every selected Job advertises the same bulk-capable
key at its current version, then reports the server's per-Job results. Pass
`--idempotency-key` to retry the same request after a network failure; the CLI
generates and prints a key if none is supplied.

The singular `mr job submit`, `cancel`, `pause`, `resume`, and `retry` commands
remain compatibility aliases for existing download scripts. `mr jobs queue`
returns the legacy queue response explicitly.

## Summary analytics and exports

Interactive `summary` is capped at 90 days. For an explicit range longer than
90 days, queue an owner-visible export:

```bash
mr jobs summary export \
  --from 2025-01-01T00:00:00Z \
  --to 2026-01-01T00:00:00Z \
  --kind remote-download \
  --format csv
```

The export Job applies the same visibility predicate and filters as interactive
summary. Its CSV or JSON is a typed artifact, not a replacement for the Job
record; export retention controls when the bytes expire. A Job's history,
encrypted replay envelope, and output artifact have separate retention policies.

## Canonical API

These routes are available together. The generated public OpenAPI contract
includes them.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/jobs` | Filtered, cursor-paginated visible Jobs |
| `GET` | `/v1/jobs/{id}` | Job detail, current commands, outputs, and lineage |
| `GET` | `/v1/jobs/{id}/events` | Ordered timeline; `afterSequence` resumes a page |
| `GET` | `/v1/jobs/{id}/outputs?key={key}` | Reauthorize and open a typed output |
| `GET` | `/v1/jobs/events?version=2` | Canonical resumable Job SSE |
| `POST` | `/v1/jobs/{id}/commands/{command}` | Recheck and run one advertised command |
| `POST` | `/v1/jobs/commands/{command}` | Run one advertised bulk command, returning per-Job outcomes |
| `GET` | `/v1/jobs/summary` | Visible aggregate with a window up to 90 days |
| `POST` | `/v1/jobs/summary/export` | Queue a CSV or JSON export for an explicit range over 90 days |

Command requests carry `expectedVersion`, `idempotencyKey`, and `origin`. The
server recomputes the command under current authorization and rejects a stale
version. Bulk requests accept at most 200 Job IDs; each result commits
independently, so a response can contain both successes and refusals.

## Replay keys and writer epoch

`JOB_REPLAY_KEY` seals accepted inputs so an authorized Retry can replay the
same request. Keep its active value stable across every PostgreSQL process.
Rotation puts the new key first and keeps old decrypt-only keys until every
envelope they sealed has expired or been explicitly forgotten. Persistent
SQLite can use the generated `_job_replay_key` file, which is created with mode
`0600`; include it with the database in backups. See [Advanced Configuration](../configuration/advanced.md#job-replay-key).

The database's Job writer epoch is checked before application migrations or
dispatch. Deploy the fence-aware binary to every writer and drain old processes
before an epoch is advanced. After advancement, an older binary must refuse to
start; rollback uses a compatible canonical reader, never a plaintext writer.
Restoring a backup from before plaintext retirement requires rerunning the
retirement verification barrier before serving traffic.

Plugin command runs and command imports require one fenced runtime owner per
database and staging namespace. Other Job Kinds and the Job Service can run in
multiple processes. Do not point independent plugin-command runtimes at one
database with separate staging roots.

## Retention

| Data | Default | Rule |
|------|---------|------|
| Succeeded and cancelled Job history | 30 days | From terminal completion |
| Failed and interrupted Job history | 90 days | From terminal completion; unresolved work is retained |
| Replay envelope after terminal completion | 7 days | Nonterminal execution-required input is retained |
| Job summary export artifact | `EXPORT_RETENTION` (24 hours) | Independent from Job history |

The list, detail page, and Jobs panel mark Jobs pinned by the current viewer.
Unpin removes that viewer's preference. While anyone keeps a Job pinned, its
metadata and events are exempt from ordinary expiry; dismissal only changes a
person's view. Output artifacts are reauthorized when opened; holding a visible
Job does not grant an output access token.

## Related pages

- [Download Queue](./download-queue.md)
- [Group Export / Import](./export-import.md)
- [Advanced Configuration](../configuration/advanced.md)
- [Backup and Restore](../deployment/backups.md)
