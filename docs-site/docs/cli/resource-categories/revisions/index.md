---
title: mr resource-categories revisions
description: List, diff, label or restore Template Revisions
sidebar_label: revisions
---

# mr resource-categories revisions

Inspect and restore the Template Revisions of a Resource Category. Every save that
changes one of its templates, its MetaSchema or its SectionConfig records a
revision, so each subcommand takes the resource category ID with `--id` and works
on that resource category's history.

The subcommands are `list`, `show`, `diff`, `restore` and `label`. All of
them require an admin account, because history is visible only to those
who may edit the resource category.

## Usage

```bash
mr resource-categories revisions
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

- [`mr resource-category edit`](../../resource-category/edit.md)
- [`mr resource-category get`](../../resource-category/get.md)
