---
sidebar_position: 7
---

# Activity Log

Mahresources maintains an activity log that tracks operations performed on entities. The log provides an audit trail of create, update, and delete actions across the system.

## Log Entry Properties

| Property | Description |
|----------|-------------|
| `level` | Severity: `info`, `warning`, or `error` |
| `action` | Operation type: `create`, `update`, `delete`, `system`, or `progress` |
| `entityType` | Type of entity affected (e.g., resource, note, group) |
| `entityId` | ID of the affected entity |
| `entityName` | Name of the affected entity at the time of the action |
| `message` | Human-readable description of the action |
| `details` | Optional JSON with additional context |
| `requestPath` | HTTP request path that triggered the action |
| `userAgent` | User agent of the client |
| `ipAddress` | IP address of the client |

## Log Levels

| Level | Usage |
|-------|-------|
| `info` | Normal operations (create, update, delete) |
| `warning` | Non-critical issues that may need attention |
| `error` | Failed operations or system errors |

## Log Actions

| Action | Description |
|--------|-------------|
| `create` | A new entity was created |
| `update` | An existing entity was modified |
| `delete` | An entity was deleted |
| `system` | System-level events (startup, migration, etc.) |
| `progress` | Long-running operation progress updates |

## Querying the Log

```
GET /v1/logs
```

Filter log entries by level, action, entity type, or date range to find specific events.
