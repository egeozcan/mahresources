---
title: mr plugin schedule-run
description: Run one of a plugin's schedules immediately
sidebar_label: schedule-run
---

# mr plugin schedule-run

Run one of a plugin's schedules straight away, without waiting for it to
come due. This is the supported way to exercise a schedule you are
developing: the alternatives are editing `next_due_at` in the database
directly, or waiting out an interval that may be an hour.

The command returns as soon as the run has *started*, not when it has
finished. A schedule handler may run for the full asynchronous job
allowance, so the request is not held open for it; the run reports
itself through the same job events as any other plugin job, and the
outcome lands on the row, where `mr plugin schedules` will show it as
`lastStatus` with an incremented `runs`. Starting takes at most the
10-second dispatch wait: the run needs its plugin to be free of other
background work, one of the server's plugin job slots and room in the
deployment's job budget.

Two things are deliberately unchanged by a manual run. It executes as
the operator who enabled the plugin, exactly as a scheduled run does,
rather than as whoever asked for it. And `nextDueAt` does not move: this
is an extra run, not a re-phasing, so the schedule stays on the cadence
it was already on.

A run is refused rather than started in five cases, each with its own
message and a non-zero exit. There is no such stored schedule. The
plugin no longer declares that id, which is what a disabled plugin and a
renamed schedule both look like. The row has no owner, so the schedule
has stopped. A run is already in flight, which is the `skip` overlap
policy doing exactly what it promises. Or the run could not start within
the dispatch wait because the plugin, the job slots or the job budget
stayed busy: nothing ran, nothing is recorded on the row and no job is
kept, so running the command again later is safe.

## Usage

```bash
mr plugin schedule-run <name> <schedule-id>
```

Positional arguments:

- `<name>`
- `<schedule-id>`


## Examples

**Run a schedule you are developing**

```bash
mr plugin schedule-run my-plugin nightly-rollup
```

**Start it**

```bash
mr plugin schedule-run my-plugin nightly-rollup
mr plugin schedules my-plugin --json | jq -r '.[] | select(.scheduleId=="nightly-rollup") | .lastStatus'
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

Object with ok (bool), name (string), scheduleId (string) and started (bool)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr plugin schedules`](./schedules.md)
- [`mr plugin enable`](./enable.md)
- [`mr plugin disable`](./disable.md)
