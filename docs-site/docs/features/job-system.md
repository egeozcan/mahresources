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
restore barrier. After upgrading, check for unfinished Jobs of accounts deleted
before this release, which carry no deleted-account mark; see
[Advanced Configuration](../configuration/advanced.md#unfinished-jobs-of-accounts-deleted-before-this-release).

## Job Kinds

Each Kind owns its replay input, execution and recovery rules, presentation, and
available commands. Current adapters include:

| Kind | Work | Recovery and visibility |
|------|------|------------------------|
| `remote-download@1` | Fetch one remote URL into a Resource | Replayable; owner-visible; no Retry for a stored address that is not an `http` or `https` URL (see [Download Queue](./download-queue.md#failure-reasons-and-retry)) |
| `deferred-download@1` | A remote download accepted for a future time | Replayable; owner-visible; Retry of one that never started is offered as **Download now** and starts at once, without its scheduled time |
| `group-export@1` | Build a group archive | Replayable; owner-visible; artifact retention is separate from Job history |
| `group-import-parse@1` | Parse an uploaded archive into a review plan | Replayable from durable staged input; owner-visible; no Retry for a file the reader refused as not an archive |
| `group-import-apply@1` | Apply a reviewed plan | Replayable only when import evidence proves it safe; owner-visible |
| `resource-reduction-compute@1` | Compute clusters for a Resource Reduction | Replayable; owner-visible |
| `similarity-recompute@1` | Recompute image similarity data | Replayable; administrator-visible |
| `plugin-action@1` | Run an asynchronous plugin action, a scheduled occurrence, or a `mah.start_job` closure | Owner-visible; process-local closures are not blindly re-run after restart; a Job that has not started offers Cancel, and a running one when its registration declares `cancel = true`; an unsuccessful declared action offers Retry, and a successful one whose handler reported `continue = true` offers Continue |
| `job-summary-export@1` | Export a filtered Job summary as CSV or JSON | Replayable; owner-visible; artifact expires by export retention |
| `plugin-command@1` | Run a plugin command | Non-restorable; administrator-visible; protected by the command runtime fence; a failed Job names the run's recorded reason (its exit status, its timeout, a quota) with a matching failure class |
| `plugin-command-import@1` | Import an admitted plugin command output | Non-restorable; administrator-visible; retry requires current importer and file proof |

Plugin command runs and imports use their separate fenced command runtime.
A command Job that started keeps a "Command history and output" output whatever
its outcome, and its **Inspect command history** command opens the run's page at
`/admin/plugin-command-runs?id={runId}` (for an import, the run that produced
the file), which shows the exit code, the terminal reason and the redacted
program output and links back to the Job. A command's outcome names such a page
as `detail.location`, a path on the same site; the Job Center opens it after the
command succeeds and ignores any other value.

Every Kind's running Jobs count against one deployment budget,
`-max-job-concurrency`. A Job whose turn comes while the budget is full waits
`queued` for a slot rather than failing. The exception is a scheduled
occurrence, which gives up after 10 seconds without a slot and records no Job; see
[Timing you should not rely on](./plugin-lua-api.md#timing-you-should-not-rely-on).
Plugin work also waits
for its plugin: in each server process a plugin runs one of its async actions,
`mah.start_job` jobs and schedule runs at a time, so a plugin's backlog is `queued`
Jobs behind one running Job and occupies one slot of the budget per process.

## State and visibility

Canonical state is one of `scheduled`, `queued`, `running`, `paused`, `blocked`,
`succeeded`, `failed`, `cancelled`, or `interrupted`. A Kind may publish a finer
phase such as parsing, downloading, or assembling without changing the Job
state. The old queue endpoints may continue to use their established status
names during compatibility.

`paused` is a hold a person asked for and the Job's executor confirmed. A
download is the Kind that can be paused: while it runs it offers Pause, and
its Resume starts the transfer again from the beginning, because the download
queue keeps no partial bytes; the paused row says so. A paused Job counts as
Active. `blocked` is work that cannot go on until a person or a policy change
lets it, such as a refusal of the account it runs as or a claim nobody could
prove stopped, and it Needs attention. Every surface names a state the same
way: the Jobs panel, the Job Center card and the Job page read one table of
labels, groups and colours, and only running work shows a moving progress bar.
A running Job with a pause or a cancellation requested and not yet confirmed
by the process running it reads **Pausing** or **Cancelling** (the API's
`controlIntent`).

One phase has a host-wide meaning. A succeeded Job with phase `partial` stopped
short of finished: its Kind recorded that the run did its share and left the
rest. A plugin action records it when its handler returns `continue = true`.
The Job Center labels such a Job **Partially completed**, and the state filter
accepts `partial` as one more alternative (`state=failed,partial` lists failed
Jobs and partial ones). `partial` is a subset of `succeeded`: a filter for
`succeeded` still includes these Jobs, because that is their stored state.

A failed Job records why, as a failure code, a class and a message. An
`interrupted` Job may record one too, and the Jobs panel and the Job Center show
it the same way: a plugin action stopped by a shutdown or by its plugin being
disabled says so, and a Job whose server process stopped says "The server process
running this stopped before it finished." (code `runtime-lost`). The summary's
failures by class count both states; see [Summary analytics and exports](#summary-analytics-and-exports).

A Job's claim is kept alive by the process running it. When that process stops
without a graceful shutdown, a process of the same boot session on the same
machine proves it gone (its recorded process no longer exists) and reconciles its
Jobs on its next pass rather than once their 2-minute lease runs out. A claim
recorded in another boot session cannot be proved gone that way, because a
hostname does not identify one machine, so its Jobs wait for their lease.

When a plugin action succeeds, its final progress is stored with the completed
Job. The Job Center also shows older successful plugin actions as complete when
their last stored percentage update was below 100%.

An owner may inspect their Jobs, subject to the Kind's visibility rule. Admin
visibility and resource scope are checked on every read. Ownership grants
visibility, not permanent authority: each command and output access rechecks
current role, scope, plugin permission, and Kind policy. A filter for another
owner or actor never grants access to that person's Jobs. Every command is a
write, so a guest sees its own Jobs and is offered no command on them, including
Pin, Dismiss and Forget.

An administrator sees every account's Jobs and is told whose each one is: the
Job Center list and the Jobs drawer name the owner of a Job that is not the
administrator's own, and the Job page names its owner and actor. The API
returns them as `ownerName` and `actorName`; anyone else is told only their own
name. On the Job Center page an administrator filters by **Owner** and **Actor**
from a list of accounts instead of typing a user number. The Owner list also
offers **Mine**, which is `owner=me` in the page's address, so a link to
`/jobs?owner=me` opens with it chosen and changing another filter keeps it.

Deleting an account removes its id from its Jobs, and marks them instead: they
read **Deleted account** as owner or actor (`ownerDeleted` and `actorDeleted`
in the API), and `ownerDeleted=true`, **A deleted account** in the Job Center's
Owner filter, lists them. Work that was still waiting to act as the deleted account never
runs as anyone else: when its turn comes it ends failed with the code
`principal-missing`, and a Job that an earlier release blocked for the same
reason is not offered Resume. A download, export, import, Job summary export,
clustering run or similarity recompute whose account is deleted after its turn
came, before it checks the account, ends failed the same way. A Job of another
Kind claimed or accepted in the same moment as the deletion can instead end up
blocked as `role-refused`; it still never runs.
Work already running when the account was deleted may still finish. The delete confirmation on `/admin/users` says how
many of the account's jobs have not finished.

The Job event streams apply the same rule for as long as they stay open. The
canonical stream checks the connection's session or API token again on every
poll, about once a second. The legacy `/v1/jobs/events` and
`/v1/download/events` streams check it after reading what each frame will say,
the initial state included, and before sending it, and once a second while
idle; events arriving within a quarter second of the last check wait for the
next one and share it. When the credential no longer authenticates, after a logout,
a disabled account or a revoked token, the stream closes. After a role or scope
change it stays open and sends only what the account may see now.

## Progress, metrics and graphs

A running Job reports a progress snapshot: a phase, a message, an amount
completed, a total, a unit and an optional estimated finish. Progress is a
snapshot, not a Job Event. It never appears on the timeline and never moves the
Job's version.

A snapshot may also carry up to 8 **metrics**: named figures reported beside the
primary measure, such as the bytes an HLS stream received next to its segment
count. Each
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
- The point that closes a finished Job's series records no rate when nothing
  was counted since the point before it, so its speed graph ends at the last
  measured speed rather than dropping to zero.

The server derives two figures from the series:

- **Speed**: the current rate, measured about once a second and smoothed. It is
  reported only while the Job is running and only while progress keeps arriving;
  after 10 seconds without a tick it is no longer reported.
- **Time left**: the executor's own estimate when it gave one. Otherwise it is
  estimated from the speed and the remaining amount, and marked as an estimate.

A Job that is not running reports its **average rate** instead of a speed: the
amount it counted divided by the time it spent running. Time spent queued,
paused or blocked is left out. The count starts from zero, so work done before
the Job's first report is included, unless that first report already carried a
count (a Continue that picks up at 120 of 500 did not do those 120 itself). A
Job that counted nothing has no average. No speed is shown for a Job counting
in `percent`.

Every surface writes these figures the same way, on one line under the bar:
the amount, then the speed and the time left while the Job runs, or its average
once it has ended ("6.5 MB of 20.0 MB · 1.2 MB/s · about 12 s left"). An amount
at least a tenth of its total is written in the total's unit ("0.97 MB of 1.0
MB") and rounded down, so it never reads as the total before it is. A finished
Job whose amount reached its total gives the amount once ("558 B"). An
estimated time left under a second reads "almost done". The bar's own label
says what the Job is doing, and a succeeded Job's bar reads **Completed**.

Who reports what:

| Work | Primary measure | Metrics |
|------|-----------------|---------|
| Download with a known size | Bytes of the total | None |
| Download of unknown size | Bytes received, no total | None |
| HLS stream | Segments of the total, video and audio together, from the playlist to the end | Bytes received; the video's size once assembled |
| Group export | Bytes written, of the export's estimated size until the count passes it; once finished, the archive's size | Items of the current phase |
| Similarity recompute | Hash rows of the total | None |
| Plugin action, schedule or `mah.start_job` | Percent, or the plugin's own counts | Whatever the plugin reports |
| Plugin command | Whatever the command prints | Whatever the command prints |

An HLS stream counts segments through the assembly too, where every segment is
done, so its bar does not fall back to zero while the video is put together and
its history keeps one unit to the end.

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

Each group lists the Jobs that entered their current state most recently
first, so a Job that has just finished or failed is at the top of its group
however long ago it was accepted. Scheduled Jobs follow the rest of **Active and
scheduled** in the order they start, the soonest first. A blocked Job says why
in one line under its title; its page's timeline has the event that blocked it.
A row names the Job's Kind in words, such as **Download** or **Scheduled
download**; the API and the Job Center's filters keep the identifier
(`remote-download`). Work that is running, waiting or needs
attention is listed up to 50 Jobs per group, and Finished Jobs up to the
`download_cockpit_limit` setting (default 10). A group that has more says
"Showing the 50 most recent." with a link to the same Jobs on the All jobs page,
its heading reads "(50+)", and so does its badge on the **Jobs** button.

The drawer reads its lists when its live stream has caught up, and again after
each state change the stream reports. It reads a Job's commands and outputs
only while it is open, and reads them again only when the Job has changed or
you (or another of your tabs) pinned, unpinned or forgot it. Dismissing, pinning or forgetting a Job, from the drawer, a Job's page or the
All jobs page, reaches your other open tabs of the same browser, whose drawers
and All jobs pages read their lists again; other browsers see it at their next
change.

If a list cannot be read, the drawer keeps what it showed, says the Jobs could
not be refreshed, and tries again after 2 seconds, doubling up to a minute, at
once when you open the drawer or return to the tab, and when you press **Try
again**. Before any list has been read it shows no counts rather than zeroes. If
the server answers that your session has ended, the drawer says so and offers
**Sign in again**, which returns you to the same page. The browser reconnects a
dropped live stream by itself but gives up on one refused with an error, such
as a proxy's 502 during a restart or a 401 after signing out; the drawer then
reopens it after 1 second, doubling up to 30 seconds, and shows
**Reconnecting** meanwhile. The Job Center and a Job's page reopen their streams
the same way; the Job Center says when its list could not be refreshed, and a
Job's page whose read failed offers **Try again** and a link to All jobs.

Progress updates arrive over the live stream. They are not announced to screen readers; state changes are,
once each, with the reason when a Job fails. That includes a Job accepted and
finished within a moment of each other, such as a download refused with a 404:
its outcome is announced even though the drawer never showed it running. A Job
whose outcome the live stream had already published when the page connected,
or published while the stream was reconnecting, is shown but not announced. The
stream publishes about every two seconds, so an outcome from the moment before
a page connected can still be announced. The drawer keeps track of at most
1,000 Jobs whose outcome arrived before it could read them; past that, the rest
are announced together as a count ("12 jobs finished or need attention;
see the Jobs panel."). Outcomes that arrived but had not yet been announced when
the live stream dropped are announced the same way. A Job counted this way can still be
announced by name if the drawer reads it later.

The Job's own page shows the same figures with larger graphs. The `/jobs` list
shows the same line under each Job's bar.

**Needs attention** lists only failures nobody has retried or continued
(`noInboundRelationship=retry-of`). Once a Job is retried, the retry is the row
to watch, and a retry that fails is listed there in its own right. Known
limit: an account that cannot write, such as a guest, is not told of a retry it
cannot see, so a failure another account retried stays in its Needs attention.

An administrator's drawer offers **My jobs** and **Everyone's**. It starts on
**My jobs** (`owner=me`), so its badges count only the administrator's own work,
and remembers the choice for the account (the user setting `jobsPanelScope`).
Choices are stored in the order they are made, and the last one is carried to
the next page opened in the same tab, which applies it and stores it again.
Known limit: a write the page left behind still had in flight can land after
the next page's write, and is then the stored choice.
**Dismiss finished** dismisses what the chosen scope lists. Other accounts see
only their own Jobs and are offered no choice.

A command asks for confirmation only when it stops work or cannot be undone:
Cancel and any other command marked destructive, a command whose Kind gives a
confirmation (a deferred download's **Download now**), and **Forget retry
data**. Its confirming button is red only for a destructive command. Dismiss,
Undismiss, Pin, Unpin and **Pin with related jobs** run at once, because each only
changes the viewer's own list or retention and can be reversed. A dismissed
Job's page reads **Dismissed by you** and offers **Undismiss**, and a row
dismissed in the drawer leaves a notice with **Undo**. **Dismiss finished**
reaches every finished Job the viewer has not dismissed, not only the rows
shown, so it asks first and says how many.

The drawer never leaves the page it is open on, which may hold unsaved work.
**Retry** and **Continue** leave a notice naming the new Job with a link to it,
and a command whose answer names a page (**Inspect command history**) offers
that page as a link. The Job's own page opens both directly. A notice names its
Job; one for a request, such as "Cancel requested for ...", stays until the Job
leaves the state it was in when the command was sent, and closing the drawer
clears it. A request the executor has already carried out by the time the
answer or the read after it arrives is said as its result ("<Job> cancelled.")
instead. After every command, and after
a refusal of a Job that still exists, the Job's controls are read again, so a
control the Job no longer offers disappears. A refusal names the command and
the Job and says why: a Kind's refusal in its own words ("Retry refused for
<Job>: <reason>"), and the service's own in terms of the Job ("Cancel is no
longer offered for <Job>, which is now succeeded."). While a command runs, the
Job's controls take no second press.

When a row changes group while the keyboard is on it, or a command replaces the
control that had focus, focus stays on that row: the same control, its
counterpart (Pin and Unpin), or the row's title. Running a plugin action opens
the drawer with focus on the Job it started (for a run that started several, the
newest of them the drawer lists), or, when a full group leaves that Job out,
with a notice linking to it. On the Job's page and in the
`/jobs` bulk bar, focus moves to the command that replaced the one pressed, else
to the first command left; when a command empties the bulk selection, focus
moves to **Select All**.

## CLI

The plural `mr jobs` command is the canonical browsing and analytics surface:

```bash
mr jobs list --state failed --kind remote-download --limit 50
mr jobs get 018f4db1-9b40-7f54-8f16-37a449bcf01d --json
mr jobs timeline 018f4db1-9b40-7f54-8f16-37a449bcf01d --after-sequence 20
mr jobs summary --window 30d --json
```

`jobs list` returns a bounded page with an opaque `nextCursor`. On the API,
`order=stateEntered` lists Jobs by when each entered its current state
(`stateEnteredAt` on every Job), newest first, instead of by acceptance; a
cursor continues the order it was issued in. Filters include
state, Kind, origin, owner, actor, accepted time, lineage relationship, text,
advertised command, and the viewer's pin and dismissal preferences, which take
`true`, `false` or `any`. Without `--dismissed` the CLI and the API list
dismissed Jobs too; the `/jobs` page instead writes its default,
`dismissed=false`, into its address. On the API, `owner=me` lists the asking
account's own Jobs without naming its id, and `ownerDeleted=true` lists Jobs
whose owner's account was deleted.

A lineage link has two ends, and each has a filter. `relationship` matches the
Job the link starts from: a Retry, Continue or Repeat successor, or a parent
stage. `inboundRelationship` matches the Job it points at: one that was retried
or continued (`retry-of`), repeated (`repeat-of`), or a child stage
(`parent-child`). `noInboundRelationship` is its negation, so
`state=failed&noInboundRelationship=retry-of` lists failed Jobs nobody has
retried. Only Jobs the viewer can see count as the other end, with one
exception: a Retry or Continue made by another account, such as an
administrator retrying your Job, still counts. Retry lineage is linear whoever
extends it, so such a Job no longer offers you Retry, reads as retried in these
filters, and its detail page says that another account retried it. The retry
itself stays hidden from you. The exception does not apply to an account that
cannot write, such as a guest, which is offered no Retry on any Job: to it, such
a Job reads as not retried. On the Job Center page these filters are the
**Has been** and **Has not been** selects. `get`
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
generates a key if none is supplied and prints it with the result, or in the
error when the request fails.

The singular `mr job submit`, `cancel`, `pause`, `resume`, and `retry` commands
remain compatibility aliases for existing download scripts. `cancel`, `pause`,
`resume`, and `retry` accept the Job id `mr jobs list` prints as well as the
legacy handle `mr job submit` returns. A Job of a Kind the legacy routes do not
project, such as a plugin command run, is sent the advertised command of the same
name instead. `mr jobs queue` returns the legacy queue response explicitly.

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

A summary's failures by class count every Job that recorded a failure: each
failed Job, and each interrupted Job that recorded why it was interrupted. An
interrupted Job that recorded no reason is not in that figure.

The export Job applies the same visibility predicate and filters as interactive
summary, except `state=partial`, `inboundRelationship`,
`noInboundRelationship`, `ownerDeleted` and `owner=me`, which an export refuses
with a 400. An export's filter
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
publish an entity output too. An entity output is offered, and opened, only
while the viewer can see the entity it names: once the entity is deleted or
leaves the viewer's scope, the Job no longer lists it and its link disappears
from every surface. The Jobs panel, the Job Center list, and the Job
detail page link a succeeded Job's available entity output. For plugin actions
they also show a direct “View result” link for older summary outputs that
stored the same safe redirect before entity outputs were published. Job detail
still offers “View JSON result” for the stored summary. A summary whose redirect
names an entity the viewer can no longer open is offered without the redirect,
here and in the legacy plugin-action reads (`GET /v1/jobs/action/job` and the
action rows of the legacy event stream). A download that failed
because the library already holds its bytes publishes the Resource holding them
as its `existing-resource` entity output. The Job detail page links to it from
the Failure section. Only a Resource the submitter can see counts as already
holding the bytes; for a user limited to a group subtree, content held only
outside it becomes a new Resource of their own (see
[Duplicate Detection](../concepts/resources.md#duplicate-detection)).

Every Job's `progress` object carries `metrics`, `rate` (running Jobs only),
`averageRate` (Jobs that are not running), `eta` with `etaEstimated`, and
`updatedAt`. A blocked Job carries `blockedReason`, the reason code its latest
`blocked` event recorded, such as `role-refused`. The progress series
is included as `progress.series` on `GET /v1/jobs/{id}`, and on `GET /v1/jobs`
only with `include=progressSeries`. Series points use short names: `t` is Unix
milliseconds, `c` the completed amount, `r` the rate per second since the
previous point, and `v` the graphed metrics by key. `series.units` records the
unit each graphed key was reported in; a key that comes back in another unit
starts a fresh history. A tick that reports no measure at all, such as an HLS
stream while it muxes, leaves the history as it was.

Each durable event on the canonical stream carries an SSE `id` of the form
`v2:<n>`, where `n` is its delivery sequence: the order events were published,
which keeps each Job's own events in their own sequence. A reconnect resumes
after the cursor in `Last-Event-ID`, or in the `cursor` query parameter when
that header is absent, and first replays what was published since. With
neither, the stream replays everything published, unless the request sets
`start=head`: it then starts at the newest delivery sequence the database has
issued and replays nothing. That is what the Jobs panel, the Job Center and a
Job's page ask for on their first connection, since they read their Jobs
themselves once caught up; a reconnect resumes from the cursor they hold. The
stream then sends `job-caught-up` with the cursor it reached, as
`{"cursor":"v2:<n>"}`. After `start=head`, that `job-caught-up` also carries the
cursor as its SSE `id`, so the browser's own reconnect resumes from it. `owner=me`
narrows the events, the progress frames and a reset's head to the viewer's own
Jobs, as it narrows a list. A resume cursor above the highest delivery sequence this
database has ever issued was issued by a different database: one restored from
an older backup, or an ephemeral server that restarted. The stream then resumes
at the viewer's last published event and adds `"reset": true` to
`job-caught-up`. That one `job-caught-up` also carries the new cursor as its SSE
`id`, so a browser that reconnects before the next event resumes from it.
Nothing the client missed is replayed, so on a reset it discards the sequences
it holds, takes the new cursor, and reads its Jobs again. A cursor this database
did issue is never reset, even when the viewer can no longer see anything at or
above it because retention deleted those Jobs or the viewer's access narrowed.

In the browser, the Job Center and a Job's detail page reload themselves on a
reset: they show nothing but Jobs from the other database and hold no input. On
every other page the Jobs panel stops instead, empties its list, and says that
job updates stopped because the database was restored or replaced, with a
**Reload page** button. It does not reload the page itself, because the page may
hold input that has not been saved. Reloading is still the right next step: a
form rendered from the other database can name ids the new one has given to
different entities.

Two limits are known. A reset, and a stream started with `start=head`, reveal
the highest sequence the database has issued, which every event id a viewer
receives already approximates, since delivery sequences are shared by every
account. And a restored database is
detected only while its sequence is below the tab's cursor: once it has
published past that cursor, a tab resuming from it skips the events in between.
A generation stored in the database cannot close this, because a restore
brings back the old generation with the old rows. After a restore, an
administrator must pass the migration-readiness check before admitting traffic
(see [Backup and Restore](../deployment/backups.md)), so a tab has to outlive that and then
reconnect after enough new events to be affected.

Once the canonical stream has sent `job-caught-up`, each poll also sends a
`job-progress` event for every visible Job whose progress changed in the last
30 seconds and whose current snapshot this connection has not sent yet, up to
the 500 most recently changed Jobs. Its
data is `{jobId, version, state, progress, point, intervalMs}`, where `point` is
the latest series point. Like an ordinary `job-caught-up`, it has no SSE `id`
and never moves the delivery cursor. A new connection can therefore receive frames for
changes an earlier connection already delivered. Each frame replaces the Job's
progress, so a reader treats a repeat as a no-op: it ignores a frame whose
`progress.updatedAt` is older than the progress it holds, and replaces rather
than appends a point whose `t` equals its last point's. Progress timestamps are
written by whichever process runs the Job, and the 30-second window is what
absorbs clock skew between those processes.

The stream's cursor, the SSE `id` (`v2:<n>`) and the `deliverySequence` of
every event, including those `GET /v1/jobs/{id}/events` returns, is one counter
for the whole deployment, which is what lets a reconnect resume exactly where
it stopped. A viewer receives only the events of Jobs they can see, so the gap
between two sequences they receive counts the Job events produced in between
on Jobs they cannot see: other accounts' work, and work no account owns. The
gap names no Job and no account.

Command requests carry `expectedVersion`, `idempotencyKey`, and `origin`. The
server recomputes the command under current authorization and rejects a stale
version. A Retry, Continue, Repeat or Resume whose work the Job's Kind would
refuse when it came to run is refused up front with `409`, result code
`refused` and the reason in `message`, and nothing is created: for example a
download or an export whose target group has left the scope of the account it
would run as. A Retry, Continue or Repeat runs as the account that asks for it;
a Resume runs as the account the Job was accepted for. A target that leaves
the scope after that check but before the new Job is created is not caught up
front: the Job is accepted, and then blocked with `scope-refused` or
`group-out-of-scope` before anything runs. When the account or group read
behind the check fails, the command answers `500` instead of refusing, and
asking again once the database answers is safe. The same read failing as a
Job is about to start (a download, an export, an import, a Job summary export,
a clustering run or a similarity recompute) neither runs the Job nor blocks it:
the Job goes back to `queued` with the event reason `checks-unanswered` and the
message "Waiting for the account and scope checks", and the server process
that tried waits 1 second before asking again, doubling after each failure in
a row up to 30 seconds. Another server process may ask sooner.
Bulk requests accept at most 200 Job IDs; each result commits independently, so
a response can contain both successes and refusals.

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
person's view, and Undismiss reverses it. A Job's `pinned` and `dismissed`
fields report the asking viewer's own preferences. Output artifacts are reauthorized when opened; holding a visible
Job does not grant an output access token.

## Related pages

- [Download Queue](./download-queue.md)
- [Group Export / Import](./export-import.md)
- [Advanced Configuration](../configuration/advanced.md)
- [Backup and Restore](../deployment/backups.md)
