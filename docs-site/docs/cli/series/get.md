---
title: mr series get
description: Get a series by ID
sidebar_label: get
---

# mr series get

Get a series by ID and print its fields. Fetches the full record
including the slug, meta JSON, and the first 50 resources attached to
the series. The key/value table shows ID, name, slug, meta and
timestamps; pass the global `--json` flag for the raw record, which is
where the resource list appears.

## Usage

```bash
mr series get <id>
```

Positional arguments:

- `<id>`


## Examples

**Get a series by ID (table output)**

```bash
mr series get 42
```

**Get as JSON and extract the name with jq**

```bash
mr series get 42 --json | jq -r .Name
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

Series object with ID (uint), Name (string), Slug (string), Meta (object), Resources ([]Resource), CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr series list`](./list.md)
- [`mr series edit`](./edit.md)
- [`mr series delete`](./delete.md)
