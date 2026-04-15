---
title: mr resource-category edit-description
description: Edit a resource category's description
sidebar_label: edit-description
---

# mr resource-category edit-description

Update the description of an existing resource category. Takes two
positional arguments: the resource category ID and the new description.
Passing an empty string clears the description. Useful for annotating
categories used across many resources without renaming them.

## Usage

    mr resource-category edit-description <id> <new-description>

Positional arguments:

- `<id>`
- `<new-description>`


## Examples

**Set a description on resource category 42**

    mr resource-category edit-description 42 "high-resolution scans"

**Clear the description by passing an empty string**

    mr resource-category edit-description 42 ""


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

- [`mr resource-category edit-name`](./edit-name.md)
- [`mr resource-category get`](./get.md)
- [`mr resource-categories list`](../resource-categories/list.md)
