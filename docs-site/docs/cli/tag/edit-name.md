---
title: mr tag edit-name
description: Edit a tag's name
sidebar_label: edit-name
---

# mr tag edit-name

Update the name of an existing tag. Takes two positional arguments: the
tag ID and the new name. The name must remain unique across tags; the
server rejects duplicates. To rename and verify in one step, chain with
`mr tag get <id> --json`.

## Usage

    mr tag edit-name <id> <new-name>

Positional arguments:

- `<id>`
- `<new-name>`


## Examples

**Rename tag 42**

    mr tag edit-name 42 "important"

**Rename and confirm with a follow-up get**

    mr tag edit-name 42 "renamed" && mr tag get 42 --json | jq -r .Name


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

- [`mr tag edit-description`](./edit-description.md)
- [`mr tag get`](./get.md)
- [`mr tags list`](../tags/list.md)
