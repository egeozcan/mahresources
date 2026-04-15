---
title: mr jobs
description: View the download job queue
sidebar_label: jobs
---

# mr jobs

The download queue is the server's in-memory list of URL download jobs. Each
job tracks a source URL, a status (pending, downloading, paused, completed,
failed, cancelled), progress counters, and the resulting Resource ID once
finished.

The plural `jobs` command group exposes read-only views of the queue. Use
`jobs list` for a full snapshot of every job the server is tracking. For
lifecycle controls (submit, pause, resume, retry, cancel) on a single job,
use the singular `job` subcommands.

## Usage

    mr jobs

## Examples


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

- [`mr job submit`](../job/submit.md)
- [`mr job cancel`](../job/cancel.md)
- [`mr resource from-url`](../resource/from-url.md)
