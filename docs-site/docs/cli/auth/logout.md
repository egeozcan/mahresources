---
title: mr auth logout
description: Remove the stored API token
sidebar_label: logout
---

# mr auth logout

Delete the locally stored API token for the current --server so this machine is no longer authenticated to it. Tokens stored for other servers are left intact. This does not revoke the token on the server; use `mr token revoke` to invalidate it everywhere.

## Usage

```bash
mr auth logout
```

## Examples

**Forget the stored credentials for the current server**

```bash
mr auth logout
```

**Logout is safe to run even when not logged in**

```bash
mr auth logout
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

0 success; 1 error (login failed, network error, or not authenticated)