---
title: mr user get
description: Show a single user account
sidebar_label: get
---

# mr user get

Fetch one user account by its numeric id and print its details. Useful before an update to confirm the current role and scope.

## Usage

```bash
mr user get <id>
```

Positional arguments:

- `<id>`


## Examples

**Show user 4**

```bash
mr user get 4
```

**As raw JSON**

```bash
mr user get 4 --json
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