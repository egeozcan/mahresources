---
title: mr job resume
description: Resume a job
sidebar_label: resume
---

# mr job resume

Restart a previously paused download job. `<id>` is the Job id `jobs
list` prints, or the legacy handle `job submit` returns as `id`. Resume
only works against a paused download; a Job that is queued, running,
finished, or cancelled returns an error. The server opens a fresh HTTP
request and queues the Job again; the transfer starts when the
deployment's job budget has room.

Because the server does not keep partial bytes across pauses, resume
effectively restarts the download from the beginning.

## Usage

```bash
mr job resume <id>
```

Positional arguments:

- `<id>`


## Examples

**Resume a specific paused download**

```bash
mr job resume 018f4db1-9b40-7f54-8f16-37a449bcf01d
```

**Resume every visible Job that currently offers resume**

```bash
mr jobs list --command resume --json | jq -r '.jobs[].id' | xargs -I {} mr job resume {}
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

Object with status set to "resumed" and canonicalJobId naming the resumed Job

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr job pause`](./pause.md)
- [`mr job cancel`](./cancel.md)
- [`mr jobs list`](../jobs/list.md)
