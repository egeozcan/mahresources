---
title: mr user list
description: List user accounts
sidebar_label: list
---

# mr user list

Show all user accounts with their id, username, role, scope group, and disabled state. Password hashes are never returned.

## Usage

```bash
mr user list
```

## Examples

**List all users**

```bash
mr user list
```

**As raw JSON**

```bash
mr user list --json
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--offset` | int | `0` | Number of users to skip |
| `--limit` | int | `0` | Maximum users to return (0 = server default) |
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