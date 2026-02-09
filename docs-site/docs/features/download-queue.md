---
sidebar_position: 6
---

# Download Queue

The download queue manages background URL downloads, allowing you to queue multiple remote files for download without blocking the UI.

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

## Progress Tracking

Subscribe to real-time progress updates via SSE:

```
GET /v1/download/events
```

Events are JSON objects with:
- `type` -- `"added"`, `"updated"`, or `"removed"`
- `job` -- The full job object with current status, progress, and metadata

Progress updates are throttled to one event per 500ms per job to avoid flooding clients.

## Timeout Configuration

Remote download timeouts are configurable via command-line flags or environment variables:

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | 30s | Timeout for establishing a connection |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | 60s | Timeout when the remote server stops sending data |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | 30m | Maximum total time for a download |

## Listing Jobs

```
GET /v1/download/queue
```

Returns all jobs in order, including their current status and progress.
