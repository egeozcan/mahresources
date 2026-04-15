---
title: mr relation-type create
description: Create a new relation type
sidebar_label: create
---

# mr relation-type create

Create a new RelationType defining a typed link between two Categories.
`--name` is required. `--from-category` and `--to-category` take
Category IDs (not names); when set, the server enforces that relations
of this type link groups of those categories. `--description` is
free-form text shown in UIs. `--reverse-name` stores a readable label
for traversing the link in the opposite direction. Sends `POST
/v1/relationType` and returns the persisted record.

## Usage

    mr relation-type create

## Examples

**Create a basic relation type between two category IDs**

    mr relation-type create --name "references" --from-category 1 --to-category 2

**Create with a description and reverse-name**

    ID=$(mr relation-type create --name "depends-on" --description "A depends on B" \
        --reverse-name "depended-on-by" --from-category 1 --to-category 2 --json | jq -r '.ID')


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Relation type name (required) **(required)** |
| `--description` | string | `` | Relation type description |
| `--reverse-name` | string | `` | Reverse relation name |
| `--from-category` | uint | `0` | From category ID |
| `--to-category` | uint | `0` | To category ID |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

RelationType object with ID, Name, Description, FromCategoryId, ToCategoryId, BackRelationId, CreatedAt/UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr relation-type edit`](./edit.md)
- [`mr relation-types list`](../relation-types/list.md)
- [`mr relation create`](../relation/create.md)
- [`mr category create`](../category/create.md)
