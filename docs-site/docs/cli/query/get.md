---
title: mr query get
description: Get a query by ID
sidebar_label: get
---

# mr query get

Get a saved query by ID and print its metadata. Fetches the full
record including Name, Text (the SQL), Template, Description, and
created/updated timestamps. Output is a key/value table by default;
pass the global `--json` flag to get the full record for scripting.

## Usage

```bash
mr query get <id>
```

Positional arguments:

- `<id>`


## Examples

**Get a query by ID (table output)**

```bash
mr query get 42
```

**Get as JSON and extract the SQL text**

```bash
mr query get 42 --json | jq -r .Text
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

Query object with ID (uint), Name (string), Text (string), Template (string), Description (string), CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr query run`](./run.md)
- [`mr query edit-name`](./edit-name.md)
- [`mr query edit-description`](./edit-description.md)
- [`mr queries list`](../queries/list.md)
