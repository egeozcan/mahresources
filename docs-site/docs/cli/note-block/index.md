---
title: mr note-block
description: Get, create, update, or delete a note block
sidebar_label: note-block
---

# mr note-block

Note blocks are ordered, typed content units attached to a single Note
(similar to Notion's blocks). Each block has a type (`text`, `heading`,
`todos`, `gallery`, `references`, `table`, `calendar`, `divider`, plus
any plugin-registered types), a free-form `content` JSON payload whose
shape depends on the type, a free-form `state` JSON payload for runtime
UI/view state, and a fractional `position` string that defines its
order within the parent note.

Use the `note-block` subcommands to operate on a single block by ID:
fetch it, create a new one on a note, update its content or state,
delete it, or list the available block types. Use `note-blocks` (plural)
for per-note listing, reorder, and rebalance operations.

## Usage

    mr note-block

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

- [`mr note-blocks list`](../note-blocks/list.md)
- [`mr note`](../note/index.md)
- [`mr notes list`](../notes/list.md)
