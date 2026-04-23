---
title: mr admin settings reset
description: Remove a runtime override and revert to boot default
sidebar_label: reset
---

# mr admin settings reset

Remove a runtime override and revert the setting to its boot-time default. The command prints the post-reset view so you can confirm the current value is back to the default.

Use `--reason` to record why the override was removed; the reason is stored in the database alongside the reset timestamp.

## Usage

```bash
mr admin settings reset <key>
```

Positional arguments:

- `<key>`


## Examples

**Reset max_upload_size to its boot default**

```bash
mr admin settings reset max_upload_size
```

**Reset with a reason for the audit log**

```bash
mr admin settings reset mrql_query_timeout --reason "back to default after testing"
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--reason` | string | `` | Free-text note recorded in the audit log |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Setting object after reset with key, label, group, type, current (equals bootDefault), bootDefault, overridden (false), updatedAt, reason

## Exit Codes

0 on success; 1 on unknown key or error

## See Also

- [`mr admin settings set`](./set.md)
- [`mr admin settings get`](./get.md)
- [`mr admin settings list`](./list.md)
