---
title: mr resource-category delete
description: Delete a resource category by ID
sidebar_label: delete
---

# mr resource-category delete

Delete a resource category by ID. Destructive: removes the resource
category row. Resources that reference this category remain but lose
their category association. Deleting a nonexistent ID may still return
success at the server level.

## Usage

    mr resource-category delete <id>

Positional arguments:

- `<id>`


## Examples

**Delete a resource category by ID**

    mr resource-category delete 42

**Delete and pipe the result to jq to inspect the response**

    mr resource-category delete 42 --json | jq .


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

- [`mr resource-category get`](./get.md)
- [`mr resource-category create`](./create.md)
- [`mr resource-categories list`](../resource-categories/list.md)
