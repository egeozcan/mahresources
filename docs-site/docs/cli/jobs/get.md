---
title: mr jobs get
description: Read Job detail and advertised commands
sidebar_label: get
---

# mr jobs get

Read one visible Job by UUID. The response includes its current state, owner,
version, timeline summary, outputs, and the commands the server currently
allows this account to run. Use a command key from this response with
`mr job command`; the CLI checks the advertised version and endpoint before
submitting it.

## Usage

```bash
mr jobs get <job-id>
```

Positional arguments:

- `<job-id>`


## Examples

**Inspect a visible Job and its current commands**

```bash
mr jobs get 018f4db1-9b40-7f54-8f16-37a449bcf01d --json
```

**Extract command keys that the server currently advertises**

```bash
mr jobs get 018f4db1-9b40-7f54-8f16-37a449bcf01d --json | jq -r '.commands[].key'
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

Canonical Job detail with currently advertised commands and outputs

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr jobs list`](./list.md)
- [`mr jobs timeline`](./timeline.md)
- [`mr job command`](../job/command.md)
