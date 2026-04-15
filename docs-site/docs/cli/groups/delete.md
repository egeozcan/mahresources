---
title: mr groups delete
description: Delete multiple groups
sidebar_label: delete
---

# mr groups delete

Bulk-delete Groups. Destructive: removes each selected Group row and
its direct join-table entries (tag links, m2m relations). Owned
children, resources, and notes are orphaned (their `OwnerId` becomes
null). Targets are selected via `--ids` (CSV of unsigned ints).

The current CLI has no dry-run; pipe `groups list --json | jq` first
if you need to preview targets, or use `groups merge` to consolidate
rather than destroy.

## Usage

    mr groups delete

## Examples

**Delete specific groups**

    mr groups delete --ids 42,43,44

**Delete the output of a filter query**

    mr groups list --tags 7 --json | jq -r 'map(.ID) | join(",")' | xargs -I {} mr groups delete --ids {}


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated group IDs to delete (required) **(required)** |
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

- [`mr group delete`](../group/delete.md)
- [`mr groups merge`](./merge.md)
- [`mr groups list`](./list.md)
