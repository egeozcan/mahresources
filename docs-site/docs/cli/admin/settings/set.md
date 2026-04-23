---
title: mr admin settings set
description: Override a runtime setting
sidebar_label: set
---

# mr admin settings set

Override a runtime setting. The override persists to the database and takes effect on the next use of the setting — no restart required. The command prints the updated setting view so you can confirm the new value.

Size values accept suffix notation (e.g., `1G`, `500M`, `2048K`). Duration values use Go's time.ParseDuration format (`30s`, `5m`, `2h`). Use `--reason` to record why the change was made; the reason is stored in the database and shown by `mr admin settings get`.

## Usage

```bash
mr admin settings set <key> <value>
```

Positional arguments:

- `<key>`
- `<value>`


## Examples

**Set max_upload_size to 2 GB**

```bash
mr admin settings set max_upload_size 2147483648 --reason "increase for video workflow"
```

**Set mrql query timeout**

```bash
mr admin settings set mrql_query_timeout 30s
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

Updated setting object with key, label, group, type, current, bootDefault, overridden, updatedAt, reason

## Exit Codes

0 on success; 1 on unknown key, invalid value, or error

## See Also

- [`mr admin settings reset`](./reset.md)
- [`mr admin settings get`](./get.md)
- [`mr admin settings list`](./list.md)
