---
title: mr note-block create
description: Create a new note block
sidebar_label: create
---

# mr note-block create

Create a new block attached to a Note. `--note-id` and `--type` are
required, and `--content` matters more than its default suggests: the
CLI sends `{}` when the flag is omitted, so the server validates and
stores that rather than substituting the type's own default content.
`text` and `heading` reject an empty object (`text block content must
have a 'text' field`, `heading level must be 1-6`); the other built-in
types accept it and are then created empty. The exact content shape
depends on the chosen type; see `note-block types` for the default
content schema of each built-in type.

`--position` is optional; when omitted the server assigns a position
after the current last block. A `text` block that sorts first on the
note is kept in sync with the note's description: creating one rewrites
the description, and creating an empty one adopts the description as its
text. The created record is returned; capture `.id` from JSON output for
use in follow-up commands.

## Usage

```bash
mr note-block create
```

## Examples

**Create a text block on note 42**

```bash
mr note-block create --note-id 42 --type text --content '{"text":"hello"}'
```

**Create a heading block with an explicit position**

```bash
mr note-block create --note-id 42 --type heading --content '{"text":"Intro","level":2}' --position a
```


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

Created NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object), createdAt (RFC3339), updatedAt (RFC3339)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-block types`](./types.md)
- [`mr note-block update`](./update.md)
- [`mr note-blocks list`](../note-blocks/list.md)
