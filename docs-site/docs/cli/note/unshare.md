---
title: mr note unshare
description: Remove the share token from a note
sidebar_label: unshare
---

# mr note unshare

Remove the share token from a note, invalidating any previous share
URL. Calling `unshare` on a note that is not currently shared is a
no-op from the client's perspective but still returns success. After
unsharing, subsequent `get` responses will omit the `shareToken`
field entirely.

## Usage

    mr note unshare <id>

Positional arguments:

- `<id>`


## Examples

**Unshare note 42**

    mr note unshare 42

**Unshare and confirm via JSON response**

    mr note unshare 42 --json | jq -e '.success == true'


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

Object with success (bool, true) on successful unshare

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note share`](./share.md)
- [`mr note get`](./get.md)
