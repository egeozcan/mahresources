---
title: mr plugin schedules
description: List a plugin's recurring schedules
sidebar_label: schedules
---

# mr plugin schedules

List the recurring work one installed plugin has registered. A schedule
is declared by the plugin with `mah.schedule` and recorded when an
operator enables it, so this shows what the deployment has actually
stored rather than what the plugin file currently says.

Two fields carry the state that is easy to misread. `registered` is
false when the row exists but the plugin no longer declares that id,
which is what a disabled plugin, a renamed schedule and a deleted
`mah.schedule` call all look like; none of them run, and the row is kept
so that re-enabling or restoring the call resumes it with its history.
`owned` is false when the operator who enabled the plugin has since been
deleted, at which point the schedule has stopped rather than merely lost
its attribution, because every run executes as that operator and there
is no safe identity to fall back to. Naming a plugin the server does not
have returns an empty list rather than an error.

## Usage

```bash
mr plugin schedules <name>
```

Positional arguments:

- `<name>`


## Examples

**What a plugin has scheduled**

```bash
mr plugin schedules my-plugin
```

**Just the ids and when each is next due**

```bash
mr plugin schedules my-plugin --json | jq -r '.[] | "\(.scheduleId) \(.nextDueAt)"'
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

Array of schedule objects with scheduleId, everySeconds, overlap, nextDueAt, runs, lastStatus, owned and registered

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr plugin enable`](./enable.md)
- [`mr plugin disable`](./disable.md)
- [`mr plugins list`](../plugins/list.md)
