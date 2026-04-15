---
title: mr relation-type edit-name
description: Edit a relation type's name
sidebar_label: edit-name
---

# mr relation-type edit-name

Replace a RelationType's `Name` field. Takes the relation-type ID and
the new name as positional arguments. Shorthand for `mr relation-type
edit --id <id> --name <value>` when name is the only change. Sends
`POST /v1/relationType/editName` and returns `{id, ok}` on success.
There is no `relation-type get`: to verify, re-read with
`mr relation-types list --name <substring>` and match the ID in jq.

## Usage

```bash
mr relation-type edit-name <id> <new-name>
```

Positional arguments:

- `<id>`
- `<new-name>`


## Examples

**Rename relation-type 5**

```bash
mr relation-type edit-name 5 "references"
```

**Rename and confirm via a filtered list**

```bash
mr relation-type edit-name 5 "contains" && \
    mr relation-types list --name "contains" --json | jq -r '.[] | select(.ID == 5) | .Name'
```


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

Status object with id (uint) and ok (bool)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr relation-type edit`](./edit.md)
- [`mr relation-type edit-description`](./edit-description.md)
- [`mr relation-types list`](../relation-types/list.md)
