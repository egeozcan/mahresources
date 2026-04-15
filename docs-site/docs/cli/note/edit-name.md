---
title: mr note edit-name
description: Edit a note's name
sidebar_label: edit-name
---

# mr note edit-name

Update only the name of an existing note. Takes two positional
arguments: the note ID and the new name. Use this when renaming is the
only change; for multi-field edits, prefer a single request via the
server API.

## Usage

    mr note edit-name <id> <new-name>

Positional arguments:

- `<id>`
- `<new-name>`


## Examples

**Rename note 42**

    mr note edit-name 42 "renamed title"

**Rename and confirm with a follow-up get**

    mr note edit-name 42 "final draft" && mr note get 42 --json | jq -r .Name


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

- [`mr note edit-description`](./edit-description.md)
- [`mr note edit-meta`](./edit-meta.md)
- [`mr note get`](./get.md)
