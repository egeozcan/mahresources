---
title: mr note-type delete
description: Delete a note type by ID
sidebar_label: delete
---

# mr note-type delete

Delete a note type by ID. Destructive: removes the note type row. Notes
that referenced it keep their rows but lose the typed schema link, so
use with care on instances where Notes depend on the type's MetaSchema
for rendering. Deleting a nonexistent ID is a no-op on the server but
still returns success.

## Usage

    mr note-type delete <id>

Positional arguments:

- `<id>`


## Examples

**Delete a note type by ID**

    mr note-type delete 42

**Delete and pipe the result to jq to confirm the response shape**

    mr note-type delete 42 --json | jq .


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

- [`mr note-type get`](./get.md)
- [`mr note-type create`](./create.md)
- [`mr note-types list`](../note-types/list.md)
