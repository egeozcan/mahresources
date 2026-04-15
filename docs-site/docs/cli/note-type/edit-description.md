---
title: mr note-type edit-description
description: Edit a note type's description
sidebar_label: edit-description
---

# mr note-type edit-description

Update only the description of an existing note type. Takes two
positional arguments: the note type ID and the new description.
Passing an empty string clears the description. Useful for annotating
a note type's intended use without touching its MetaSchema or rendering
fields. Returns `{"id":N,"ok":true}` on success.

## Usage

    mr note-type edit-description <id> <new-description>

Positional arguments:

- `<id>`
- `<new-description>`


## Examples

**Set a description on note type 1**

    mr note-type edit-description 1 "for weekly engineering standups"

**Clear the description by passing an empty string**

    mr note-type edit-description 1 ""


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

- [`mr note-type edit-name`](./edit-name.md)
- [`mr note-type edit`](./edit.md)
- [`mr note-type get`](./get.md)
