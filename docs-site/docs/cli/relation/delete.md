---
title: mr relation delete
description: Delete a relation by ID
sidebar_label: delete
---

# mr relation delete

Delete a Relation by ID. Destructive: removes the link row entirely.
The two groups and the relation-type are unaffected. Deleting a
nonexistent ID returns exit code 1. To confirm the removal, re-fetch
either participating group with `mr group get <id> --json` and check
that the relation no longer appears in its `Relationships` array.

## Usage

    mr relation delete <id>

Positional arguments:

- `<id>`


## Examples

**Delete relation 7**

    mr relation delete 7

**Delete and pipe the result to jq**

    mr relation delete 7 --json | jq .


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
## Output

Status object with id

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr relation create`](./create.md)
- [`mr relation edit-name`](./edit-name.md)
- [`mr group get`](../group/get.md)
