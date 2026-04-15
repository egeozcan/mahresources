---
title: mr series edit
description: Edit a series
sidebar_label: edit
---

# mr series edit

Edit a series. `--name` is required on every call; `--meta` is optional
and takes a JSON string merged into the series meta. The slug is derived
from the original name at creation time and is not updated by this
command, so changing the name here leaves the slug untouched.

## Usage

```bash
mr series edit <id>
```

Positional arguments:

- `<id>`


## Examples

**Rename a series and set meta in one call**

```bash
mr series edit 42 --name "volume-1-final" --meta '{"season":"fall"}'
```

**Rename only (meta unchanged)**

```bash
mr series edit 42 --name "renamed"
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Series name (required) **(required)** |
| `--meta` | string | `` | Series metadata as JSON |
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

- [`mr series edit-name`](./edit-name.md)
- [`mr series get`](./get.md)
- [`mr series list`](./list.md)
