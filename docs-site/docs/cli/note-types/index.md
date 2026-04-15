---
title: mr note-types
description: List note types
sidebar_label: note-types
---

# mr note-types

Discover Note Types, the typed schemas assigned to Notes. The
`note-types` subcommand currently exposes `list` for filtered queries
(with pagination via the global `--page` flag). Pipe `note-types list
--json` through `jq` when you need to derive IDs to feed into
`note create --note-type-id`.

Singular operations (get, create, edit, delete) live under the sibling
`note-type` command.

## Usage

    mr note-types

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

- [`mr note-type get`](../note-type/get.md)
- [`mr notes list`](../notes/list.md)
- [`mr note create`](../note/create.md)
