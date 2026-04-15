---
title: mr log entity
description: Get log entries for a specific entity
sidebar_label: entity
---

# mr log entity

Fetch every log entry recorded for one specific entity. Both
`--entity-type` (e.g. `group`, `resource`, `note`, `tag`) and
`--entity-id` are required. The response is the same paginated wrapper
`logs list` returns, so the `logs` array contains the actual rows and
pagination is controlled by the global `--page` flag.

This is the reliable way to discover a log row's ID from code: create
or touch an entity, then query its history to get the `id` value used
by `log get`. The action field (`create`, `update`, `delete`, `system`)
lets scripts filter to just the events they care about.

## Usage

    mr log entity

## Examples

**List every log entry for group 42**

    mr log entity --entity-type=group --entity-id=42

**Pull only the actions for one resource**

    mr log entity --entity-type=resource --entity-id=7 --json | jq -r '.logs[].action'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--entity-type` | string | `` | Entity type (required) **(required)** |
| `--entity-id` | uint | `0` | Entity ID (required) **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Paginated wrapper with logs (array of entries), totalCount, page, perPage; each entry has id, level, action, entityType, entityId, entityName, message, requestPath, createdAt (lowercase keys)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr logs list`](../logs/list.md)
- [`mr log get`](./get.md)
