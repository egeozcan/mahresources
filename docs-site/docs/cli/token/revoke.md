---
title: mr token revoke
description: Revoke one of your API tokens
sidebar_label: revoke
---

# mr token revoke

Invalidate an API token by its id so it can no longer authenticate. This affects every client using that token.

## Usage

```bash
mr token revoke <id>
```

Positional arguments:

- `<id>`


## Examples

**Revoke token 3**

```bash
mr token revoke 3
```

**Revoke after listing**

```bash
mr token revoke 5
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

0 success; 1 error (not authenticated, network error, or token not found)