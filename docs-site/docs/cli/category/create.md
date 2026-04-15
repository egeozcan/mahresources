---
title: mr category create
description: Create a new category
sidebar_label: create
---

# mr category create

Create a new Category. `--name` is required; `--description` is optional
free-form text. The optional `--custom-header`, `--custom-sidebar`,
`--custom-summary`, `--custom-avatar`, and `--custom-mrql-result` flags
accept template or HTML strings applied to Groups assigned to this
category. `--meta-schema` and `--section-config` take JSON strings
controlling structured metadata and which sections render on group
detail pages. On success prints a confirmation line with the new ID;
pass the global `--json` flag to emit the full record for scripting.

## Usage

    mr category create

## Examples

**Create a category with just a name**

    mr category create --name "Project"

**Create with a description and capture the ID via jq**

    ID=$(mr category create --name "Location" --description "Places you know about" --json | jq -r .ID)


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Category name (required) **(required)** |
| `--description` | string | `` | Category description |
| `--custom-header` | string | `` | Custom header HTML |
| `--custom-sidebar` | string | `` | Custom sidebar HTML |
| `--custom-summary` | string | `` | Custom summary HTML |
| `--custom-avatar` | string | `` | Custom avatar HTML |
| `--meta-schema` | string | `` | Meta schema JSON |
| `--section-config` | string | `` | JSON controlling which sections are visible on group detail pages for this category |
| `--custom-mrql-result` | string | `` | Pongo2 template for rendering groups of this category in MRQL results |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Created Category object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr category get`](./get.md)
- [`mr category edit-name`](./edit-name.md)
- [`mr categories list`](../categories/list.md)
