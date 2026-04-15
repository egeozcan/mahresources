---
title: mr log get
description: Get a log entry by ID
sidebar_label: get
---

# mr log get

Get a single log entry by its numeric ID and print its fields. Output
is a key/value table by default; pass the global `--json` flag to emit
the raw record for scripting. Note that log entries use lowercase JSON
keys (`id`, `level`, `action`, `entityType`, `entityId`, `message`,
`createdAt`) rather than the PascalCase names most other mahresources
entities use.

Log IDs are discovered via `logs list` or `log entity`; they are not
stable across fresh databases, so doctests create an entity first and
then look up the triggered row.

## Usage

    mr log get <id>

Positional arguments:

- `<id>`


## Examples

**Get a log entry by ID (table output)**

    mr log get 42

**Get as JSON and extract the action field with jq**

    mr log get 42 --json | jq -r .action


## Flags

This command has no local flags.
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Log entry object with id (uint), level, action, entityType, entityId, entityName, message, requestPath, createdAt (all lowercase keys)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr logs list`](../logs/list.md)
- [`mr log entity`](./entity.md)
