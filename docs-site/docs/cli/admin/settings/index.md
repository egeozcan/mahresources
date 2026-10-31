---
title: mr admin settings
description: View and manage runtime configuration overrides
sidebar_label: settings
---

# mr admin settings

View and manage runtime configuration overrides. Overrides persist to the database and take effect immediately without restarting the server.

Use `list` to see all settings, `get` to inspect one, `set` to apply an override, and `reset` to revert a key to its boot-time default.

AI provider keys are write-only settings: reads show whether a key is configured, and never return its value. An empty override clears a key and suppresses its environment fallback; `reset` restores that fallback. Use `/admin/settings` to save a generation profile's provider, model and limits together.

## Usage

```bash
mr admin settings
```

## Examples

**Show all settings in a table**

```bash
mr admin settings list
```

**Get a single setting**

```bash
mr admin settings get max_upload_size
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

- [`mr admin stats`](../stats.md)
- [`mr admin settings list`](./list.md)
- [`mr admin settings set`](./set.md)
