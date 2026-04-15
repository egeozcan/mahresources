---
title: mr groups list
description: List groups
sidebar_label: list
---

# mr groups list

List Groups, optionally filtered. Filter flags combine with AND.
Comma-separated ID lists on `--tags` and `--groups` match any of the
given IDs via the `?Add` query parameter. Date flags
(`--created-before`, `--created-after`) expect `YYYY-MM-DD`. Pagination
via the global `--page` flag (default page size 50).

Use `--owner-id=0` to restrict to root groups (no parent). The JSON
output is a flat array — use `group children <id>` for tree-structured
traversal.

## Usage

    mr groups list

## Examples

**List all groups (paged)**

    mr groups list

**Filter by name prefix**

    mr groups list --name "Trips"

**Filter by owner and tag**

    mr groups list --owner-id 5 --tags 3 --json | jq -r '.[].Name'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Filter by name |
| `--description` | string | `` | Filter by description |
| `--tags` | string | `` | Comma-separated tag IDs to filter by |
| `--groups` | string | `` | Comma-separated group IDs to filter by |
| `--owner-id` | uint | `0` | Filter by owner group ID |
| `--category-id` | uint | `0` | Filter by category ID |
| `--url` | string | `` | Filter by URL |
| `--created-before` | string | `` | Filter by creation date (before) |
| `--created-after` | string | `` | Filter by creation date (after) |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Array of Group objects with ID, Name, Description, Meta, OwnerId, CategoryId, CreatedAt/UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group get`](../group/get.md)
- [`mr group create`](../group/create.md)
- [`mr groups timeline`](./timeline.md)
