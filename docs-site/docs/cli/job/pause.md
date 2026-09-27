---
title: mr job pause
description: Pause a job
sidebar_label: pause
---

# mr job pause

Suspend an in-flight download without cancelling it. `<id>` is the Job
id `jobs list` prints, or the legacy handle `job submit` returns as
`id`. Pause only works while the download is queued or running in the
server process that holds its transfer; the server answers HTTP 409
Conflict for a Job no transfer in that process belongs to, such as one
running in another server process, and rejects pause requests against
finished, cancelled, or already-paused jobs. The
transfer is cancelled, discarding the bytes received so far, and the
Job's state becomes `paused` until you call `job resume`, which starts
the download again from the beginning. To pause a download whichever
server process is running it, use the canonical command:
`mr job command <id> pause`.

Generic jobs (group exports, imports) cannot be paused -- their runners
are not re-entrant. Pause is intended for long URL fetches.

## Usage

```bash
mr job pause <id>
```

Positional arguments:

- `<id>`


## Examples

**Pause a specific download**

```bash
mr job pause 018f4db1-9b40-7f54-8f16-37a449bcf01d
```

**Pause every download currently running**

```bash
mr jobs list --kind remote-download --state running --json | jq -r '.jobs[].id' | xargs -I {} mr job pause {}
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

Object with status set to "paused" and canonicalJobId naming the paused Job

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr job resume`](./resume.md)
- [`mr job cancel`](./cancel.md)
- [`mr jobs list`](../jobs/list.md)
