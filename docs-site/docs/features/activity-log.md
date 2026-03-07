---
sidebar_position: 7
title: Activity Log
---

# Activity Log

Every create, update, and delete operation is recorded with the HTTP request context that triggered it. Log entries are written asynchronously (fire-and-forget) so they do not slow down the operation itself.

![Activity log showing recent entity changes](/img/activity-log.png)

## Log Entry Properties

| Property | Type | Description |
|----------|------|-------------|
| `level` | string | `info`, `warning`, or `error` |
| `action` | string | `create`, `update`, `delete`, `system`, or `progress` |
| `entityType` | string | Entity kind: `resource`, `note`, `group`, etc. |
| `entityId` | uint | ID of the affected entity (nullable) |
| `entityName` | string | Name of the entity at the time of the action |
| `message` | string | Human-readable description |
| `details` | string | JSON with additional context |
| `requestPath` | string | HTTP path that triggered the action |
| `userAgent` | string | Client user agent |
| `ipAddress` | string | Client IP address |

## Log Levels

| Level | Usage |
|-------|-------|
| `info` | Normal operations -- entity creation, updates, deletions |
| `warning` | Non-critical issues that may need attention |
| `error` | Failed operations or system errors |

## Log Actions

| Action | Description |
|--------|-------------|
| `create` | A new entity was created |
| `update` | An existing entity was modified |
| `delete` | An entity was deleted |
| `system` | System-level events (startup, migration, configuration) |
| `progress` | Long-running operation progress updates |
| `plugin` | Plugin hook or action execution events |

## Viewing Logs

### In the UI

Navigate to the activity log page to see a chronological list of all logged operations. Each entry shows the level, action, entity link, message, and timestamp.

Entity detail pages also display recent log entries for that specific entity.

### Filtering

Filter log entries by combining any of these parameters:

| Parameter | Type | Description |
|-----------|------|-------------|
| `level` | string | Filter by level: `info`, `warning`, `error` |
| `action` | string | Filter by action: `create`, `update`, `delete`, `system`, `progress` |
| `entityType` | string | Filter by entity kind |
| `entityId` | uint | Filter by specific entity ID |
| `Message` | string | Search by log message |
| `RequestPath` | string | Filter by HTTP request path |
| `CreatedBefore` | timestamp | Entries created before this time |
| `CreatedAfter` | timestamp | Entries created after this time |
| `SortBy` | string | Sort field |

## Configuration

### Log Cleanup

Old log entries can be deleted automatically at startup:

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `-cleanup-logs-days` | `CLEANUP_LOGS_DAYS` | `0` (disabled) | Delete entries older than N days on startup |

```bash
./mahresources -cleanup-logs-days=90 ...
```

Set to `0` (default) to retain all log entries indefinitely.

## API Endpoints

### List Log Entries

```
GET /v1/logs
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `level` | string | Filter by log level |
| `action` | string | Filter by action type |
| `entityType` | string | Filter by entity kind |
| `entityId` | uint | Filter by entity ID |
| `Message` | string | Search by log message |
| `RequestPath` | string | Filter by HTTP request path |
| `CreatedBefore` | timestamp | Entries before this time |
| `CreatedAfter` | timestamp | Entries after this time |
| `SortBy` | string | Sort field |

```bash
curl "http://localhost:8181/v1/logs?level=error"
```

```json
{
  "logs": [
    {
      "ID": 42,
      "level": "error",
      "action": "system",
      "entityType": "",
      "entityName": "",
      "message": "Failed to generate thumbnail",
      "details": "{\"resourceId\": 1234}",
      "requestPath": "/v1/resource/preview",
      "userAgent": "Mozilla/5.0",
      "ipAddress": "127.0.0.1",
      "CreatedAt": "2025-03-01T10:30:00Z"
    }
  ],
  "totalCount": 1,
  "page": 1,
  "perPage": 20
}
```

### Get Single Log Entry

```
GET /v1/log
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | uint | Log entry ID |

```bash
curl "http://localhost:8181/v1/log?id=42"
```

### Get Logs for a Specific Entity

```
GET /v1/logs/entity
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `entityType` | string | Entity kind (e.g., `resource`, `note`) |
| `entityId` | uint | Entity ID |

```bash
curl "http://localhost:8181/v1/logs/entity?entityType=resource&entityId=123"
```

Returns all log entries related to the specified entity, ordered by most recent first.

## Troubleshooting

### Log table growing too large

Enable automatic cleanup at startup:

```bash
./mahresources -cleanup-logs-days=30 ...
```

### Missing log entries

Log writes are fire-and-forget -- if the database insert fails (e.g., disk full, connection lost), the error is printed to stderr but the original operation still succeeds. Check stderr output for write failures.
