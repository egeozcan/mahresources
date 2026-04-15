---
title: mr resources list
description: List resources
sidebar_label: list
---

# mr resources list

List Resources, optionally filtered. Filter flags combine with AND.
Comma-separated ID lists on `--tags`, `--groups`, `--notes` use the
`?Add` query parameter to match any of the given IDs. Date flags
(`--created-before`, `--created-after`) expect `YYYY-MM-DD`. Sort with
`--sort-by=field1,-field2` (prefix with `-` for descending). Pagination
via the global `--page` flag (default page size 50).

## Usage

    mr resources list

## Examples

**List all resources (paged)**

    mr resources list

**Filter by content type**

    mr resources list --content-type image/jpeg

**Filter by tag + date**

    mr resources list --tags 5 --created-after 2026-01-01 --json | jq -r '.[].Name'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Filter by name |
| `--description` | string | `` | Filter by description |
| `--content-type` | string | `` | Filter by content type |
| `--owner-id` | uint | `0` | Filter by owner group ID |
| `--tags` | string | `` | Comma-separated tag IDs to filter by |
| `--groups` | string | `` | Comma-separated group IDs to filter by |
| `--notes` | string | `` | Comma-separated note IDs to filter by |
| `--resource-category-id` | uint | `0` | Filter by resource category ID |
| `--created-before` | string | `` | Filter by creation date (before) |
| `--created-after` | string | `` | Filter by creation date (after) |
| `--min-width` | uint | `0` | Minimum width |
| `--min-height` | uint | `0` | Minimum height |
| `--max-width` | uint | `0` | Maximum width |
| `--max-height` | uint | `0` | Maximum height |
| `--hash` | string | `` | Filter by hash |
| `--original-name` | string | `` | Filter by original name |
| `--sort-by` | string | `` | Comma-separated sort fields |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Array of resources with id, name, content type, size, dimensions, owner id, created

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource get`](../resource/get.md)
- [`mr groups list`](../groups/list.md)
- [`mr mrql`](../mrql/index.md)
