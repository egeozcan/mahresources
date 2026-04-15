---
title: mr group delete
description: Delete a group by ID
sidebar_label: delete
---

# mr group delete

Delete a Group by ID. Destructive: removes the Group row and its
direct join-table entries (tag links, m2m relations). Owned children,
resources, and notes are orphaned (their `OwnerId` becomes null) rather
than cascaded. Use `groups delete --ids=...` for bulk deletion, or
`groups merge` to consolidate rather than destroy.

## Usage

```bash
mr group delete <id>
```

Positional arguments:

- `<id>`


## Examples

**Delete a single group**

```bash
mr group delete 42
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

0 on success; 1 on any error

## See Also

- [`mr group create`](./create.md)
- [`mr groups delete`](../groups/delete.md)
- [`mr groups merge`](../groups/merge.md)
