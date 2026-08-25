---
title: mr relation-type delete
description: Delete a relation type by ID
sidebar_label: delete
---

# mr relation-type delete

Delete a RelationType by ID. Destructive: removes the type row
entirely. Every Relation of this type is deleted with it, along with the
Relations of its paired back-relation type, in one transaction; inspect
affected groups with `mr group get <id> --json` after a delete. Deleting
a nonexistent ID returns exit code 1.

## Usage

```bash
mr relation-type delete <id>
```

Positional arguments:

- `<id>`


## Examples

**Delete relation-type 5**

```bash
mr relation-type delete 5
```

**Delete and pipe the result to jq**

```bash
mr relation-type delete 5 --json | jq .
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
## Output

Status object with id

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr relation-type create`](./create.md)
- [`mr relation-types list`](../relation-types/list.md)
- [`mr relation delete`](../relation/delete.md)
