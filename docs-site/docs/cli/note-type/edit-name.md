---
title: mr note-type edit-name
description: Edit a note type's name
sidebar_label: edit-name
---

# mr note-type edit-name

Update only the name of an existing note type. Takes two positional
arguments: the note type ID and the new name. Shorthand for
`mr note-type edit --id <id> --name <value>` when name is the only
change. Returns `{"id":N,"ok":true}` on success; chain with
`mr note-type get <id>` to inspect the renamed record.

## Usage

    mr note-type edit-name <id> <new-name>

Positional arguments:

- `<id>`
- `<new-name>`


## Examples

**Rename note type 1**

    mr note-type edit-name 1 "Team Meeting"

**Rename and confirm with a follow-up get**

    mr note-type edit-name 1 "renamed" && mr note-type get 1 --json | jq -r .Name


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

- [`mr note-type edit-description`](./edit-description.md)
- [`mr note-type edit`](./edit.md)
- [`mr note-type get`](./get.md)
