---
title: mr note-blocks reorder
description: Reorder note blocks
sidebar_label: reorder
---

# mr note-blocks reorder

Move specific blocks to new positions on their parent note. `--note-id`
and `--positions` are both required. `--positions` takes a JSON object
mapping block ID (as a string key) to its new fractional `position`
string. Only the listed blocks are moved; every other block on the
note keeps its current position. Fractional positions sort
lexicographically, so `"a" < "m" < "z"` — pick new values that slot
into the desired order.

After many reorders, positions can grow long; run `note-blocks
rebalance` to normalize them.

## Usage

    mr note-blocks reorder

## Examples

**Move block 10 to the top and block 11 to the bottom of note 42**

    mr note-blocks reorder --note-id 42 --positions '{"10":"a","11":"z"}'

**Move one block between two siblings using a midpoint string**

    mr note-blocks reorder --note-id 42 --positions '{"10":"m"}'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--note-id` | uint | `0` | Note ID (required) **(required)** |
| `--positions` | string | `` | Positions JSON map (required), e.g. '{"1":"a","2":"b"}' **(required)** |
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

- [`mr note-blocks rebalance`](./rebalance.md)
- [`mr note-blocks list`](./list.md)
- [`mr note-block update`](../note-block/update.md)
