---
title: mr user
description: Administer user accounts (admin only)
sidebar_label: user
---

# mr user

List, inspect, create, update, and delete user accounts. These commands target the admin user-management API and require an administrator identity. When the server runs without auth every request is an implicit admin, so they work there too.

## Usage

```bash
mr user
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

0 success; 1 error (not authenticated, insufficient role, validation error, or user not found)