---
title: mr job retry
description: Retry a failed job
sidebar_label: retry
---

# mr job retry

Re-queue a failed or cancelled download job for another attempt.
Retry only works against jobs in the `failed` or `cancelled` state;
the server rejects retry on jobs that are still active, paused, or
already completed. The existing job's ID is reused — progress, error
message, and completion times are cleared, then the worker re-runs the
original URL fetch.

Useful when a transient network error blew up the first attempt.
Persistent failures need an updated URL, which means calling
`job submit` fresh rather than `job retry`.

## Usage

    mr job retry <id>

Positional arguments:

- `<id>`


## Examples

**Retry a specific failed job**

    mr job retry a1b2c3d4

**Retry every failed job in the queue**

    mr jobs list --json | jq -r '.jobs[] | select(.status == "failed") | .id' | xargs -I {} mr job retry {}


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

Object with status set to "retrying"

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr job submit`](./submit.md)
- [`mr jobs list`](../jobs/list.md)
- [`mr job cancel`](./cancel.md)
