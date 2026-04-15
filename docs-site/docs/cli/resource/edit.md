---
title: mr resource edit
description: Edit a resource
sidebar_label: edit
---

# mr resource edit

Edit fields on an existing resource. Any flag left unset keeps the
existing value (partial update). Collection flags (`--tags`, `--groups`,
`--notes`) take comma-separated ID lists and replace the current set;
`--meta` takes a JSON string merged onto existing meta.

## Usage

```bash
mr resource edit <id>
```

Positional arguments:

- `<id>`


## Examples

**Rename and update the description**

```bash
mr resource edit 42 --name "renamed" --description "new description"
```

**Attach tags 5 and 7**

```bash
mr resource edit 42 --tags 5,7
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Resource name |
| `--description` | string | `` | Resource description |
| `--tags` | string | `` | Comma-separated tag IDs |
| `--groups` | string | `` | Comma-separated group IDs |
| `--notes` | string | `` | Comma-separated note IDs |
| `--owner-id` | uint | `0` | Owner group ID |
| `--meta` | string | `` | Meta JSON string |
| `--category` | string | `` | Category |
| `--resource-category-id` | uint | `0` | Resource category ID |
| `--original-name` | string | `` | Original file name |
| `--original-location` | string | `` | Original file location |
| `--width` | uint | `0` | Width in pixels |
| `--height` | uint | `0` | Height in pixels |
| `--series-id` | uint | `0` | Series ID |
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

- [`mr resource get`](./get.md)
- [`mr resource upload`](./upload.md)
- [`mr resource versions`](./versions.md)
