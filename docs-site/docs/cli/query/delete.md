---
title: mr query delete
description: Delete a query by ID
sidebar_label: delete
---

# mr query delete

Delete a saved query by ID. Destructive: removes the database row
for the query. Any downstream references (saved dashboards, bookmarks)
should be updated separately. Deleting a nonexistent ID returns exit
code 1.

## Usage

```bash
mr query delete <id>
```

Positional arguments:

- `<id>`


## Examples

**Delete a query by ID**

```bash
mr query delete 42
```

**Delete and pipe the result to jq to confirm**

```bash
mr query delete 42 --json | jq .
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
- [`mr query create`](./create.md)
- [`mr queries list`](../queries/list.md)
