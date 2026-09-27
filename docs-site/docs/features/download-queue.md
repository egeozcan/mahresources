---
sidebar_position: 6
---

# Download Queue

Queue up to 100 URLs for background download. Concurrency is the shared background-job budget set by `-max-job-concurrency` (default 6), with real-time progress via Server-Sent Events.

Every newly accepted download also publishes a durable `remote-download` Job.
Its canonical UUID and lifecycle remain stable across retries and restarts. The
legacy queue and history endpoints continue to project their established
download response during the Job Center compatibility window. See the [Job
System](./job-system.md) for canonical states, commands, retention, and the
release-gated API.

![Download queue on dashboard](/img/download-queue.png)

## How It Works

When you submit a URL for download:

1. A job is created and added to the queue
2. The download starts in the background (once a slot in the shared job concurrency budget is free)
3. Progress is tracked and broadcast via Server-Sent Events (SSE)
4. On completion, a Resource is created from the downloaded file

A download with no name of its own is named after what the server delivered:
the filename a `Content-Disposition` header gives (the RFC 6266 `filename*`
form when it sends one), otherwise the last path segment of the URL the
response came from, which after a redirect is the redirect's target, decoded
and without its query string, otherwise the host. The name keeps only its last
path element, and control and bidirectional-formatting characters are removed.
The same rule names a resource created from the remote-resource form,
`POST /v1/resource/remote` and `mah.db.create_resource_from_url`. An assembled
HLS stream takes the playlist's name with an `.mp4` extension.

The Resource is created as the person who submitted the download, with their account as it stands when the transfer finishes. For a user limited to a group subtree it lands inside that subtree, and content the library holds only outside it becomes their own Resource rather than a link to one they cannot open (see [Duplicate Detection](../concepts/resources.md#duplicate-detection)). If the account has been disabled or deleted by then, or its role no longer allows creating content, no Resource is created, and the Job fails with the code `submitter-refused` and a message saying so.

## Queue Limits

| Setting | Value |
|---------|-------|
| Max concurrent jobs | `-max-job-concurrency` (default 6, shared with group export and import) |
| Max queue size | 100 (counts export and import jobs too) |
| Job retention (completed) | 1 hour |
| Job retention (paused) | 24 hours |

When the queue is full, completed jobs are evicted first (oldest first), then failed/cancelled jobs. Active and paused jobs are never evicted.

## Download history

A finished download remains visible through the durable Job record and, while
the compatibility projection is retained, through the legacy download history
row in the Job Center at `/jobs`. The Job survives queue eviction and restart according to
Job retention. Legacy handles follow the current Retry leaf during the
compatibility window; a canonical UUID always identifies one Job. Group
exports, imports, Resource Reduction computation, similarity recomputes, and
plugin actions also publish through the durable Job Service.

- Every non-admin principal sees only the rows it submitted.
- **Retry** through the legacy handle creates a new canonical Job and moves the handle to that Retry leaf. The source Job keeps its original outcome. Current authorization and download scope are checked again. While the queue still holds the failed attempt and another download of the same URL is pending, downloading, processing or paused, the retry is refused with 409; otherwise the new Job waits for the URL (see **One transfer per URL** below).
- **Delete** removes the queue entry along with the row, so the SSE stream's `init` replay cannot resurrect it.
- A restart is not a cancellation, and nothing records it as one. When the server stops gracefully, a download that was running goes back to the queue under the same Job and handle, with the event reason `server-shutdown` and the message "Stopped by a server shutdown; it starts again from the beginning" on its row, and starts again from the beginning when a server claims it: at once in a deployment with another process running, otherwise when the server is back. A paused download stays held until someone resumes or cancels it. Neither writes a history row until it finishes. After a crash the Job reaches the same state once its claim expires and the process that held it is known to be gone. A download the queue runs without a durable Job is recorded as failed with the reason "The server shut down before the download finished".

See [Job System](./job-system.md) for the UI, and [Runtime Settings](../configuration/runtime-settings.md) for the retention windows.

## Plugin download pacing and deferral

A plugin can throttle only the downloads it submits through `mah.download.submit`
by declaring `download_limits` in its manifest:

```lua
plugin = {
  api_version = 1,
  capabilities = { "db:write" },
  network = { "*.example.com" },
  download_limits = {
    { host = "*.example.com", concurrency = 2, min_interval = "5s", backoff = "60s" },
  },
}
```

`host` uses the same grammar as the plugin `network` allowlist. The first
matching rule wins. `concurrency` limits this plugin's jobs for that rule;
leaving it out means unlimited concurrency for the rule. `min_interval`
spaces job starts, and `backoff` pauses later matching jobs after the submitted
URL answers `429` or `503`; `Retry-After` is honored when present and clamped
to the declared backoff ceiling. A zero or absent backoff disables that part.
All waits happen before the shared job semaphore, so one throttled domain does
not occupy the deployment-wide download slots while it sleeps.

This is a **job-level** gate. One HLS download may still fetch segments in
parallel according to `-hls-concurrency`, and segment-level `429` handling stays
inside the HLS fetcher. Generic jobs (exports, imports and plugin action jobs)
do not enter this gate because they have no submitted remote URL. The pacing
state is process-local memory: a restart forgets last-start and backoff timing,
while durable permissions and download history remain in the database.

`mah.download.submit` can also defer a single host download with `{ delay =
"2h" }` or `{ start_at = <unix seconds> }`, one or the other. A delay must
satisfy `0 <= delay <= 30 days`; an absolute `start_at` must be in the
future and before the year 10000. The deferral is stored as a scheduled-download
row, which the plugin management page and `mr plugin scheduled-downloads` list,
and as a `scheduled` Job of Kind `deferred-download`, which the Job Center
shows. Both are durable and survive a restart, but neither is a resident queue
job until it is due; keeping future work out of the in-memory queue avoids the
100-job cap and pending-job eviction rules. When the due time comes, the Job is
queued, the plugin and submitting user are re-validated, and the plugin
scheduler's next tick marks the row `submitted` with the Job's id; until then
the row reads `pending` even if the Job has started. The re-validation belongs to
the Job: if the plugin is disabled, or the user may no longer write or reach the
download's targets, the Job is blocked with that reason in the Job Center, and
the row still reads `submitted`. If the same URL is already downloading, the Job
waits for that transfer and then runs. The download becomes eligible to start at the
time a `start_at` or a `delay` names, whatever time zone the server runs in; it
then runs as soon as the job runtime has capacity for it. If the submitting user is
deleted before a pending row fires, the row becomes ownerless and is never
claimed.

The row and the Job are cancelled together. The Job's **Cancel**, used before
the download has started, ends the row as `cancelled`. The admin-only
`POST /v1/plugin/scheduled-downloads/cancel` endpoint cancels a pending row and
its Job; it refuses a row that has been submitted or whose Job has started,
which the Job's own Cancel stops instead.

Retrying a deferred download that never started downloads it now: the control
reads **Download now** and asks for confirmation, because the time it was
scheduled for is not kept. The new Job is an ordinary download. The plugin's
row keeps the status it ended with and does not follow it. A deferred download
that ran offers an ordinary **Retry**.

## Streaming playlists (HLS)

A URL that returns an HLS playlist is assembled rather than stored. The server
fetches the playlist, picks the highest-bandwidth rendition of a master
playlist, downloads every segment, and muxes them into a single MP4 with
ffmpeg. The resource that lands is the video, not the few kilobytes of text
that listed it.

This applies wherever the server fetches a URL for you: the download queue, the
remote-resource form, `POST /v1/resource/remote`, the `mr` CLI, and a plugin's
`mah.db.create_resource_from_url`. Nothing needs to be enabled, and a playlist
is recognised from its content rather than from an `.m3u8` extension, because
these URLs are usually generated endpoints with no extension at all.

Progress is reported through the job's phase counters (`downloading segments
42/380`, then `assembling video`) rather than its byte counters, since the size
of a stream is not known until its last segment has arrived.

**Every segment and key is fetched by the server itself, through the same
policy the submitted URL was checked against** — see [Where downloads may
point](#where-downloads-may-point). ffmpeg is handed local files only and
cannot open a network connection, so a playlist cannot be used to reach an
address the deployment does not allow.

What is refused, and why the message says so rather than failing obscurely:

| Case | Reason |
|------|--------|
| A live stream (no `#EXT-X-ENDLIST`) | Its segment window slides, so a download would capture an arbitrary clip of whatever was current, not the broadcast |
| DRM (`SAMPLE-AES`, FairPlay, Widevine, PlayReady) | The content is licensed; this is not a missing parser |
| A playlist naming a `file:` or other non-HTTP URL | The fetch policy checks addresses, not schemes |
| More segments or bytes than the deployment allows | `-hls-max-segments` (default 5000) and `-hls-max-bytes` (default 16 GiB) |
| No ffmpeg on the server | Reported before any segment is downloaded, so a doomed transfer is not paid for |

AES-128 encrypted playlists are supported: the key is fetched through the same
policed client and handed to ffmpeg as a local file. So are streams whose audio
is a separate rendition (`#EXT-X-MEDIA`), which are downloaded and muxed in --
without that the result would be a silent video that plays perfectly.

`-hls-concurrency` (default 4) sets how many segments are fetched at once.

`-hls-temp-dir` sets where the assembly works. It defaults to the system temp
directory, which in most container images is the root filesystem -- and an
assembly holds every segment plus the finished video, so a long recording wants
a multiple of its own size somewhere that has it. It also covers the copy made
while the finished file is stored, since assembling onto a media volume and then
copying to the root filesystem is the same problem one step later.

## Job Lifecycle

The legacy queue projects these download statuses. They are not canonical Job
states:

| Status | Description |
|--------|-------------|
| `pending` | Queued, waiting for a download slot |
| `downloading` | Actively downloading the file |
| `processing` | Download complete, creating the Resource |
| `completed` | Resource created successfully |
| `failed` | An error occurred |
| `cancelled` | Cancelled by user |
| `paused` | Paused by user (can be resumed) |

A failed download's Job records why it failed: for example
`HTTP 403 Forbidden`, `connect: connection refused`, or the timeout that ended
the transfer. A transfer that runs past `-remote-overall-timeout` says it did
not finish within the overall time limit and names the limit; only a person's
cancel reads as cancelled. The Jobs drawer shows it under the failed row as **Reason**, and
`/jobs` and the Job detail page show the same text. The reason names no URL
beyond its scheme and host, because a Job's failure message is stored as plain,
searchable text and a URL's path and query can hold a signature or token. An
HTTP status is named by its code and standard meaning, not by the text the
server sent with it. The legacy download endpoints still report the error's
full text. A failure that has no message shows its code instead.

A download whose bytes are already in the library fails with the code
`resource-exists` and the class `conflict`, and its message names the
existing resource's ID. The Job publishes that resource as its
`existing-resource` entity output, so the Failure section of the Job detail
page links to it with **View existing resource**. The link checks access
when opened, like every entity output.

### Failure reasons and Retry

Every failed download records a code and a class, which the Job Center's
failure breakdown and the summary export group on. Retry is offered for every
failure except one whose stored address can never be fetched by itself: an
address that is not an absolute `http` or `https` URL. Everything else depends
on something that can change. A remote's answer can (a 404 becomes a 200 once
something is published there, a live stream ends), the library can (the
resource already holding the bytes can be deleted), and so can this
deployment's policy and limits.

| Code | Class | Retry | Cause |
|------|-------|-------|-------|
| `invalid-url` | `validation` | no | The stored address is not an absolute `http` or `https` URL with a host. Submission refuses such an address; a Job accepted before it did can still hold one |
| `remote-client-error` | `dependency` | yes | The remote answered with a 4xx not listed below |
| `remote-forbidden` | `dependency` | yes | The remote answered 403 |
| `remote-busy` | `dependency` | yes | The remote answered 423, 425 or 429 |
| `remote-server-error` | `dependency` | yes | The remote answered 5xx or another unexpected status |
| `remote-connection-failed` | `dependency` | yes | The name did not resolve; the connection was refused, reset or dropped, before the answer or inside its body; or the TLS handshake failed, an untrusted or invalid certificate included |
| `remote-timeout` | `timeout` | yes | Connecting or waiting for the response headers timed out, or the remote answered 408 |
| `idle-timeout` | `timeout` | yes | The remote stopped sending for longer than `-remote-idle-timeout` |
| `overall-timeout` | `timeout` | yes | The transfer ran past `-remote-overall-timeout` |
| `address-refused` | `policy` | yes | The fetch policy refused an address or host (see [Where downloads may point](#where-downloads-may-point)) |
| `plugin-unavailable` | `policy` | yes | A plugin's download whose plugin, and so its network policy, is no longer enabled |
| `submitter-refused` | `policy` | yes | The submitter may no longer create content |
| `unsupported-stream` | `validation` | yes | An HLS stream this server refuses (live, DRM, a non-HTTP URL, a kind it does not handle) |
| `stream-over-limit` | `policy` | yes | An HLS stream over `-hls-max-segments` or `-hls-max-bytes` |
| `ffmpeg-unavailable` | `dependency` | yes | An HLS stream and no ffmpeg to assemble it |
| `resource-exists` | `conflict` | yes | The library already holds the bytes |
| `download-failed` | `internal` | yes | Anything else |

## Job Operations

- **Cancel, pause, resume, retry** -- The compatibility endpoints remain
  available while clients migrate. The canonical interface renders only the
  commands the current Job detail advertises and rechecks role, scope, and Job
  version when the command runs.
- **Retry** -- Creates a linked Job and preserves the earlier Job's terminal
  state. A failed legacy download handle resolves to the current Retry leaf for
  at least one documented release and six months after canonical cutover. It is
  offered for every failure except a stored address that is not a download (see
  [Failure reasons and Retry](#failure-reasons-and-retry)).
- **One transfer per URL** -- A Job about to start while another transfer in
  this process is fetching the same URL goes back to the queue to wait for it,
  with the phase `waiting` and the message "Waiting for another download of this
  URL to finish" on its row. It holds no slot of the concurrency budget while it
  waits, the dispatch loop passes over it, and it starts on the first pass after
  the other transfer ends. That covers a new submission, a Retry, a deferred
  download coming due, queued work, and a paused download being resumed. A
  paused download does not hold its URL, since it fetches nothing and may wait
  for a person indefinitely. A cancel ends the waiting Job like any queued Job
  and leaves the other transfer alone. A Retry from the Job Center
  (`POST /v1/jobs/{id}/commands/retry`) always waits. The legacy
  `POST /v1/download/retry` and `POST /v1/jobs/retry` refuse the retry with 409
  instead while the queue still holds the failed attempt and another download
  of the URL is pending, downloading, processing or paused; once the queue no
  longer holds it, they create a Retry that waits like any other.

## Submitting Downloads

### Single URL

``` 
POST /v1/jobs/download/submit
Content-Type: application/json

{
  "url": "https://example.com/file.pdf",
  "name": "My Document",
  "ownerId": 123,
  "tags": [1, 2]
}
```

Legacy alias: `POST /v1/download/submit`

### Multiple URLs

Submit multiple URLs separated by newlines in the `url` field. Each URL becomes a separate job in the queue.

Each line must be an absolute `http` or `https` URL with a host. A line that is
not one is refused when it is submitted, before any Job is made for it, and the
others are still accepted: the answer is `202 Accepted` with the refused lines
listed under `refused`, each with its `url` and `reason`. When no line is
accepted the answer is `400 Bad Request` naming the first refused line. A
deferred plugin download is checked the same way when it is scheduled.

## Timeout Configuration

Remote download timeouts are configurable via command-line flags or environment variables:

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | 30s | Timeout for establishing a connection |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | 60s | Timeout when the remote server stops sending data |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | 30m | Maximum total time for a download |
| `-remote-user-agent` | `REMOTE_USER_AGENT` | browser-like | User-Agent every request this server makes on your behalf sends |

The same four values are also runtime-editable at `/admin/settings`, and the queue reads them at the start of every download, so a change applies without a restart.

### Request headers

The server identifies itself with a browser-like `User-Agent`, because some
media endpoints answer Go's default with HTTP 403. Set `-remote-user-agent` (or
the runtime `remote_user_agent`) to send something else; it applies to the
synchronous remote upload, the download queue, every HLS playlist, key and
segment beneath them, and the calendar block's ICS fetch.

One download can also carry extra headers of its own — a `Referer` or a
`Cookie` a particular endpoint wants. They are accepted as a `headers` object
on the JSON body of `POST /v1/download/submit` and `POST /v1/resource/remote`,
and as a `headers` option on the plugin calls `mah.download.submit` and
`mah.db.create_resource_from_url`:

```lua
mah.download.submit("https://example.com/media/123", {
  owner_id = 42,
  headers = { Referer = "https://example.com/watch/123" },
})
```

Three rules govern them:

- **A `User-Agent` among them replaces the deployment's, for the whole
  download.** An endpoint that refuses one agent refuses it on its CDN too, so
  binding it to the submitted host would fix the playlist and leave every
  segment failing. It names the fetcher rather than the user, which is why it
  is safe on any host — which also means it is the wrong place for a secret.
  Put anything that must not travel in a header of its own, which stays bound
  to the submitted origin.
- **Every other header is sent to the submitted URL's own host and nowhere
  else.** An HLS
  playlist names further URLs, and the host fetch policy permits any public
  host, so replaying a `Cookie` onto whatever a playlist says would hand your
  credential to a server the content chose. The `User-Agent` has no such
  restriction: it identifies the fetcher, not you.
- **Connection-level headers are refused** — `Host`, `Content-Length`,
  `Connection`, `Keep-Alive`, `Transfer-Encoding`, `Upgrade`, `TE`, `Trailer`
  and the whole `Proxy-*` family — along with `Range`, which the HLS assembler
  sets itself per byte range. So are two spellings of one header (`Cookie` and
  `cookie`), which would otherwise resolve by map order. The refusal
  happens when you submit, not when the transfer runs.
- **Headers follow the origin, not just the host.** A redirect from `https` to
  the same name over plain `http` is a downgrade, and the caller's headers stay
  behind rather than going out in clear.
- **They are stored on the download history row** so a retry replays them, and
  a stored credential outlives the browser tab you typed it into. The payload
  is never rendered to a page, but it is in the database until the row is
  swept.

Headers travel on JSON bodies only. The create-resource form posts no header
map.

### History retention

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-download-failed-retention` | `DOWNLOAD_FAILED_RETENTION` | 168h | How long a failed or cancelled download stays in the download history |
| `-download-history-retention` | `DOWNLOAD_HISTORY_RETENTION` | 24h | How long a completed download stays in the download history |
| `-download-cockpit-limit` | `DOWNLOAD_COCKPIT_LIMIT` | 10 | How many finished jobs the Jobs drawer shows; running, waiting and failed jobs are always listed |

All three are editable at runtime via `/admin/settings`. A zero value falls back to the default rather than expiring on write.

## Where downloads may point

A download URL is supplied by a user and fetched by the server, so the queue
refuses any URL that resolves to a **private** address -- loopback, link-local
(including the cloud metadata endpoint), RFC1918 and carrier-grade NAT. Public
hosts are unaffected, except `168.63.129.16`, Azure's host-internal
platform-agent endpoint, which is refused like a private address despite being
numbered out of public space.

To download from your own network, name the addresses with
`-allow-private-fetch`; see
[Fetching from your own network](../configuration/overview.md#fetching-from-your-own-network).
A download refused this way fails with a "blocked request" error that
deliberately does not name the address the URL resolved to -- the submitter is not
told which internal addresses exist. The full detail, including the resolved
address, is written to the activity log for administrators.

## API Endpoints

### Legacy Download-Specific Aliases

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/download/submit` | Submit download URL(s) |
| `GET` | `/v1/download/queue` | List all download jobs |
| `POST` | `/v1/download/cancel` | Cancel a download (`id`) |
| `POST` | `/v1/download/pause` | Pause a download (`id`) |
| `POST` | `/v1/download/resume` | Resume a paused download (`id`) |
| `POST` | `/v1/download/retry` | Retry a failed download (`id`) |
| `GET` | `/v1/download/events` | SSE event stream (downloads and plugin action jobs) |

### Legacy Job Compatibility Routes

These routes preserve their existing download queue and handle shapes. They
remain available during the documented compatibility window; they are not the
canonical Job Center API.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/jobs/download/submit` | Submit download URL(s) |
| `GET` | `/v1/jobs/queue` | List download jobs |
| `POST` | `/v1/jobs/cancel` | Cancel a download |
| `POST` | `/v1/jobs/pause` | Pause a download |
| `POST` | `/v1/jobs/resume` | Resume a download |
| `POST` | `/v1/jobs/retry` | Retry a download |
| `GET` | `/v1/jobs/get` | Return one job snapshot by id |
| `POST` | `/v1/jobs/clearCompleted` | Dismiss every finished job (completed, failed, cancelled) |
| `GET` | `/v1/jobs/events` | SSE event stream (all job types) |

Canonical list, detail, command, timeline, output, summary, and export routes
are described in the [Job System API table](./job-system.md#canonical-api). They
are available together after the startup retirement barrier passes.

### Download history

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/downloads` | List persisted download history rows |
| `POST` | `/v1/downloads/retry` | Retry history rows by id |
| `POST` | `/v1/downloads/delete` | Delete history rows by id, and their queue entries |

## SSE Event Format

On connect, the server sends an `init` event with the full current state of all jobs:

```
event: init
data: {"jobs":[...],"actionJobs":[...]}
```

Subsequent events use SSE event names (`added`, `updated`, `removed`) with JSON data:

```
event: added
data: {"type":"added","job":{"id":"abcd1234","status":"pending","url":"https://example.com/file.pdf"}}

event: updated
data: {"type":"updated","job":{"id":"abcd1234","status":"downloading","progress":45}}

event: removed
data: {"type":"removed","job":{"id":"abcd1234","status":"completed"}}
```

Each event data contains:
- `type` -- `"added"`, `"updated"`, or `"removed"`
- `job` -- The full job object with current status, progress, and metadata

Download progress updates are throttled to one event per 500ms per job.

:::note
The `/v1/download/events` and `/v1/jobs/events` endpoints serve identical streams. Both merge download job events and plugin action job events into a single SSE connection.
:::

Each frame is filtered for the account as it is when the frame is sent, not as it was when the connection opened. A stream whose session or token stops authenticating closes, and one whose account changes role or scope goes on with what the account may see now. See [Job System](./job-system.md#state-and-visibility).
