---
title: mr relation edit-description
description: Edit a relation's description
sidebar_label: edit-description
---

# mr relation edit-description

Replace a Relation's `Description` field. Takes the relation ID and
the new description as positional arguments; pass an empty string to
clear. Sends `POST /v1/relation/editDescription` and returns
`{id, ok}` on success. There is no `relation get`: to verify, re-fetch
a participating group with `mr group get <id> --json` and read the
description from its `Relationships` array.

## Usage

```bash
mr relation edit-description <id> <new-description>
```

Positional arguments:

- `<id>`
- `<new-description>`


## Examples

**Set the description on relation 7**

```bash
mr relation edit-description 7 "confirmed by archival records"
```

**Clear the description by passing an empty string**

```bash
mr relation edit-description 7 ""
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

- [`mr relation create`](./create.md)
- [`mr relation edit-name`](./edit-name.md)
- [`mr group get`](../group/get.md)
