---
title: mr job
description: Submit, cancel, pause, or retry a download job
sidebar_label: job
---

# mr job

A download job fetches a remote URL and stores the result as a new
Resource. Each submission creates one job per URL; the server downloads
in the background while the queue tracks progress, pause/resume, and
retry state. Jobs are ephemeral — they live in server memory and do not
persist across restarts.

Use the `job` subcommands to operate on a single job by ID: `submit`
new URLs, `cancel` an active job, `pause` / `resume` an in-flight
transfer, or `retry` a failed one. Use `jobs list` to discover IDs and
check current statuses.

## Usage

```bash
mr job
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

- [`mr jobs list`](../jobs/list.md)
- [`mr resource from-url`](../resource/from-url.md)
- [`mr admin`](../admin/index.md)
