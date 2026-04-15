---
title: mr note-blocks rebalance
description: Rebalance note block positions
sidebar_label: rebalance
---

# mr note-blocks rebalance

Rewrite every block's `position` string on a note to evenly spaced,
compact values while preserving the current display order. Use this
as a cleanup step after heavy reordering, when fractional positions
have grown long (e.g. `"aaamzzz"`), or when you want a predictable
position layout before a batch of reorders. The block IDs, types,
content, and state are untouched.

## Usage

```bash
mr note-blocks rebalance
```

## Examples

**Rebalance all block positions on note 42**

```bash
mr note-blocks rebalance --note-id 42
```

**Rebalance**

```bash
mr note-blocks rebalance --note-id 42
mr note-blocks list --note-id 42 --json | jq -r '.[] | [.id, .position] | @tsv'
```


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
## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-blocks reorder`](./reorder.md)
- [`mr note-blocks list`](./list.md)
