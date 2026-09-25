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
| `plugin-action@1` | Run an asynchronous plugin action, a scheduled occurrence, or a `mah.start_job` closure | Owner-visible; process-local closures are not blindly re-run after restart; an unsuccessful declared action offers Retry, and a successful one whose handler reported `continue = true` offers Continue |
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

One phase has a host-wide meaning. A succeeded Job with phase `partial` stopped
short of finished: its Kind recorded that the run did its share and left the
rest. A plugin action records it when its handler returns `continue = true`.
The Job Center labels such a Job **Partially completed**, and the state filter
accepts `partial` as one more alternative (`state=failed,partial` lists failed
Jobs and partial ones). `partial` is a subset of `succeeded`: a filter for
`succeeded` still includes these Jobs, because that is their stored state.

When a plugin action succeeds, its final progress is stored with the completed
Job. The Job Center also shows older successful plugin actions as complete when
their last stored percentage update was below 100%.

An owner may inspect their Jobs, subject to the Kind's visibility rule. Admin
visibility and resource scope are checked on every read. Ownership grants
visibility, not permanent authority: each command and output access rechecks
current role, scope, plugin permission, and Kind policy. A filter for another
owner or actor never grants access to that person's Jobs.

## Progress, metrics and graphs

A running Job reports a progress snapshot: a phase, a message, an amount
completed, a total, a unit and an optional estimated finish. Progress is a
snapshot, not a Job Event. It never appears on the timeline and never moves the
Job's version.

A snapshot may also carry up to 8 **metrics**: named figures reported beside the
primary measure, such as the segments of an HLS stream next to its bytes. Each
metric has a key, a label, a value, an optional total and an optional unit. Up
to 3 metrics per snapshot can be marked for graphing.

Each Job keeps a bounded **progress series** on its row, so a graph survives a
reload, a restart and the Job finishing:

- A point is recorded at most once per interval. The interval starts at one
  second.
- Each point holds the completed amount, the rate since the previous point, and
  the value of every graphed metric.
- When the series reaches 120 points, adjacent points are merged and the
  interval doubles. The first point is kept, so the series always starts where
  the Job did and covers its whole life.
- A rate is only recorded between comparable points: the same unit, an amount
  that did not go down, and a gap of at most 10 seconds or three intervals,
  whichever is longer. A pause or a restart therefore shows as a gap, not as a
  slow stretch.

The server derives two figures from the series:

- **Speed**: the current rate, measured about once a second and smoothed. It is
  reported only while the Job is running and only while progress keeps arriving;
  after 10 seconds without a tick it is no longer reported.
- **Time left**: the executor's own estimate when it gave one. Otherwise it is
  estimated from the speed and the remaining amount, and marked as an estimate.

A finished Job reports its **average rate** across the series instead of a
speed. No speed is shown for a Job counting in `percent`.

Who reports what:

| Work | Primary measure | Metrics |
|------|-----------------|---------|
| Download with a known size | Bytes of the total | HLS segments, when the stream has them |
| Download of unknown size | Bytes received, no total | None |
| HLS stream | Segments of the total | Bytes received |
| Group export | Bytes written | Items of the current phase |
| Plugin action, schedule or `mah.start_job` | Percent, or the plugin's own counts | Whatever the plugin reports |
| Plugin command | Whatever the command prints | Whatever the command prints |

Plugins report metrics with the table form of `mah.job_progress`; see
[Counts, metrics and graphs](./plugin-actions.md#counts-metrics-and-graphs).
Commands print `::mah-progress` lines; see
[Report progress from a command](./plugin-lua-api.md#report-progress-from-a-command).

### The Jobs drawer

The **Jobs** button in the header, or Control/Command + Shift + D, opens the
Jobs drawer on the right. It groups Jobs into **Needs attention**, **Active and
scheduled** and **Finished**. A running Job shows its progress bar, the amount
completed, its speed, the time left, its metrics and a graph for its speed and
for each graphed metric. A finished Job shows its average speed.

Work that is running, waiting or needs attention is listed up to 50 Jobs per
group. Finished Jobs are limited by the `download_cockpit_limit` setting
(default 10); older ones stay on the All jobs page. Progress updates arrive over
the live stream. They are not announced to screen readers; state changes are.

The Job's own page shows the same figures with larger graphs. The `/jobs` list
shows the speed and time left under each running Job's bar.

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
advertised command, and the viewer's pin and dismissal preferences.

A lineage link has two ends, and each has a filter. `relationship` matches the
Job the link starts from: a Retry, Continue or Repeat successor, or a parent
stage. `inboundRelationship` matches the Job it points at: one that was retried
or continued (`retry-of`), repeated (`repeat-of`), or a child stage
(`parent-child`). `noInboundRelationship` is its negation, so
`state=failed&noInboundRelationship=retry-of` lists failed Jobs nobody has
retried. Only Jobs the viewer can see count as the other end, so a Job whose
only retry is hidden from the viewer reads as not retried. On the Job Center
page these are the **Has been** and **Has not been** selects. `get`
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
summary, except `state=partial`, `inboundRelationship` and
`noInboundRelationship`, which an export refuses with a 400. An export's filter
is stored and run later, possibly by a worker from an older release. Such a
worker fails an export filtered by `state=partial`, which it reads as an unknown
state, but it silently ignores the inbound relationship filters and exports a
wider summary than was asked for. Its CSV or JSON is a typed artifact, not a replacement for the Job
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
| `GET` | `/v1/jobs/events?version=2` | Canonical resumable Job SSE, with live `job-progress` frames |
| `POST` | `/v1/jobs/{id}/commands/{command}` | Recheck and run one advertised command |
| `POST` | `/v1/jobs/commands/{command}` | Run one advertised bulk command, returning per-Job outcomes |
| `GET` | `/v1/jobs/summary` | Visible aggregate with a window up to 90 days |
| `POST` | `/v1/jobs/summary/export` | Queue a CSV or JSON export for an explicit range over 90 days |

A succeeded download publishes the Resource it created as its `resource` entity
output. Plugin actions that return a local Resource, Note, or Group redirect
publish an entity output too. Opening an entity output rechecks access before
navigating to the entity. The Jobs panel, the Job Center list, and the Job
detail page link a succeeded Job's available entity output. For plugin actions
they also show a direct “View result” link for older summary outputs that
stored the same safe redirect before entity outputs were published. Job detail
still offers “View JSON result” for the stored summary.

Every Job's `progress` object carries `metrics`, `rate` (running Jobs only),
`averageRate`, `eta` with `etaEstimated`, and `updatedAt`. The progress series
is included as `progress.series` on `GET /v1/jobs/{id}`, and on `GET /v1/jobs`
only with `include=progressSeries`. Series points use short names: `t` is Unix
milliseconds, `c` the completed amount, `r` the rate per second since the
previous point, and `v` the graphed metrics by key.

Once the canonical stream has sent `job-caught-up`, each poll also sends a
`job-progress` event for every visible Job whose progress changed in the last
30 seconds and whose current snapshot this connection has not sent yet. Its
data is `{jobId, version, state, progress, point, intervalMs}`, where `point` is
the latest series point. Like `job-caught-up`, it has no SSE `id` and never
moves the delivery cursor. A new connection can therefore receive frames for
changes an earlier connection already delivered. Each frame replaces the Job's
progress, so a reader treats a repeat as a no-op: it ignores a frame whose
`progress.updatedAt` is older than the progress it holds, and replaces rather
than appends a point whose `t` equals its last point's. Progress timestamps are
written by whichever process runs the Job, and the 30-second window is what
absorbs clock skew between those processes.

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
