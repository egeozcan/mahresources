---
title: mr group
description: Get, create, edit, delete, or clone a group
sidebar_label: group
---

# mr group

Groups are hierarchical collections in mahresources. A Group has a name,
description, optional meta JSON, an optional owner (the parent group),
an optional category, and many-to-many links to Resources, Notes, Tags,
and other Groups. The owner relationship forms a tree, so a Group can
also have child groups (descendants whose `OwnerId` points at this one).

Use the `group` subcommands to operate on a single group by ID: fetch
metadata, edit its name/description/meta, walk its ancestor chain or
direct children, clone it, or export/import a self-contained subtree as
a portable tar archive. Use `groups list` to discover groups matching
filters, or the bulk subcommands under `groups` to mutate many at once.

## Usage

    mr group

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

- [`mr groups list`](../groups/list.md)
- [`mr resources list`](../resources/list.md)
- [`mr tags list`](../tags/list.md)
