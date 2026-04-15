---
title: mr log
description: View a log entry or entity history
sidebar_label: log
---

# mr log

The activity log is an append-only record of create, update, and delete
events the server writes as resources, notes, groups, tags, and other
entities change. Each entry captures a level, action, entity type and
ID, a human message, the request path, and a timestamp — the raw JSON
uses lowercase keys (`id`, `level`, `action`, `entityType`, etc.), not
the PascalCase shape used elsewhere in the API.

Use the `log` subcommands to inspect single rows. `log get <id>` fetches
one entry by its numeric ID. `log entity --entity-type=X --entity-id=Y`
returns every entry for one specific entity, newest first. For a broad
query across the whole system, use `logs list`.

## Usage

    mr log

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
## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr logs list`](../logs/list.md)
- [`mr log entity`](./entity.md)
- [`mr log get`](./get.md)
