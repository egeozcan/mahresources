---
title: mr plugin enable
description: Enable a plugin
sidebar_label: enable
---

# mr plugin enable

Enable an installed plugin by name. Once enabled, the plugin's
registered shortcodes, event hooks, and UI injections become active on
the server until a matching `plugin disable` call runs. Enabling a
plugin that declares required settings will fail until those settings
have been written via `plugin settings`. Enabling an already-enabled
or unknown plugin name returns a non-zero exit code and an error
message from the server.

## Usage

    mr plugin enable <name>

Positional arguments:

- `<name>`


## Examples

**Enable a plugin by name**

    mr plugin enable example-plugin

**Enable and confirm via the JSON response**

    mr plugin enable example-plugin --json | jq -e '.enabled == true'


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

Object with name, enabled=true, and ok=true on success

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr plugin disable`](./disable.md)
- [`mr plugin settings`](./settings.md)
- [`mr plugins list`](../plugins/list.md)
