---
title: mr queries list
description: List queries
sidebar_label: list
---

# mr queries list

List saved Queries, optionally filtered by name. Pagination is
controlled via the global `--page` flag (default page size 50). The
`--name` flag does a substring match on query names (SQL `LIKE`
under the hood). Use the global `--json` flag to retrieve the raw
array of query records for scripting; the default table output
truncates long Name/Description cells for readability.

## Usage

    mr queries list

## Examples

**List all queries (first page)**

    mr queries list

**Filter by a name substring**

    mr queries list --name "count"

**JSON + jq: print each query's ID and name**

    mr queries list --json | jq -r '.[] | "\(.ID)\t\(.Name)"'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Filter by name |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Array of query objects with ID, Name, Text, Template, Description, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr query get`](../query/get.md)
- [`mr query run`](../query/run.md)
- [`mr queries timeline`](./timeline.md)
