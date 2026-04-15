---
title: mr group get
description: Get a group by ID
sidebar_label: get
---

# mr group get

Get a group by ID and print its metadata. Fetches the full record
including the owner chain, category, tags, and any custom Meta JSON
object. Output is a key/value table by default; pass the global `--json`
flag to get the full record for scripting (related collections such as
`Tags`, `OwnResources`, `OwnNotes`, and `OwnGroups` are included).

## Usage

    mr group get <id>

Positional arguments:

- `<id>`


## Examples

**Get a group by ID (table output)**

    mr group get 42

**Get as JSON and extract a single field with jq**

    mr group get 42 --json | jq -r .Name


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

Group object with ID (uint), Name, Description, Meta (object), OwnerId, CategoryId, CreatedAt/UpdatedAt, plus related collections (Tags, OwnResources, OwnNotes, OwnGroups)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group create`](./create.md)
- [`mr group edit-name`](./edit-name.md)
- [`mr group parents`](./parents.md)
- [`mr group children`](./children.md)
