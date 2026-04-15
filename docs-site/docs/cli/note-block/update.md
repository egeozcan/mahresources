---
title: mr note-block update
description: Update a note block's content
sidebar_label: update
---

# mr note-block update

Replace a block's `content` payload. Takes the block ID as a positional
argument and the new content as `--content` JSON. The content shape
must match the block's type (see `note-block types` for the default
content schema of each built-in type). This command does not touch the
block's `state`, `position`, or `type` — use `note-block update-state`
for state changes and `note-blocks reorder` for position changes.

## Usage

    mr note-block update <id>

Positional arguments:

- `<id>`


## Examples

**Update a text block's content**

    mr note-block update 42 --content '{"text":"new body"}'

**Update and print the updated record as JSON**

    mr note-block update 42 --content '{"text":"new body"}' --json | jq .


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--content` | string | `{}` | Block content JSON (required) **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Updated NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-block update-state`](./update-state.md)
- [`mr note-block get`](./get.md)
- [`mr note-block create`](./create.md)
