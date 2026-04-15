---
title: mr query edit-description
description: Edit a query's description
sidebar_label: edit-description
---

# mr query edit-description

Update the description of an existing saved query. Passing an empty
string clears the description. Description is metadata only and does
not affect execution.

## Usage

```bash
mr query edit-description <id> <value>
```

Positional arguments:

- `<id>`
- `<value>`


## Examples

**Set the description on query 42**

```bash
mr query edit-description 42 "Counts resources grouped by content type"
```

**Clear the description by passing an empty string**

```bash
mr query edit-description 42 ""
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

- [`mr query get`](./get.md)
- [`mr query edit-name`](./edit-name.md)
- [`mr queries list`](../queries/list.md)
