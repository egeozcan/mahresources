---
title: mr plugins
description: List installed plugins
sidebar_label: plugins
---

# mr plugins

Discover and inspect the plugins installed on the mahresources server.
The plural `plugins` command group is read-only: use `plugins list` for
a full snapshot of every plugin the server knows about, including its
current `enabled` state and any stored setting values. For lifecycle
controls (enable, disable, configure, purge) on a single plugin, use
the singular `plugin` subcommands.

## Usage

    mr plugins

## Examples


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

- [`mr plugins list`](./list.md)
- [`mr plugin enable`](../plugin/enable.md)
- [`mr plugin settings`](../plugin/settings.md)
