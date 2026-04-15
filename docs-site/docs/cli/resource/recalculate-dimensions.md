---
title: mr resource recalculate-dimensions
description: Recalculate resource dimensions
sidebar_label: recalculate-dimensions
---

# mr resource recalculate-dimensions

Re-read an image Resource's bytes and update its stored width and
height. Useful after external file edits or when the original ingest
path failed to decode dimensions. Does not modify the file content
itself; only updates the database record.

## Usage

```bash
mr resource recalculate-dimensions <id>
```

Positional arguments:

- `<id>`


## Examples

**Recalculate dimensions for a single resource**

```bash
mr resource recalculate-dimensions 42
```

**Pipe from a list query to bulk-recalculate**

```bash
mr resources list --content-type image/jpeg --json | jq -r '.[].id' | xargs -I {} mr resource recalculate-dimensions {}
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

- [`mr resource get`](./get.md)
- [`mr resource rotate`](./rotate.md)
- [`mr resources set-dimensions`](../resources/set-dimensions.md)
