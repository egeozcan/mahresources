---
title: mr job command
description: Run a command advertised by Job detail
sidebar_label: command
---

# mr job command

Run a command key that the Job currently advertises in `jobs get`. The CLI
re-reads detail immediately before submission, uses the advertised endpoint
and version, and sends an idempotency key. Supply `--idempotency-key` to replay
the same logical request after a network failure; otherwise the CLI generates
and prints a key for this request. Commands marked destructive or requiring
confirmation need `--confirm`.

## Usage

```bash
mr job command <job-id> <command-key>
```

Positional arguments:

- `<job-id>`
- `<command-key>`


## Examples

**Run a command currently advertised by the server**

```bash
mr job command 018f4db1-9b40-7f54-8f16-37a449bcf01d retry
```

**Confirm a destructive command and provide a reusable key**

```bash
mr job command 018f4db1-9b40-7f54-8f16-37a449bcf01d cancel --confirm --idempotency-key ops-2026-09-23-1
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--idempotency-key` | string | `` | Stable key to replay this command safely |
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

Accepted Job command result

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr jobs get`](../jobs/get.md)
- [`mr job bulk-command`](./bulk-command.md)
