---
title: mr note-block create
description: Create a new note block
sidebar_label: create
---

# mr note-block create

Create a new block attached to a Note. `--note-id` and `--type` are
required. Use `--content` to supply the block's content JSON (the exact
shape depends on the chosen type — see `note-block types` for the
default content schema of each built-in type). `--position` is optional;
when omitted the server assigns a position after the current last block.
The created record is returned; capture `.id` from JSON output for use
in follow-up commands.

## Usage

    mr note-block create

## Examples

**Create a text block on note 42**

    mr note-block create --note-id 42 --type text --content '{"text":"hello"}'

**Create a heading block with an explicit position**

    mr note-block create --note-id 42 --type heading --content '{"text":"Intro","level":2}' --position a


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--note-id` | uint | `0` | Note ID (required) **(required)** |
| `--type` | string | `` | Block type (required) **(required)** |
| `--content` | string | `{}` | Block content JSON |
| `--position` | string | `` | Block position |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Created NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-block types`](./types.md)
- [`mr note-block update`](./update.md)
- [`mr note-blocks list`](../note-blocks/list.md)
