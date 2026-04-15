---
title: mr note-blocks list
description: List note blocks for a note
sidebar_label: list
---

# mr note-blocks list

List every block attached to a Note in position order. `--note-id` is
required; the server returns the full set (no pagination), ordered by
the fractional `position` string. Use this to inspect the current
layout before reordering, to dump a note's structured content to JSON
for processing, or to feed block IDs into downstream commands.

## Usage

    mr note-blocks list

## Examples

**List every block on note 42 (table output)**

    mr note-blocks list --note-id 42

**Get blocks as JSON and extract id + position pairs**

    mr note-blocks list --note-id 42 --json | jq -r '.[] | [.id, .position] | @tsv'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--note-id` | uint | `0` | Note ID (required) **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Array of NoteBlock objects with id, noteId, type, position, content, state, createdAt, updatedAt (ordered by position ascending)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-block get`](../note-block/get.md)
- [`mr note-blocks reorder`](./reorder.md)
- [`mr note-blocks rebalance`](./rebalance.md)
