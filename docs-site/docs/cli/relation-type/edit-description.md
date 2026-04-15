---
title: mr relation-type edit-description
description: Edit a relation type's description
sidebar_label: edit-description
---

# mr relation-type edit-description

Replace a RelationType's `Description` field. Takes the relation-type
ID and the new description as positional arguments; pass an empty
string to clear. Shorthand for `mr relation-type edit --id <id>
--description <value>`. Sends `POST /v1/relationType/editDescription`
and returns `{id, ok}`. There is no `relation-type get`: to verify,
re-read with `mr relation-types list --name <substring>` and inspect
the `.Description` field in jq.

## Usage

```bash
mr relation-type edit-description <id> <new-description>
```

Positional arguments:

- `<id>`
- `<new-description>`


## Examples

**Set the description on relation-type 5**

```bash
mr relation-type edit-description 5 "references another record"
```

**Clear the description by passing an empty string**

```bash
mr relation-type edit-description 5 ""
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
- [`mr relation-type edit-name`](./edit-name.md)
- [`mr relation-types list`](../relation-types/list.md)
