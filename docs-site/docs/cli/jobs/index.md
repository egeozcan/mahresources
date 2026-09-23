---
title: mr jobs
description: Browse and summarize background Jobs
sidebar_label: jobs
---

# mr jobs

Browse durable background Jobs with `list`, `get`, and `timeline`, and
aggregate them with `summary`. A Job keeps its identity, owner, state, events,
and outputs after execution finishes, subject to the server's retention
settings. The server decides which Jobs and controls the current account can
see.

`job submit` remains the compatibility command for remote downloads. The
singular `job cancel`, `pause`, `resume`, and `retry` commands also remain
available for older clients and scripts. Use `job command` or
`job bulk-command` for controls currently advertised by canonical Job detail.

## Usage

```bash
mr jobs
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

- [`mr job submit`](../job/submit.md)
- [`mr job command`](../job/command.md)
- [`mr jobs get`](./get.md)
- [`mr jobs summary`](./summary/index.md)
