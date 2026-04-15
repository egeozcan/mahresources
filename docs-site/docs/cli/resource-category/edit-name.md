---
title: mr resource-category edit-name
description: Edit a resource category's name
sidebar_label: edit-name
---

# mr resource-category edit-name

Update the name of an existing resource category. Takes two positional
arguments: the resource category ID and the new name. The name should
remain unique across resource categories. To rename and verify in one
step, chain with `mr resource-category get <id> --json`.

## Usage

```bash
mr resource-category edit-name <id> <new-name>
```

Positional arguments:

- `<id>`
- `<new-name>`


## Examples

**Rename resource category 42**

```bash
mr resource-category edit-name 42 "Photos"
```

**Rename and confirm with a follow-up get**

```bash
mr resource-category edit-name 42 "renamed" && mr resource-category get 42 --json | jq -r .Name
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

- [`mr resource-category edit-description`](./edit-description.md)
- [`mr resource-category get`](./get.md)
- [`mr resource-categories list`](../resource-categories/list.md)
