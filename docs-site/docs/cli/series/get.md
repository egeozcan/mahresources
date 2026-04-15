---
title: mr series get
description: Get a series by ID
sidebar_label: get
---

# mr series get

Get a series by ID and print its fields. Fetches the full record
including the slug, meta JSON, and the list of resources currently
attached to the series. Output is a key/value table by default; pass the
global `--json` flag to emit the raw record for scripting.

## Usage

    mr series get <id>

Positional arguments:

- `<id>`


## Examples

**Get a series by ID (table output)**

    mr series get 42

**Get as JSON and extract the name with jq**

    mr series get 42 --json | jq -r .Name


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
