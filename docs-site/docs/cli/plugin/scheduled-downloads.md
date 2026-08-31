---
title: mr plugin scheduled-downloads
description: List a plugin's deferred downloads
sidebar_label: scheduled-downloads
---

# mr plugin scheduled-downloads

List the one-shot downloads a plugin deferred with `mah.download.submit`.
A deferred download is stored durably until the plugin scheduler submits it
to the in-memory download queue. Pending rows have not started yet;
submitted rows carry the queue `jobId`; failed and cancelled rows are
terminal.

Every row fires under the plugin name that submitted it, so a restart does
not turn it into an unrestricted host download. It also fires as the user
who submitted it and is re-validated at fire time. If that user is deleted before a pending row fires,
the row becomes `owned: false` and stops rather than falling back to an
administrator. Submitted, failed and cancelled rows keep their terminal
status even if their owner is later deleted.

Naming a plugin the server does not have returns an empty list rather than
an error.

## Usage

```bash
mr plugin scheduled-downloads <name>
```

Positional arguments:

- `<name>`


## Examples

**What a plugin has deferred**

```bash
mr plugin scheduled-downloads my-plugin
```

**Just pending URLs and their due times**

```bash
mr plugin scheduled-downloads my-plugin --json | jq -r '.[] | select(.status=="pending") | "\(.dueAt) \(.url)"'
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

Array of scheduled download objects with id, pluginName, url, dueAt, status, jobId, lastError, attempts, owned, createdAt, updatedAt and claimedAt while a scheduler tick holds the submit claim, in JSON mode; a table in human mode whose STATE column reports stopped (no owner) for ownerless pending rows that can no longer fire

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr plugin schedules`](./schedules.md)
- [`mr plugin enable`](./enable.md)
- [`mr plugin disable`](./disable.md)
