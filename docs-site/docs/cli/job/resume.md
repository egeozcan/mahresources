---
title: mr job resume
description: Resume a job
sidebar_label: resume
---

# mr job resume

Restart a previously paused download job. Resume only works against
jobs currently in the `paused` state — jobs that are pending, running,
finished, or cancelled return an error. The server opens a fresh HTTP
request, resets the progress counters, and marks the job `pending`;
the background worker picks it up on the next scheduler tick.

Because the server does not keep partial bytes across pauses, resume
effectively restarts the download from the beginning.

## Usage

    mr job resume <id>

Positional arguments:

- `<id>`


## Examples

**Resume a specific paused job**

    mr job resume a1b2c3d4

**Resume every paused job in one pass**

    mr jobs list --json | jq -r '.jobs[] | select(.status == "paused") | .id' | xargs -I {} mr job resume {}


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

Object with status set to "resumed"

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr job pause`](./pause.md)
- [`mr job cancel`](./cancel.md)
- [`mr jobs list`](../jobs/list.md)
