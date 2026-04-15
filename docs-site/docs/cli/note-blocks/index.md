---
title: mr note-blocks
description: List, reorder, or rebalance note blocks
sidebar_label: note-blocks
---

# mr note-blocks

Discover and reorganize the blocks attached to a Note. The `note-blocks`
subcommands operate on the full set of blocks owned by one parent note:
`list` returns every block in position order, `reorder` moves specific
blocks to new positions via an explicit `blockId -> position` map, and
`rebalance` normalizes every block's position string to clean, evenly
spaced values (useful after many reorders cause position strings to
grow long).

All commands require `--note-id` to scope to a single note. To mutate
an individual block's content, state, or type, use the singular
`note-block` subcommands.

## Usage

    mr note-blocks

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

- [`mr note-block get`](../note-block/get.md)
- [`mr note-block create`](../note-block/create.md)
- [`mr note get`](../note/get.md)
