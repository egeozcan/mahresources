---
title: mr relation-types
description: List relation types
sidebar_label: relation-types
---

# mr relation-types

Discover RelationTypes. The `relation-types` group currently exposes
only `list` for paginated, filterable reads. Use `relation-type`
(singular) for create/edit/delete operations on a specific type.

List results power downstream workflows: pipe `relation-types list
--json` into jq to pick an ID by name, then pass it to `mr relation
create --relation-type-id <id>` when linking two groups.

## Usage

    mr relation-types

## Examples


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

- [`mr relation-type create`](../relation-type/create.md)
- [`mr relation create`](../relation/create.md)
- [`mr categories list`](../categories/list.md)
