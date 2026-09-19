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

A plugin that declares server commands requires a separate acknowledgement.
The first invocation without `--confirm-commands` exits non-zero and prints every
command, its exact argument display, and timeout. Review that list, then re-run
with `--confirm-commands` to record durable consent. The flag confirms all
commands declared by that plugin; command execution is not available with an
in-memory-only consent store. Adding a command or changing its positional argv,
timeout, or sensitive-parameter set requires this acknowledgement again.

Command processes run as the server service account and are not sandboxed. They
have unrestricted networking, including private and loopback addresses, and can
read anything the OS account can read, including sibling plugin exchange
folders. Plugin-supplied parameter values may be interpreted as flags or
otherwise alter program behavior, and the executable may itself be an
interpreter that executes scripts or code. The host launches argv without a
shell, but that does not make the invoked program safe. Executable basenames and
their helpers resolve only through the server's `PLUGIN_COMMAND_PATH`; pin that
path to minimal trusted absolute directories before enabling command plugins.

## Usage

```bash
mr plugin enable <name>
```

Positional arguments:

- `<name>`


## Examples

**Enable a plugin by name**

```bash
mr plugin enable my-plugin
```

**Enable and confirm via the JSON response**

```bash
mr plugin enable my-plugin --json | jq -e '.enabled == true'
```

**After reviewing a command plugin's refusal output**

```bash
mr plugin enable media-tools --confirm-commands
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--confirm-commands` | bool | `false` | Acknowledge and enable every command declared by this plugin |
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
