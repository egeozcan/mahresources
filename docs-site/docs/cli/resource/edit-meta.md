---
title: mr resource edit-meta
description: Edit a single metadata field by JSON path
sidebar_label: edit-meta
---

# mr resource edit-meta

Edit a single metadata field at a dot-separated JSON path. Takes three
positional arguments: the resource ID, the path (e.g., `address.city`),
and a JSON literal value (e.g., `'"Berlin"'`, `42`, `'{"nested":"obj"}'`,
`'[1,2,3]'`). Creates intermediate path segments as needed and leaves
sibling keys at each level untouched.

## Usage

```bash
mr resource edit-meta <id> <path> <value>
```

Positional arguments:

- `<id>`
- `<path>`
- `<value>`


## Examples

**Set a top-level string field (note: shell-quoted JSON string)**

```bash
mr resource edit-meta 5 status '"active"'
```

**Set a nested numeric field (creates address.postalCode if missing)**

```bash
mr resource edit-meta 5 address.postalCode 10115
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

- [`mr resource edit`](./edit.md)
- [`mr resources add-meta`](../resources/add-meta.md)
- [`mr resources meta-keys`](../resources/meta-keys.md)
