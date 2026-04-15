---
title: mr resource edit-description
description: Edit a resource's description
sidebar_label: edit-description
---

# mr resource edit-description

Update only the description of an existing resource. Passing an empty
string clears the description. Shorthand for `mr resource edit <id> --description <value>`.

## Usage

    mr resource edit-description <id> <new-description>

Positional arguments:

- `<id>`
- `<new-description>`


## Examples

**Set the description on resource 42**

    mr resource edit-description 42 "scanned contract, Q1 2026"

**Clear the description by passing an empty string**

    mr resource edit-description 42 ""


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

- [`mr resource edit`](./edit.md)
- [`mr resource edit-name`](./edit-name.md)
- [`mr resource edit-meta`](./edit-meta.md)
