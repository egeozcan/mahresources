---
title: mr group children
description: List child groups (tree children) of a group
sidebar_label: children
---

# mr group children

List the direct children of a Group as lightweight tree-node records.
Each node returns `id`, `name`, `categoryName`, `childCount` (the
number of grandchildren under that child), and `ownerId`. Returns
a JSON array ordered alphabetically by name. A group with no children
returns an empty array.

Field names on tree-node responses are lowercase (`id`, `name`), not
PascalCase — unlike full Group objects returned by `group get`.

## Usage

    mr group children <id>

Positional arguments:

- `<id>`


## Examples

**List the direct children of group 42**

    mr group children 42

**Extract child IDs as CSV**

    mr group children 42 --json | jq -r 'map(.id) | join(",")'


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

Array of GroupTreeNode objects with id (uint), name (string), categoryName (string), childCount (int), ownerId (uint or null)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group get`](./get.md)
- [`mr group parents`](./parents.md)
- [`mr groups list`](../groups/list.md)
