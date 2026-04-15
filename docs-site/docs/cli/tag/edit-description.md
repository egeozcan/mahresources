---
title: mr tag edit-description
description: Edit a tag's description
sidebar_label: edit-description
---

# mr tag edit-description

Update the description of an existing tag. Takes two positional
arguments: the tag ID and the new description. Passing an empty string
clears the description. Useful for annotating tags used across many
resources without renaming them.

## Usage

```bash
mr tag edit-description <id> <new-description>
```

Positional arguments:

- `<id>`
- `<new-description>`


## Examples

**Set a description on tag 42**

```bash
mr tag edit-description 42 "used for Q1 2026 scans"
```

**Clear the description by passing an empty string**

```bash
mr tag edit-description 42 ""
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
## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr tag edit-name`](./edit-name.md)
- [`mr tag get`](./get.md)
- [`mr tags list`](../tags/list.md)
