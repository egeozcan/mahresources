---
title: mr relation
description: Create, edit, or delete a group relation
sidebar_label: relation
---

# mr relation

A Relation is a typed, directional link between two Groups. It has a
`FromGroupId`, a `ToGroupId`, and a `RelationTypeId` pointing at a
`relation-type` that defines the allowed category pairing and the
relationship's semantics. Relations may also carry an optional `Name`
and `Description`.

Use the `relation` subcommands to operate on a single relation by ID:
`create` links two groups, `edit-name` and `edit-description` update
its labels, and `delete` removes the link. There is no `relation list`
or `relation get`: to read a relation back, fetch a participating group
with `mr group get <id> --json` and inspect its `Relationships` array,
or query via `mr mrql`.

## Usage

    mr relation

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

- [`mr relation-type`](../relation-type/index.md)
- [`mr relation-types list`](../relation-types/list.md)
- [`mr group get`](../group/get.md)
- [`mr group children`](../group/children.md)
- [`mr group parents`](../group/parents.md)
