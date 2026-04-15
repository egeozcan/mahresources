---
title: mr note delete
description: Delete a note by ID
sidebar_label: delete
---

# mr note delete

Delete a note by ID. Destructive: removes the database row and all of
its tag/group/resource associations. Deleting a nonexistent ID returns
exit code 1 with an HTTP 404 error message.

## Usage

    mr note delete <id>

Positional arguments:

- `<id>`


## Examples

**Delete a note by ID**

    mr note delete 42

**Delete and pipe the response to jq to confirm**

    mr note delete 42 --json | jq .


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

- [`mr note get`](./get.md)
- [`mr notes delete`](../notes/delete.md)
- [`mr notes list`](../notes/list.md)
