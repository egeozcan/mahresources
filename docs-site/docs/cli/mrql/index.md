---
title: mr mrql
description: Execute and manage MRQL queries
sidebar_label: mrql
---

# mr mrql

MRQL (Mahresources Query Language) is a small DSL for querying the
mahresources data model across Resources, Notes, and Groups. A single
expression selects an entity type and applies filters, scope, ordering,
limit, and optional `GROUP BY` aggregations — for example
`type = resource AND tags = "photo"` or
`type = resource GROUP BY contentType COUNT()`.

The top-level `mrql` command executes a one-off query supplied as a
positional argument, via `-f <file>`, or on stdin with `-`. Use the
subcommands to manage saved queries: `save` to register a named query,
`list` to discover them, `run` to execute a saved query by name or ID,
and `delete` to remove one. Saved MRQL queries differ from SQL-based
`query` records (see `query run`): MRQL is the high-level DSL, whereas
`query` executes raw read-only SQL.

## Usage

    mr mrql [query]

Positional arguments:

- `<query>` (optional)


## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` | string | `` | Read query from file |
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
## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr mrql list`](./list.md)
- [`mr mrql run`](./run.md)
- [`mr query run`](../query/run.md)
