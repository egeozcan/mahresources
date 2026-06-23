---
title: mr user create
description: Create a user account
sidebar_label: create
---

# mr user create

Create a new user account with a username, password, and role (admin, editor, user, or guest). Guests require a scope group; users may optionally have one; admins and editors must not.

## Usage

```bash
mr user create
```

## Examples

**Create an editor**

```bash
mr user create --username alice --password s3cret --role editor
```

**Create a guest confined to group 7**

```bash
mr user create --username bob --password s3cret --role guest --scope-group 7
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--username` | string | `` | Username (required, unique) **(required)** |
| `--password` | string | `` | Password (required) **(required)** |
| `--role` | string | `` | Role: admin, editor, user, or guest (required) **(required)** |
| `--display-name` | string | `` | Optional display name |
| `--scope-group` | uint | `0` | Scope group id (required for guest, optional for user) |
| `--disabled` | bool | `false` | Create the account disabled |
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