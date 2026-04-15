---
title: mr plugin purge-data
description: Purge all data for a plugin
sidebar_label: purge-data
---

# mr plugin purge-data

Purge all key/value data a plugin has written through the plugin KV
API. Destructive: wipes every row the plugin has persisted in its
private KV tables. The plugin itself stays installed and its stored
settings values (written via `plugin settings`) are preserved. The
plugin must be disabled first; calling `purge-data` on an enabled
plugin returns a non-zero exit code.

This is the reset button for plugin KV state; use it when a plugin's
stored data is corrupt, stale, or no longer needed. There is no
confirmation prompt and no undo.

## Usage

```bash
mr plugin purge-data <name>
```

Positional arguments:

- `<name>`


## Examples

**Purge all KV data for a plugin by name**

```bash
mr plugin purge-data my-plugin
```

**Purge and confirm the JSON response**

```bash
mr plugin purge-data my-plugin --json | jq -e '.ok == true'
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

Object with name and ok=true on success

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr plugin disable`](./disable.md)
- [`mr plugin settings`](./settings.md)
- [`mr plugins list`](../plugins/list.md)
