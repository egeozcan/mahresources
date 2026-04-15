---
title: mr category
description: Get, create, edit, or delete a group category
sidebar_label: category
---

# mr category

Categories are labels that classify Groups (distinct from ResourceCategory
which labels Resources). A Category has a name, optional description, and
optional presentation fields (CustomHeader, CustomSidebar, CustomSummary,
CustomAvatar, CustomMRQLResult) plus a MetaSchema JSON that Groups assigned
to this category inherit for structured metadata.

Use the `category` subcommands to operate on a single Category by ID:
fetch it, create a new one, rename or redescribe it, or delete it. Use
`categories list` to discover categories and `categories timeline` to
view creation activity over time.

## Usage

    mr category

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

- [`mr categories list`](../categories/list.md)
- [`mr group`](../group/index.md)
- [`mr groups list`](../groups/list.md)
