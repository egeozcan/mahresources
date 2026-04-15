---
title: mr note-block update-state
description: Update a note block's state
sidebar_label: update-state
---

# mr note-block update-state

Replace a block's `state` payload. Takes the block ID as a positional
argument and the new state as `--state` JSON. `state` is separate from
`content`: it holds runtime/UI state like which todo items are checked,
which gallery layout is selected, or a calendar's current view. The
shape depends on the block's type (see `note-block types` for default
state schemas). Sending `null` or an empty body returns an error: the
state column has a NOT NULL constraint.

## Usage

    mr note-block update-state <id>

Positional arguments:

- `<id>`


## Examples

**Mark a text block as "done" via a custom state field**

    mr note-block update-state 42 --state '{"done":true}'

**Check off a todo item (todos blocks use `{"checked":[itemId**

    mr note-block update-state 42 --state '{"checked":["task-1"]}'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--state` | string | `{}` | Block state JSON (required) **(required)** |
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

- [`mr note-block update`](./update.md)
- [`mr note-block get`](./get.md)
- [`mr note-block types`](./types.md)
