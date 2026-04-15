---
title: mr tags merge
description: Merge tags into a winner
sidebar_label: merge
---

# mr tags merge

Merge one or more "loser" tags into a single "winner". The winner's ID
and name are preserved; Resources, Notes, and Groups previously tagged
with any loser are re-tagged with the winner; the loser tag rows are
then deleted. Use to consolidate duplicate or redundant tags (e.g.,
`photo` and `photos`) without losing associations.

## Usage

    mr tags merge

## Examples

**Merge tags 2 and 3 into winner 1**

    mr tags merge --winner 1 --losers 2,3

**Merge the result of a filter**

    mr tags merge --winner 1 --losers $(mr tags list --name "dup-" --json | jq -r 'map(.ID) | join(",")')


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--winner` | uint | `0` | Winning tag ID (required) **(required)** |
| `--losers` | string | `` | Comma-separated loser tag IDs (required) **(required)** |
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

- [`mr tags delete`](./delete.md)
- [`mr tags list`](./list.md)
- [`mr resources merge`](../resources/merge.md)
