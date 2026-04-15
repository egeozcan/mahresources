---
title: mr resource-category
description: Get, create, edit, or delete a resource category
sidebar_label: resource-category
---

# mr resource-category

A ResourceCategory is a taxonomy label attached to Resources. It has a
name, optional description, and a range of optional presentation fields
(custom header, sidebar, summary, avatar, MRQL result template) plus a
MetaSchema and SectionConfig used to shape resource detail pages for
resources in this category. Resource categories are distinct from
Categories, which label Groups.

Use the `resource-category` subcommands to operate on a single category
by ID: fetch it, create a new one, rename or redescribe it, or delete
it. Use `resource-categories list` to discover categories matching
filters.

## Usage

    mr resource-category

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

- [`mr resource-categories list`](../resource-categories/list.md)
- [`mr resource get`](../resource/get.md)
- [`mr resources list`](../resources/list.md)
