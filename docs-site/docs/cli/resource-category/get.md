---
title: mr resource-category get
description: Get a resource category by ID
sidebar_label: get
---

# mr resource-category get

Get a resource category by ID and print its fields. The server has no
single-resource-category GET endpoint, so the CLI fetches the first page
of the list and filters in-process. Only the first 50 resource
categories are searched: beyond that, a category that exists is reported
as `resource category <id> not found`, so use
`resource-categories list --name <substring>` to locate it instead.

Output is a key/value table by default; pass the global `--json` flag to
emit those fields as JSON for scripting. The template slots, MetaSchema
and SectionConfig are not included; read them from
`resource-categories list --json`.

## Usage

```bash
mr resource-category get <id>
```

Positional arguments:

- `<id>`


## Examples

**Get a resource category by ID (table output)**

```bash
mr resource-category get 42
```

**Get as JSON and extract the name with jq**

```bash
mr resource-category get 42 --json | jq -r .Name
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
## Output

ResourceCategory object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource-category create`](./create.md)
- [`mr resource-category edit-name`](./edit-name.md)
- [`mr resource-categories list`](../resource-categories/list.md)
