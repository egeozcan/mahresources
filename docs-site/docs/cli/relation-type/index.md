---
title: mr relation-type
description: Create, edit, or delete a relation type
sidebar_label: relation-type
---

# mr relation-type

A RelationType (`GroupRelationType`) defines the typed link allowed
between two Categories of Groups. Each relation-type has a `Name`
(e.g., "references", "contains", "depends-on"), an optional
`Description`, an optional `ReverseName` for reading the link
backwards, and references to `FromCategory` and `ToCategory`. When a
Relation is created with `mr relation create --relation-type-id <id>`,
the server enforces that the source group belongs to `FromCategory`
and the target group belongs to `ToCategory`.

Use the `relation-type` subcommands to operate on a single relation
type by ID: `create` defines a new typed link, `edit` updates any
field, `edit-name` and `edit-description` are scoped updates, and
`delete` removes the type. There is no `relation-type get`: to read a
relation-type back, use `mr relation-types list --name <substring>`
and filter by ID in jq, or fetch the full list.

## Usage

    mr relation-type

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

- [`mr relation-types list`](../relation-types/list.md)
- [`mr relation create`](../relation/create.md)
- [`mr category`](../category/index.md)
- [`mr categories list`](../categories/list.md)
