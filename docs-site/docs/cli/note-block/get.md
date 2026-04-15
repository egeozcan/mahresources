---
title: mr note-block get
description: Get a note block by ID
sidebar_label: get
---

# mr note-block get

Get a single note block by ID and print its fields. Fetches the full
record including the parent note ID, block type, fractional position,
content JSON, state JSON, and timestamps. Output is a key/value table
by default; pass the global `--json` flag to get the full record for
scripting.

## Usage

    mr note-block get <id>

Positional arguments:

- `<id>`


## Examples

**Get a note block by ID (table output)**

    mr note-block get 42

**Get as JSON and extract the block type**

    mr note-block get 42 --json | jq -r .type


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
## Output

NoteBlock object with id (uint), noteId (uint), type (string), position (string), content (object), state (object), createdAt (RFC3339), updatedAt (RFC3339)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-block update`](./update.md)
- [`mr note-block update-state`](./update-state.md)
- [`mr note-blocks list`](../note-blocks/list.md)
