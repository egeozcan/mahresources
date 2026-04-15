---
title: mr note-block delete
description: Delete a note block by ID
sidebar_label: delete
---

# mr note-block delete

Delete a note block by ID. Destructive: removes the database row.
Deleting a nonexistent ID returns exit code 1 with an HTTP 404 error.
Deleting a block does not affect its parent Note or sibling blocks;
to remove every block on a note, delete the note itself.

## Usage

    mr note-block delete <id>

Positional arguments:

- `<id>`


## Examples

**Delete a note block by ID**

    mr note-block delete 42

**Delete and pipe the response to jq to confirm**

    mr note-block delete 42 --json | jq .


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

- [`mr note-block get`](./get.md)
- [`mr note-blocks list`](../note-blocks/list.md)
- [`mr note delete`](../note/delete.md)
