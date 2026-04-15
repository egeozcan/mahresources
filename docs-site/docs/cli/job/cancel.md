---
title: mr job cancel
description: Cancel a job
sidebar_label: cancel
---

# mr job cancel

Stop an active download job. Cancel only works while the job is still
in progress (pending, downloading, or processing); the server rejects
cancellation of jobs that have already finished, been cancelled, or are
paused. On success the server marks the job `cancelled` and leaves it
in the queue for inspection.

Use `jobs list` to see which jobs are eligible — any job with a status
other than pending, downloading, or processing cannot be cancelled.

## Usage

    mr job cancel <id>

Positional arguments:

- `<id>`


## Examples

**Cancel a specific job**

    mr job cancel a1b2c3d4

**Pipe through jq to cancel every active job**

    mr jobs list --json | jq -r '.jobs[] | select(.status == "downloading" or .status == "pending") | .id' | xargs -I {} mr job cancel {}


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

Object with status set to "cancelled"

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr job submit`](./submit.md)
- [`mr job pause`](./pause.md)
- [`mr jobs list`](../jobs/list.md)
