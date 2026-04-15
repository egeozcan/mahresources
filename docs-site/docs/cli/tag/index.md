---
title: mr tag
description: Get, create, edit, or delete a tag
sidebar_label: tag
---

# mr tag

Tags are lightweight labels attached to Resources, Notes, and Groups.
A Tag has a name and optional description; the name is the user-visible
handle. Tags are the primary way to categorize content across entity
types and are commonly used as filter selectors in list and timeline
commands.

Use the `tag` subcommands to operate on a single tag by ID: fetch it,
create a new one, rename or redescribe it, or delete it. Use
`tags list` to discover tags and `tags merge` to fold a tag's
relationships into another.

## Usage

    mr tag

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

- [`mr tags list`](../tags/list.md)
- [`mr tags merge`](../tags/merge.md)
- [`mr tags delete`](../tags/delete.md)
