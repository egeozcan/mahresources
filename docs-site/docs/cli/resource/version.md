---
title: mr resource version
description: Get a specific version by ID
sidebar_label: version
---

# mr resource version

Fetch metadata for a single version by its version ID. Returns the same
fields as `versions` but as a single key/value record. Useful when you
know the version ID and need its size or comment without a list call.

## Usage

```bash
mr resource version <version-id>
```

Positional arguments:

- `<version-id>`


## Examples

**Fetch a version by ID**

```bash
mr resource version 17
```

**Extract size via jq**

```bash
mr resource version 17 --json | jq -r .size
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

Version object with id, number, size, type, comment, created

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource versions`](./versions.md)
- [`mr resource version-download`](./version-download.md)
- [`mr resource version-restore`](./version-restore.md)
