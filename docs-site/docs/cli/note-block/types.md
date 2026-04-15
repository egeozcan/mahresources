---
title: mr note-block types
description: Show available block types (text, table, calendar, etc.)
sidebar_label: types
---

# mr note-block types

List every block type the server knows about, including built-in types
(`text`, `heading`, `todos`, `gallery`, `references`, `table`,
`calendar`, `divider`) and any types registered by active plugins. Each
entry includes `defaultContent` and `defaultState` — the canonical
empty-payload shapes you should extend when creating a block of that
type. Useful for discovering the content/state schema a given type
expects before calling `note-block create` or `note-block update`.

## Usage

    mr note-block types

## Examples

**List all block types as a table (default)**

    mr note-block types

**List types as JSON and extract just the names**

    mr note-block types --json | jq -r '.[].type'


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

Array of block type descriptors, each with type (string), defaultContent (object), defaultState (object), and optional plugin metadata (label, icon, description, plugin, pluginName)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-block create`](./create.md)
- [`mr note-block update`](./update.md)
- [`mr note-block update-state`](./update-state.md)
