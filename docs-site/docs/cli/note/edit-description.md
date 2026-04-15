---
title: mr note edit-description
description: Edit a note's description
sidebar_label: edit-description
---

# mr note edit-description

Update only the description of an existing note. Takes two positional
arguments: the note ID and the new description. Passing an empty
string clears the description.

## Usage

    mr note edit-description <id> <new-description>

Positional arguments:

- `<id>`
- `<new-description>`


## Examples

**Set the description on note 42**

    mr note edit-description 42 "raw brainstorm, needs polish"

**Clear the description by passing an empty string**

    mr note edit-description 42 ""


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

- [`mr note edit-name`](./edit-name.md)
- [`mr note edit-meta`](./edit-meta.md)
- [`mr note get`](./get.md)
