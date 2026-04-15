---
title: mr resource-category create
description: Create a new resource category
sidebar_label: create
---

# mr resource-category create

Create a new resource category. `--name` is required; all other flags
are optional, including a plain `--description`, presentation
fields (`--custom-header`, `--custom-sidebar`, `--custom-summary`,
`--custom-avatar`, `--custom-mrql-result`) and structural fields
(`--meta-schema`, `--section-config`). On success prints a confirmation
line with the new ID; pass the global `--json` flag to emit the full
record for scripting.

## Usage

```bash
mr resource-category create
```

## Examples

**Create a resource category with just a name**

```bash
mr resource-category create --name "Photos"
```

**Create with a description and capture the ID via jq**

```bash
ID=$(mr resource-category create --name "Scans" --description "scanned documents" --json | jq -r .ID)
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Resource category name (required) **(required)** |
| `--description` | string | `` | Resource category description |
| `--custom-header` | string | `` | Custom header HTML |
| `--custom-sidebar` | string | `` | Custom sidebar HTML |
| `--custom-summary` | string | `` | Custom summary HTML |
| `--custom-avatar` | string | `` | Custom avatar HTML |
| `--meta-schema` | string | `` | Meta schema JSON |
| `--section-config` | string | `` | JSON controlling which sections are visible on resource detail pages for this category |
| `--custom-mrql-result` | string | `` | Pongo2 template for rendering resources of this category in MRQL results |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Created ResourceCategory object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource-category get`](./get.md)
- [`mr resource-category edit-name`](./edit-name.md)
- [`mr resource-categories list`](../resource-categories/list.md)
