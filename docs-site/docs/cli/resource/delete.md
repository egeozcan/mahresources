---
title: mr resource delete
description: Delete a resource by ID
sidebar_label: delete
---

# mr resource delete

Delete a resource by ID. Destructive: removes both the database row and
the stored file bytes. Deleting a nonexistent ID returns exit code 1.

## Usage

```bash
mr resource delete <id>
```

Positional arguments:

- `<id>`


## Examples

**Delete a resource by ID**

```bash
mr resource delete 42
```

**Delete and pipe the result to jq to confirm the response**

```bash
mr resource delete 42 --json | jq .
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

- [`mr resource get`](./get.md)
- [`mr resources list`](../resources/list.md)
- [`mr resources delete`](../resources/delete.md)
