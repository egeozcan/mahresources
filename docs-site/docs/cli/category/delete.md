---
title: mr category delete
description: Delete a category by ID
sidebar_label: delete
---

# mr category delete

Delete a Category by ID. Destructive: removes the category row. Groups
previously assigned to this category become uncategorized (the group
records themselves are preserved). Deleting a nonexistent ID is a no-op
on the server but still returns success.

## Usage

    mr category delete <id>

Positional arguments:

- `<id>`


## Examples

**Delete a category by ID**

    mr category delete 42

**Delete and pipe the result to jq to confirm the response shape**

    mr category delete 42 --json | jq .


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

0 on success; 1 on any error

## See Also

- [`mr category get`](./get.md)
- [`mr category create`](./create.md)
- [`mr categories list`](../categories/list.md)
