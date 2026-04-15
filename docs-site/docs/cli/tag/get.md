---
title: mr tag get
description: Get a tag by ID
sidebar_label: get
---

# mr tag get

Get a tag by ID and print its fields. The server has no single-tag GET
endpoint, so the CLI fetches the full tag list and filters in-process;
on large instances this is slower than a direct lookup would be. Output
is a key/value table by default; pass the global `--json` flag to emit
the raw record for scripting.

## Usage

    mr tag get <id>

Positional arguments:

- `<id>`


## Examples

**Get a tag by ID (table output)**

    mr tag get 42

**Get as JSON and extract the name with jq**

    mr tag get 42 --json | jq -r .Name


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

Tag object with ID (uint), Name (string), Description (string), CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr tag create`](./create.md)
- [`mr tag edit-name`](./edit-name.md)
- [`mr tags list`](../tags/list.md)
