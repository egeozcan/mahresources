---
title: mr job bulk-command
description: Run one advertised bulk command for several Jobs
sidebar_label: bulk-command
---

# mr job bulk-command

Run one bulk-capable command key on the listed Jobs. Before submitting, the
CLI reads each Job and verifies that the command is advertised as bulk-capable
and that all advertisements have a current version. The server returns a
per-Job result, so some Jobs can succeed while others are refused. Commands
marked destructive or requiring confirmation need `--confirm`.

Provide `--idempotency-key` to safely retry the same request after a network
failure. Otherwise a key is generated and printed with the result.

## Usage

```bash
mr job bulk-command <command-key> <job-id> [job-id...]
```

Positional arguments:

- `<command-key>` (variadic; one or more)
- `<job-id>` (variadic; one or more)


## Examples

**Cancel two Jobs after confirming the advertised command**

```bash
mr job bulk-command cancel 018f4db1-9b40-7f54-8f16-37a449bcf01d 018f4db2-01e5-74b3-9960-653f90e09fa1 --confirm
```

**Supply a stable key for a request that may need replay**

```bash
mr job bulk-command retry 018f4db1-9b40-7f54-8f16-37a449bcf01d 018f4db2-01e5-74b3-9960-653f90e09fa1 --idempotency-key retry-batch-17
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--idempotency-key` | string | `` | Stable key to replay this bulk command safely |
| `--confirm` | bool | `false` | Confirm a destructive command or advertised confirmation |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Per-Job command outcomes, including partial success

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr job command`](./command.md)
- [`mr jobs get`](../jobs/get.md)
