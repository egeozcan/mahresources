---
title: mr plugin disable
description: Disable a plugin
sidebar_label: disable
---

# mr plugin disable

Disable an installed plugin by name. Once disabled, the plugin stops
contributing shortcodes, hooks, and UI injections, but its stored
settings values and persisted KV data are preserved (use `plugin
purge-data` to remove the KV data). Disabling a plugin that is
already disabled is idempotent and returns `ok`.

## Usage

    mr plugin disable <name>

Positional arguments:

- `<name>`


## Examples

**Disable a plugin by name**

    mr plugin disable example-plugin

**Disable and confirm via the JSON response**

    mr plugin disable example-plugin --json | jq -e '.enabled == false'


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

Object with name, enabled=false, and ok=true on success

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr plugin enable`](./enable.md)
- [`mr plugin purge-data`](./purge-data.md)
- [`mr plugins list`](../plugins/list.md)
