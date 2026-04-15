---
title: mr plugin
description: Enable, disable, or configure a plugin
sidebar_label: plugin
---

# mr plugin

Plugins are server-side extensions that register shortcodes, hook into
entity lifecycle events, or inject custom UI into the mahresources web
interface. Each plugin is identified by a unique name and reports its
version, human description, an `enabled` flag, and an optional settings
schema (a list of `{name, type, label, default}` descriptors the plugin
will read at runtime).

Use the `plugin` subcommands to operate on one plugin at a time by
name: `enable` / `disable` toggle activation, `settings` writes the
plugin's configuration values, and `purge-data` wipes the plugin's
persisted state. Use `plugins list` to discover the names of installed
plugins and inspect their current enablement and stored settings.

## Usage

```bash
mr plugin
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

- [`mr plugins list`](../plugins/list.md)
- [`mr plugin enable`](./enable.md)
- [`mr plugin disable`](./disable.md)
