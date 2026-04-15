---
title: mr job pause
description: Pause a job
sidebar_label: pause
---

# mr job pause

Suspend an in-flight download without cancelling it. Pause only works
while the job is pending or downloading; the server rejects pause
requests against finished, cancelled, or already-paused jobs. The
background goroutine stops after the current chunk and the job stays
in the queue with status `paused` until you call `job resume`.

Generic jobs (group exports, imports) cannot be paused — their runners
are not re-entrant. Pause is intended for long URL fetches.

## Usage

```bash
mr job pause <id>
```

Positional arguments:

- `<id>`


## Examples

**Pause a specific job**

```bash
mr job pause a1b2c3d4
```

**Pause every job currently downloading**

```bash
mr jobs list --json | jq -r '.jobs[] | select(.status == "downloading") | .id' | xargs -I {} mr job pause {}
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

Object with status set to "paused"

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr job resume`](./resume.md)
- [`mr job cancel`](./cancel.md)
- [`mr jobs list`](../jobs/list.md)
