---
title: mr note
description: Get, create, edit, delete, or share a note
sidebar_label: note
---

# mr note

Notes are free-form text records in mahresources. A Note has a name,
description, optional meta JSON, an optional owner group, an optional
note type (template), optional start/end dates, and many-to-many links
to Tags, Resources, and Groups. A Note may also carry a share token
that exposes it at `/s/<token>` for read-only public access.

Use the `note` subcommands to operate on a single note by ID: fetch the
full record, create a new one, edit the name/description/meta fields,
toggle sharing, or delete it. Use `notes list` to discover notes
matching filters, or the bulk subcommands under `notes` to mutate many
at once.

## Usage

    mr note

## Examples


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

- [`mr notes list`](../notes/list.md)
- [`mr groups list`](../groups/list.md)
