---
title: mr relation-type edit
description: Edit a relation type
sidebar_label: edit
---

# mr relation-type edit

Edit fields on an existing RelationType. `--id` is required; any other
flag left unset keeps the existing value (partial update). `--name`
and `--description` replace those fields; `--reverse-name` replaces
the reverse label. `--from-category` and `--to-category` rewire the
allowed category pairing; use with caution, as existing relations
using this type may become inconsistent. Sends `POST
/v1/relationType/edit` and returns the full updated record.

## Usage

    mr relation-type edit

## Examples

**Rename a relation type and update its description**

    mr relation-type edit --id 5 --name "referenced-by" --description "backward link"

**Rewire the target category (relation-type 5 now points to category 7)**

    mr relation-type edit --id 5 --to-category 7


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--id` | uint | `0` | Relation type ID (required) **(required)** |
| `--name` | string | `` | Relation type name |
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

- [`mr relation-type edit-name`](./edit-name.md)
- [`mr relation-type edit-description`](./edit-description.md)
- [`mr relation-types list`](../relation-types/list.md)
