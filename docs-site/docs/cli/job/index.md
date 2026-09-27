---
title: mr job
description: Control Jobs and submit legacy downloads
sidebar_label: job
---

# mr job

A download job fetches a remote URL and stores the result as a new
Resource. Each submission creates one job per URL; the server downloads
in the background while the queue tracks progress, pause/resume, and
retry state. Queue entries are ephemeral: they live in server memory and
do not persist across restarts. Every download is also a durable Job,
readable in the Job Center at `/jobs` and through `jobs list` until Job
retention removes it; exports and imports are durable Jobs as well.

Use the `job` subcommands to operate on a single job by ID: `submit`
new URLs, `cancel` an active job, `pause` / `resume` an in-flight
transfer, or `retry` a failed one. `cancel`, `pause`, `resume` and
`retry` take either the Job id that `jobs list` prints or the legacy
handle `job submit` returns as `id`. A legacy handle follows the
current Retry of its Job; a Job id always names the one Job. Downloads,
exports, imports, Resource Reduction computation and similarity
recomputes go through the legacy download controls. Any other Job, such
as a plugin command run, is sent the command of the same name that it
advertises, and the verb is refused when the Job does not offer it.
Either way the answer has the same shape. Use `jobs list` to discover
Job ids and their current `state`, and `job command` for any other
command a Job advertises.

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
- [`mr jobs get`](../jobs/get.md)
- [`mr resource from-url`](../resource/from-url.md)
- [`mr admin`](../admin/index.md)
