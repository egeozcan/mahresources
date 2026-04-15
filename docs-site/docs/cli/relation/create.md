---
title: mr relation create
description: Create a new group relation
sidebar_label: create
---

# mr relation create

Create a new Relation linking two Groups with a typed relationship.
`--from-group-id`, `--to-group-id`, and `--relation-type-id` are all
required. The referenced relation-type's `FromCategory` and
`ToCategory` must match the categories of the two groups; otherwise
the server rejects the request. `--name` and `--description` are
optional labels stored on the relation itself. Sends `POST /v1/relation`
and returns the persisted record.

## Usage

```bash
mr relation create
```

## Examples

**Create a relation linking group 3 to group 4 with relation-type 2**

```bash
mr relation create --from-group-id 3 --to-group-id 4 --relation-type-id 2
```

**Create a named relation with a description**

```bash
mr relation create --from-group-id 3 --to-group-id 4 --relation-type-id 2 \
    --name "directed-by" --description "Kubrick directed 2001"
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from-group-id` | uint | `0` | Source group ID (required) **(required)** |
| `--to-group-id` | uint | `0` | Target group ID (required) **(required)** |
| `--relation-type-id` | uint | `0` | Relation type ID (required) **(required)** |
| `--name` | string | `` | Relation name |
| `--description` | string | `` | Relation description |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Relation object with ID, Name, Description, FromGroupId, ToGroupId, RelationTypeId, CreatedAt/UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr relation delete`](./delete.md)
- [`mr relation edit-name`](./edit-name.md)
- [`mr relation-type create`](../relation-type/create.md)
- [`mr group get`](../group/get.md)
