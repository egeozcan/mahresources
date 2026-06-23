---
title: mr user update
description: Update a user account
sidebar_label: update
---

# mr user update

Update an existing user account. Only the flags you pass are changed; the rest are preserved by reading the current account first. Use --disabled to lock an account (revoking its sessions and tokens) and --enable to unlock it.

## Usage

```bash
mr user update <id>
```

Positional arguments:

- `<id>`


## Examples

**Promote user 4 to editor**

```bash
mr user update 4 --role editor
```

**Disable an account and reset its password**

```bash
mr user update 4 --disabled --password newpass
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--username` | string | `` | New username |
| `--password` | string | `` | New password (omit to keep the current one) |
| `--role` | string | `` | New role: admin, editor, user, or guest |
| `--display-name` | string | `` | New display name |
| `--scope-group` | uint | `0` | New scope group id |
| `--disabled` | bool | `false` | Disable the account (revokes its sessions and tokens) |
| `--enable` | bool | `false` | Re-enable a disabled account |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

0 success; 1 error (not authenticated, insufficient role, validation error, or user not found)