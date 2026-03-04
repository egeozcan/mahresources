---
sidebar_position: 6
---

# Download Queue

Queue up to 100 URLs for background download. Three run concurrently, with real-time progress via Server-Sent Events.

## How It Works

When you submit a URL for download:

1. A job is created and added to the queue
2. The download starts in the background (up to 3 concurrent downloads)
3. Progress is tracked and broadcast via Server-Sent Events (SSE)
4. On completion, a Resource is created from the downloaded file

## Queue Limits

| Setting | Value |
|---------|-------|
| Max concurrent downloads | 3 |
| Max queue size | 100 |
| Job retention (completed) | 1 hour |
| Job retention (paused) | 24 hours |

When the queue is full, completed jobs are evicted first (oldest first), then failed/cancelled jobs. Active and paused jobs are never evicted.

## Job Lifecycle

Each download job goes through these statuses:

| Status | Description |
|--------|-------------|
| `pending` | Queued, waiting for a download slot |
| `downloading` | Actively downloading the file |
| `processing` | Download complete, creating the Resource |
| `completed` | Resource created successfully |
| `failed` | An error occurred |
| `cancelled` | Cancelled by user |
| `paused` | Paused by user (can be resumed) |

## Job Operations

- **Cancel** -- Stop an active download
- **Pause** -- Pause a pending or downloading job (can be resumed later)
- **Resume** -- Resume a paused job (restarts the download from the beginning)
- **Retry** -- Retry a failed or cancelled job

## Submitting Downloads

### Single URL

```
POST /v1/download/submit
Content-Type: application/json

{
  "url": "https://example.com/file.pdf",
  "name": "My Document",
  "ownerId": 123,
  "tags": [1, 2]
}
```

### Multiple URLs

Submit multiple URLs separated by newlines in the `url` field. Each URL becomes a separate job in the queue.

## Timeout Configuration

Remote download timeouts are configurable via command-line flags or environment variables:

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | 30s | Timeout for establishing a connection |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | 60s | Timeout when the remote server stops sending data |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | 30m | Maximum total time for a download |

## API Endpoints

### Download-Specific Routes

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/download/submit` | Submit download URL(s) |
| `GET` | `/v1/download/queue` | List all download jobs |
| `POST` | `/v1/download/cancel` | Cancel a download (`id`) |
| `POST` | `/v1/download/pause` | Pause a download (`id`) |
| `POST` | `/v1/download/resume` | Resume a paused download (`id`) |
| `POST` | `/v1/download/retry` | Retry a failed download (`id`) |
| `GET` | `/v1/download/events` | SSE event stream (downloads only) |

### Unified Job Routes

These routes serve the same handlers but are prefixed under `/v1/jobs/` and include both download and plugin action jobs where applicable.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/jobs/download/submit` | Submit download URL(s) |
| `GET` | `/v1/jobs/queue` | List all jobs (downloads + plugin actions) |
| `POST` | `/v1/jobs/cancel` | Cancel a download |
| `POST` | `/v1/jobs/pause` | Pause a download |
| `POST` | `/v1/jobs/resume` | Resume a download |
| `POST` | `/v1/jobs/retry` | Retry a download |
| `GET` | `/v1/jobs/events` | SSE event stream (all job types) |

## SSE Event Format

Events use SSE event names (`added`, `updated`, `removed`) with JSON data:

```
event: updated
data: {"type":"updated","job":{"id":"abcd1234","status":"downloading","progress":45}}
```

Each event data contains:
- `type` -- `"added"`, `"updated"`, or `"removed"`
- `job` -- The full job object with current status, progress, and metadata

Download progress updates are throttled to one event per 500ms per job.
