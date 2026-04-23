---
title: mr admin stats
description: Show server and data statistics
sidebar_label: stats
---

# mr admin stats

Show administrative statistics about the running server and its data. By default the command fetches three sections — server health (uptime, memory, DB connections), data counts (entity totals), and expensive stats that require full-table scans (hash collisions, dangling references). Together they give a one-page picture of instance size and health.

Use `--server-only` to fetch just the server health block, or `--data-only` to fetch just the data counts — useful for lightweight monitoring that skips the expensive scans. Neither flag is required; when both are unset the command fetches all three sections.

## Usage

```bash
mr admin stats
```

## Examples

**Full admin stats (human-readable, three sections)**

```bash
mr admin
```

**Server health only**

```bash
mr admin --server-only --json
```

**Data counts only**

```bash
mr admin --data-only
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server-only` | bool | `false` | Show server stats only |
| `--data-only` | bool | `false` | Show data stats only |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Combined stats object &#123;serverStats, dataStats, expensiveStats&#125; in JSON mode; three sectioned tables in human mode

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resources versions-cleanup`](../resources/versions-cleanup.md)
- [`mr jobs list`](../jobs/list.md)
- [`mr logs list`](../logs/list.md)
