---
title: mr resources merge
description: Merge resources into a winner
sidebar_label: merge
---

# mr resources merge

Merge one or more "loser" Resources into a single "winner". The
winner's bytes and ID are preserved; tags, groups, notes, and relations
from the losers are moved onto the winner; the loser records and their
file bytes are then deleted. Use to consolidate duplicates after
perceptual-hash detection or manual review.

## Usage

    mr resources merge

## Examples

**Merge resources 2 and 3 into winner 1**

    mr resources merge --winner 1 --losers 2,3

**Pipe duplicate IDs from a search**

    mr resources merge --winner 1 --losers $(mr resources list --hash abcd1234 --json | jq -r 'map(.id) | join(",")')


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--winner` | uint | `0` | Winning resource ID (required) **(required)** |
| `--losers` | string | `` | Comma-separated loser resource IDs (required) **(required)** |
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

- [`mr resource get`](../resource/get.md)
- [`mr resources delete`](./delete.md)
