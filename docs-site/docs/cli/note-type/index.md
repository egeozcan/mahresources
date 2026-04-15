---
title: mr note-type
description: Get, create, edit, or delete a note type
sidebar_label: note-type
---

# mr note-type

Note Types are typed schemas for Notes. A NoteType defines the shape of
a Note's metadata via a JSON Schema (`MetaSchema`) and may carry custom
rendering bits: `CustomHeader`, `CustomSidebar`, `CustomSummary`,
`CustomAvatar`, `CustomMRQLResult`, and a `SectionConfig` JSON toggle
for which sections appear on note detail pages. Typical examples are
"Meeting Minutes", "Code Review", or "Bug Report".

Use the `note-type` subcommands to operate on a single note type by ID:
fetch it, create a new one, edit it (whole record or scoped name /
description), or delete it. Use `note-types list` to discover the
available note types and feed their IDs into `note create --note-type-id`.

## Usage

    mr note-type

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

- [`mr note-types list`](../note-types/list.md)
- [`mr note create`](../note/create.md)
- [`mr notes list`](../notes/list.md)
