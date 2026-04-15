---
title: mr category edit-description
description: Edit a category's description
sidebar_label: edit-description
---

# mr category edit-description

Update the description of an existing Category. Takes two positional
arguments: the category ID and the new description. Passing an empty
string clears the description. Useful for annotating categories with
guidance about what Groups belong under them without renaming.

## Usage

    mr category edit-description <id> <new-description>

Positional arguments:

- `<id>`
- `<new-description>`


## Examples

**Set a description on category 42**

    mr category edit-description 42 "places and venues"

**Clear the description by passing an empty string**

    mr category edit-description 42 ""


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

- [`mr category edit-name`](./edit-name.md)
- [`mr category get`](./get.md)
- [`mr categories list`](../categories/list.md)
