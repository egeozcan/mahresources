---
title: mr group edit-meta
description: Edit a single metadata field by JSON path
sidebar_label: edit-meta
---

# mr group edit-meta

Edit a single metadata field by JSON path. Takes three positional
arguments: the Group ID, a dot-separated path (e.g. `address.city`),
and a JSON-literal value (e.g. `'"Berlin"'`, `42`, `'[1,2,3]'`,
`'{"nested":true}'`). The server deep-merges the value at the given
path onto the existing Meta object and returns the full merged Meta
in the response.

Values must be valid JSON literals — string values need to be quoted
twice (bash single quotes around a JSON-quoted string), as in the
examples below.

## Usage

```bash
mr group edit-meta <id> <path> <value>
```

Positional arguments:

- `<id>`
- `<path>`
- `<value>`


## Examples

**Set a top-level string value**

```bash
mr group edit-meta 5 status '"active"'
```

**Set a nested field**

```bash
mr group edit-meta 5 address.city '"Berlin"'
```

**Replace a field with an array**

```bash
mr group edit-meta 5 scores '[1,2,3]'
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

Status object with id (uint), ok (bool), and meta (object reflecting the merged Meta)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group get`](./get.md)
- [`mr groups add-meta`](../groups/add-meta.md)
- [`mr groups meta-keys`](../groups/meta-keys.md)
