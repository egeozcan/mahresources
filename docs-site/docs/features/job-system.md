---
sidebar_position: 14
title: Job System
---

# Job System

The job system aggregates download queue jobs and plugin action jobs into a single SSE event stream.

## Job Sources

| Source | Origin | ID Format | Max Concurrent |
|--------|--------|-----------|---------------|
| `download` | Download queue | Random 8-char hex | 3 |
| `plugin` | Async plugin actions and `mah.start_job()` | Random 16-char hex | 3 |

Both job types share the same SSE infrastructure. Download jobs are also available via a dedicated listing endpoint (`/v1/jobs/queue`), while plugin action jobs appear only in the SSE event stream (via the `init` payload and subsequent `action_*` events).

## Download Jobs

Download jobs are created when URLs are submitted to the download queue. See [Download Queue](./download-queue.md) for submission, pause/resume, and retry details.

### Download Job Statuses

| Status | Description |
|--------|-------------|
| `pending` | Queued, waiting for a download slot |
| `downloading` | Actively transferring data |
| `processing` | Download complete, creating a Resource |
| `completed` | Resource created |
| `failed` | Error occurred |
| `cancelled` | Cancelled by user |
| `paused` | Paused by user |

## Plugin Action Jobs

Plugin action jobs are created when an async action is triggered. See [Plugin Actions](./plugin-actions.md) for registration and handler details.

### Action Job Structure

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Random hex ID |
| `source` | string | Always `"plugin"` |
| `pluginName` | string | Source plugin name |
| `actionId` | string | Action identifier |
| `label` | string | Action display label |
| `entityId` | uint | Target entity ID |
| `entityType` | string | `"resource"`, `"note"`, `"group"`, or `"custom"` (for `mah.start_job()`) |
| `status` | string | `"pending"`, `"running"`, `"completed"`, or `"failed"` |
| `progress` | int | 0-100 |
| `message` | string | Current status message |
| `result` | object | Action result data (on completion) |
| `createdAt` | timestamp | Job creation time |

### Progress Control from Lua

```lua
mah.job_progress(job_id, 50, "Processing image...")
mah.job_complete(job_id, { message = "Done", redirect = "/resource?id=42" })
mah.job_fail(job_id, "API returned 500")
```

## Programmatic Action Jobs (`mah.start_job`)

Plugins can create action jobs programmatically using `mah.start_job(label, fn)`, without requiring a user to click an action button. The job is an `ActionJob` with `source: "plugin"`, `actionId: "start_job"`, and `entityType: "custom"`. The callback runs in a background goroutine with a 5-minute timeout and receives the `job_id` as its first argument.

```lua
local job_id = mah.start_job("Import data", function(job_id)
    for i = 1, 100 do
        mah.job_progress(job_id, i, "Processing row " .. i)
    end
    mah.job_complete(job_id, { rows = 100 })
end)
```

## SSE Event Stream

Subscribe to real-time job updates from all sources:

```
GET /v1/jobs/events
```

The stream uses SSE event names to distinguish job types and lifecycle events.

**Initialization**: On connect, an `init` event is sent with all current jobs:

```
event: init
data: {"jobs":[...],"actionJobs":[...]}
```

**Download events** use event names `added`, `updated`, `removed`:

```
event: updated
data: {"type":"updated","job":{"id":"abcd1234","status":"downloading","progress":50}}
```

**Plugin action events** use event names `action_added`, `action_updated`, `action_removed`:

```
event: action_updated
data: {"job":{"id":"a1b2c3d4e5f6g7h8","source":"plugin","status":"running","progress":50}}
```

### Event Types

| SSE Event Name | Source | Trigger |
|---------------|--------|---------|
| `added` | Download | New download job created |
| `updated` | Download | Download status, progress, or message changed |
| `removed` | Download | Download job cleaned up after retention period |
| `action_added` | Plugin | New action job created |
| `action_updated` | Plugin | Action status, progress, or message changed |
| `action_removed` | Plugin | Action job cleaned up after retention period |

### Progress Throttling

SSE notifications are rate-limited to prevent flooding clients:

| Source | Throttle Interval |
|--------|------------------|
| Plugin actions | 200ms |
| Downloads | 500ms |

Progress updates at 100% are always sent immediately regardless of throttling.

Subscribers receive events through a buffered channel (capacity 100). Slow subscribers that fall behind are skipped (non-blocking send).

## Download Job Listing

```
GET /v1/jobs/queue
```

Returns the retained download jobs currently held by the queue manager. Plugin action jobs are not included in this endpoint; they are delivered through the SSE event stream (`/v1/jobs/events`).

```bash
curl http://localhost:8181/v1/jobs/queue
```

## Job Cleanup

Completed and failed jobs are removed automatically:

| Setting | Value |
|---------|-------|
| Cleanup interval | Every 5 minutes |
| Action job retention | 1 hour |
| Download job retention (completed) | 1 hour |
| Download job retention (paused) | 24 hours |

Removed jobs trigger `"removed"` SSE events so clients can update their UI.

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/jobs/queue` | List download jobs |
| `GET` | `/v1/jobs/events` | SSE event stream (all job types) |
| `POST` | `/v1/jobs/action/run` | Run a plugin action |
| `GET` | `/v1/jobs/action/job?id={id}` | Get plugin action job status |
| `POST` | `/v1/jobs/download/submit` | Submit download URL(s) |
| `POST` | `/v1/jobs/cancel` | Cancel a download |
| `POST` | `/v1/jobs/pause` | Pause a download |
| `POST` | `/v1/jobs/resume` | Resume a download |
| `POST` | `/v1/jobs/retry` | Retry a download |

## Related Pages

- [Download Queue](./download-queue.md) -- URL submission, pause/resume, and retry details
- [Plugin Actions](./plugin-actions.md) -- action registration, parameters, and async execution
- [Plugin System](./plugin-system.md) -- plugin installation, configuration, and lifecycle
