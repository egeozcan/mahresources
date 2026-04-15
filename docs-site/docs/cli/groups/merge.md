---
title: mr groups merge
description: Merge groups into a winner
sidebar_label: merge
---

# mr groups merge

Merge one or more "loser" Groups into a single "winner". The winner's
ID and fields are preserved; tags, owned resources, owned notes, and
m2m relations from the losers are moved onto the winner; the loser
records are then deleted. Use to consolidate duplicates after manual
review or deduplication.

Both flags are required: `--winner <id>` is a single ID, and
`--losers` is a comma-separated list of IDs to merge in.

## Usage

    mr groups merge

## Examples

**Merge groups 2 and 3 into winner 1**

    mr groups merge --winner 1 --losers 2,3


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--winner` | uint | `0` | Winning group ID (required) **(required)** |
| `--losers` | string | `` | Comma-separated loser group IDs (required) **(required)** |
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

- [`mr group get`](../group/get.md)
- [`mr groups delete`](./delete.md)
- [`mr groups list`](./list.md)
