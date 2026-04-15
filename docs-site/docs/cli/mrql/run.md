---
title: mr mrql run
description: Run a saved MRQL query by name or ID
sidebar_label: run
---

# mr mrql run

Execute a saved MRQL query by name or numeric ID. The argument is tried
as an ID first and then as a name, so a name that happens to be numeric
can still be resolved. Returns the same shape as a one-off `mrql` call:
either a standard result with an `entityType` plus the matching entity
arrays, or — for `GROUP BY` queries — a grouped result with `mode`
(`aggregated` or `bucketed`) and `rows` / `groups`.

Pagination and shaping flags (`--limit`, `--buckets`, `--offset`, plus
the global `--page`) apply to the stored query exactly as they would to
an inline `mrql` invocation. Pass `--render` to request server-side
template rendering via the `CustomMRQLResult` template. A missing ID or
name returns HTTP 404.

This is distinct from `query run`, which executes SQL-backed Query
records rather than MRQL DSL expressions.

## Usage

```bash
mr mrql run <name-or-id>
```

Positional arguments:

- `<name-or-id>`


## Examples

**Run a saved query by ID**

```bash
mr mrql run 42
```

**Run by name with bucketed GROUP BY pagination**

```bash
mr mrql run "resources-by-type" --buckets 5
```

**Run and extract result ids with jq**

```bash
mr mrql run "recent-photos" --json | jq -r '.resources[].ID'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--limit` | int | `0` | Items per bucket for GROUP BY, or total items for regular queries |
| `--buckets` | int | `0` | Groups per page for bucketed GROUP BY queries |
| `--offset` | int | `0` | Bucket offset for cursor-based GROUP BY pagination |
| `--render` | bool | `false` | Request server-side template rendering via CustomMRQLResult |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

MRQL result object with entityType (string) and resources/notes/groups arrays, or a grouped result with mode + rows/groups for GROUP BY queries

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr mrql save`](./save.md)
- [`mr mrql list`](./list.md)
- [`mr query run`](../query/run.md)
