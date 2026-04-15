---
title: mr relation edit-name
description: Edit a relation's name
sidebar_label: edit-name
---

# mr relation edit-name

Replace a Relation's `Name` field. Takes the relation ID and the new
name as positional arguments. Sends `POST /v1/relation/editName` and
returns `{id, ok}` on success. There is no `relation get`: to verify,
re-fetch a participating group with `mr group get <id> --json` and
read the name from its `Relationships` array.

## Usage

```bash
mr relation edit-name <id> <new-name>
```

Positional arguments:

- `<id>`
- `<new-name>`


## Examples

**Rename relation 7**

```bash
mr relation edit-name 7 "directed-by"
```

**Rename and confirm via the source group**

```bash
mr relation edit-name 7 "produced-by" && \
    mr group get 3 --json | jq -r '.Relationships[] | select(.ID == 7) | .Name'
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
- [`mr relation edit-description`](./edit-description.md)
- [`mr group get`](../group/get.md)
