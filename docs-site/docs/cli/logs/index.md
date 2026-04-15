---
title: mr logs
description: List and filter audit log entries
sidebar_label: logs
---

# mr logs

The plural `logs` command group reads the server's activity log across
the whole system rather than a single entry. It exposes filtered,
paginated listings so scripts can audit changes, inspect recent
deletes, or build dashboards. Only read operations are provided — the
log is append-only and the server writes it automatically as entities
change.

Use `logs list` with the filter flags (`--level`, `--action`,
`--entity-type`, `--entity-id`, `--message`, `--created-before`,
`--created-after`) to narrow the result set. For single-row lookups
use the singular `log` subcommands (`log get`, `log entity`).

## Usage

```bash
mr logs
```

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

- [`mr log get`](../log/get.md)
- [`mr log entity`](../log/entity.md)
- [`mr admin`](../admin.md)
