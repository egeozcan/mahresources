---
title: mr user delete
description: Delete a user account
sidebar_label: delete
---

# mr user delete

Permanently delete a user account by its numeric id, removing its sessions and API tokens, and nulling the creator on any content they stamped (the content is kept). This cannot be undone. Deleting the last enabled administrator is refused with HTTP 409 Conflict.

## Usage

```bash
mr user delete <id>
```

Positional arguments:

- `<id>`


## Examples

**Delete user 4**

```bash
mr user delete 4
```

**Delete after listing**

```bash
mr user delete 9
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