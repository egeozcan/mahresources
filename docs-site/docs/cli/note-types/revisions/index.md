---
title: mr note-types revisions
description: List, diff, label or restore Template Revisions
sidebar_label: revisions
---

# mr note-types revisions

Inspect and restore the Template Revisions of a Note Type. Every save that
changes one of its templates, its MetaSchema or its SectionConfig records a
revision, so each subcommand takes the note type ID with `--id` and works
on that note type's history.

The subcommands are `list`, `show`, `diff`, `restore` and `label`. All of
them require an editor or admin account, because history is visible only to those
who may edit the note type.

## Usage

```bash
mr note-types revisions
```

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

- [`mr note-types revisions list`](./list.md)
- [`mr note-type edit`](../../note-type/edit.md)
- [`mr note-type get`](../../note-type/get.md)
