---
title: mr job retry
description: Retry a failed job
sidebar_label: retry
---

# mr job retry

Re-queue a failed or cancelled download job for another attempt.
Retry only works against jobs in the `failed` or `cancelled` state;
the server rejects retry on jobs that are still active, paused, or
already completed. It also refuses a download whose stored address is
not an absolute http or https URL, which no retry can fetch.

One URL is transferred once at a time. While the server's queue still
holds the failed attempt and another download of the same URL is
pending, downloading, processing or paused there, this command is
refused with HTTP 409. Otherwise the retry is accepted, and if the URL
is downloading when the new attempt would start, it waits in the queue
with the phase `waiting` until that transfer ends. A retry from the Job
Center (`POST /v1/jobs/{id}/commands/retry`) is never refused for this:
it always waits.

The download ID you pass keeps working. For a download the Job Center
records, the retry is a new Job linked to the failed one, and the ID
moves to the new Job. The failed Job keeps its outcome, and its own
Job UUID keeps naming it; the response's `canonicalJobId` is the new
Job's UUID. A download from before the Job Center is retried in place:
its progress, error message and completion times are cleared, then the
worker fetches the URL again.

Useful when a transient network error blew up the first attempt.
Persistent failures need an updated URL, which means calling
`job submit` fresh rather than `job retry`.

## Usage

```bash
mr job retry <id>
```

Positional arguments:

- `<id>`


## Examples

**Retry a specific failed job**

```bash
mr job retry a1b2c3d4
```

**Retry every failed job in the queue**

```bash
mr jobs list --json | jq -r '.jobs[] | select(.status == "failed") | .id' | xargs -I {} mr job retry {}
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

Object with status set to "retrying"

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr job submit`](./submit.md)
- [`mr jobs list`](../jobs/list.md)
- [`mr job cancel`](./cancel.md)
