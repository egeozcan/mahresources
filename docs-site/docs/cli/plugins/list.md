---
title: mr plugins list
description: List plugins and management info
sidebar_label: list
---

# mr plugins list

Return every plugin installed on the server, regardless of whether it
is currently enabled. The response is a single array ordered by plugin
name. Each entry includes the plugin's `name`, `version`,
`description`, an `enabled` boolean, and a `settings` descriptor
array (or `null` when the plugin declares no settings). When a plugin
has stored configuration values, a `values` object is also present
keyed by setting name.

Plugin management info has a variable shape depending on what each
plugin reports, so `plugins list` always emits JSON; piping through
`jq` is the expected usage pattern.

## Usage

```bash
mr plugins list
```

## Examples

**Show every installed plugin as JSON**

```bash
mr plugins list
```

**Print just the names of enabled plugins**

```bash
mr plugins list | jq -r '.[] | select(.enabled == true) | .name'
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

Array of plugins; each entry has name, version, description, enabled, settings (nullable array of setting descriptors), and an optional values object holding stored configuration values

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr plugin enable`](../plugin/enable.md)
- [`mr plugin disable`](../plugin/disable.md)
- [`mr plugin settings`](../plugin/settings.md)
