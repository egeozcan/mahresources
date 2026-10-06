---
title: mr note-block delete
description: Delete a note block by ID
sidebar_label: delete
---

# mr note-block delete

Delete a note block by ID. `--note-id` is required and must be the note that
owns the block; a block that belongs to a different note is refused with an
HTTP 400 error and left in place. Destructive: removes the database row.
Deleting a nonexistent ID returns exit code 1 with an HTTP 404 error.
Sibling blocks are untouched, but deleting a `text` block re-syncs the
parent Note's description to whatever text block now sorts first. To
remove every block on a note, delete the note itself.

## Usage

```bash
mr note-block delete <id>
```

Positional arguments:

- `<id>`


## Examples

**Delete a note block by ID**

```bash
mr note-block delete 42 --note-id 7
```

**Delete**

```bash
mr note-block delete 42 --note-id 7 && mr note-blocks list --note-id 7
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--note-id` | uint | `0` | ID of the note that owns the block (required) **(required)** |
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
