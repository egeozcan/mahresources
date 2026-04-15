---
title: mr plugin settings
description: Update plugin settings (pass JSON via --data)
sidebar_label: settings
---

# mr plugin settings

Write configuration values for an installed plugin. Pass the values as
a JSON object via the required `--data` flag; keys must match the
`name` fields declared in the plugin's settings descriptor (see the
`settings` array on `plugins list`). The server stores the decoded
object as the plugin's persisted values and returns `ok=true` on
success.

This command replaces the stored values wholesale — keys omitted from
the `--data` payload are not preserved. Run `plugins list --json` to
inspect the current `values` object before writing a new one.

## Usage

    mr plugin settings <name>

Positional arguments:

- `<name>`


## Examples

**Update a plugin's banner text**

    mr plugin settings my-plugin --data '{"banner_text":"Hello from CLI"}'

**Write multiple settings in one call**

    mr plugin settings my-plugin --data '{"banner_text":"Hi","show_banner":true}'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data` | string | `{}` | Plugin settings as JSON (required) **(required)** |
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

- [`mr plugin enable`](./enable.md)
- [`mr plugin purge-data`](./purge-data.md)
- [`mr plugins list`](../plugins/list.md)
