---
title: mr note share
description: Generate a share token for a note
sidebar_label: share
---

# mr note share

Generate a share token for a note, making it readable via the public
`/s/<token>` share URL without authentication. Calling `share` on a
note that is already shared rotates the token, invalidating any
previous share URL. The response contains both the raw token and the
relative share URL for convenience.

## Usage

```bash
mr note share <id>
```

Positional arguments:

- `<id>`


## Examples

**Share note 42 and print the share URL**

```bash
mr note share 42 --json | jq -r .shareUrl
```

**Share and capture just the token for use elsewhere**

```bash
TOKEN=$(mr note share 42 --json | jq -r .shareToken)
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

Object with shareToken (string) and shareUrl (string path beginning with /s/)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note unshare`](./unshare.md)
- [`mr note get`](./get.md)
- [`mr note create`](./create.md)
