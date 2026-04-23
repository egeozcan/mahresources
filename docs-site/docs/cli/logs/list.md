---
title: mr logs list
description: List log entries
sidebar_label: list
---

# mr logs list

List log entries across the whole system, optionally filtered. Filter
flags combine with AND. `--level` accepts `info`, `warning`, or
`error`; `--action` accepts `create`, `update`, `delete`, or `system`.
`--entity-type` and `--entity-id` scope results to a single entity
kind or row, while `--message` does a substring match. Date filters
(`--created-before`, `--created-after`) expect RFC3339 strings such
as `2026-04-15T00:00:00Z`.

Pagination uses the global `--page` flag with a fixed page size of 50.
The response wraps the `logs` array with `totalCount`, `page`, and
`perPage` so scripts can walk the full result set. JSON output uses
lowercase keys throughout — match them exactly when building jq
filters.

## Usage

```bash
mr logs list
```

## Examples

**List recent log entries (first page, table output)**

```bash
mr logs list
```

**Filter to deletions only**

```bash
mr logs list --action delete --json | jq -r '.logs[] | "\(.entityType) \(.entityId) \(.message)"'
```

**Filter by entity type and a date window**

```bash
mr logs list --entity-type group --created-after 2026-01-01T00:00:00Z --json
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--level` | string | `` | Filter by level (info/warning/error) |
| `--action` | string | `` | Filter by action (create/update/delete/system) |
| `--entity-type` | string | `` | Filter by entity type |
| `--entity-id` | uint | `0` | Filter by entity ID |
| `--message` | string | `` | Filter by message |
| `--created-before` | string | `` | Filter by created before (RFC3339) |
| `--created-after` | string | `` | Filter by created after (RFC3339) |
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

- [`mr log get`](../log/get.md)
- [`mr log entity`](../log/entity.md)
- [`mr admin`](../admin/index.md)
