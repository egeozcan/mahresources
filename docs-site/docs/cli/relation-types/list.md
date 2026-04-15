---
title: mr relation-types list
description: List relation types
sidebar_label: list
---

# mr relation-types list

List RelationTypes, optionally filtered. `--name` and `--description`
do substring matches on those fields. Pagination via the global
`--page` flag (default page size 50). Use the JSON output to feed
scripted workflows: look up a type ID by name and pass it to
`mr relation create --relation-type-id <id>`.

## Usage

    mr relation-types list

## Examples

**List all relation types (paged)**

    mr relation-types list

**Filter by name substring**

    mr relation-types list --name references

**JSON output + jq to extract the ID for a known name**

    mr relation-types list --name "depends-on" --json | jq -r '.[0].ID'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Filter by name |
| `--description` | string | `` | Filter by description |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Array of relation types with ID, Name, Description, FromCategoryId, ToCategoryId, CreatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr relation-type create`](../relation-type/create.md)
- [`mr relation-type edit`](../relation-type/edit.md)
- [`mr relation create`](../relation/create.md)
- [`mr categories list`](../categories/list.md)
