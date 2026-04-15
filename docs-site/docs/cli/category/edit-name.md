---
title: mr category edit-name
description: Edit a category's name
sidebar_label: edit-name
---

# mr category edit-name

Update the name of an existing Category. Takes two positional arguments:
the category ID and the new name. The name must remain unique across
categories; the server rejects duplicates. To rename and verify in one
step, chain with `mr category get <id> --json`.

## Usage

    mr category edit-name <id> <new-name>

Positional arguments:

- `<id>`
- `<new-name>`


## Examples

**Rename category 42**

    mr category edit-name 42 "Projects"

**Rename and confirm with a follow-up get**

    mr category edit-name 42 "renamed" && mr category get 42 --json | jq -r .Name


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

- [`mr category edit-description`](./edit-description.md)
- [`mr category get`](./get.md)
- [`mr categories list`](../categories/list.md)
