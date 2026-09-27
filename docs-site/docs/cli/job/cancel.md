---
title: mr job cancel
description: Cancel a job
sidebar_label: cancel
---

# mr job cancel

Stop a job that has not finished. `<id>` is the Job id `jobs list`
prints, or the legacy handle `job submit` returns as `id`. Cancel works
while the Job is queued, running, or paused; the server rejects
cancellation of a Job that has already succeeded, failed, or been
cancelled, answering HTTP 409 Conflict. On success the Job is recorded
as `cancelled` and stays readable through `jobs get` for inspection.

Use `jobs list --command cancel` to see which Jobs currently offer
cancellation.

## Usage

```bash
mr job cancel <id>
```

Positional arguments:

- `<id>`


## Examples

**Cancel a specific Job**

```bash
mr job cancel 018f4db1-9b40-7f54-8f16-37a449bcf01d
```

**Cancel every visible Job that currently offers cancellation**

```bash
mr jobs list --command cancel --json | jq -r '.jobs[].id' | xargs -I {} mr job cancel {}
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
## Output

Object with status set to "cancelled" and canonicalJobId naming the cancelled Job

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr job submit`](./submit.md)
- [`mr job pause`](./pause.md)
- [`mr jobs list`](../jobs/list.md)
