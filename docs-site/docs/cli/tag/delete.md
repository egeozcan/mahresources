---
title: mr tag delete
description: Delete a tag by ID
sidebar_label: delete
---

# mr tag delete

Delete a tag by ID. Destructive: removes the tag row and detaches it
from any Resources, Notes, or Groups it was attached to (the related
entities themselves are preserved). Deleting a nonexistent ID is a
no-op on the server but still returns success.

## Usage

    mr tag delete <id>

Positional arguments:

- `<id>`


## Examples

**Delete a tag by ID**

    mr tag delete 42

**Delete and pipe the result to jq to confirm the response shape**

    mr tag delete 42 --json | jq .


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

- [`mr tags delete`](../tags/delete.md)
- [`mr tag get`](./get.md)
- [`mr tag create`](./create.md)
