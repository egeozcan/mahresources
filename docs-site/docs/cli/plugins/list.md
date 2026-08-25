---
title: mr plugins list
description: List plugins and management info
sidebar_label: list
---

# mr plugins list

Return every plugin installed on the server, regardless of whether it
is currently enabled. The response is a single array ordered by the
plugin's directory name, which is usually but not necessarily its
declared `name`. Each entry includes the plugin's `name`, `version`,
`description`, an `enabled` boolean, and a `settings` descriptor
array (or `null` when the plugin declares no settings). When a plugin
has stored configuration values, a `values` object is also present
keyed by setting name.

Each entry also reports what the plugin is granted, which is the same
information the management page shows before you enable it. `legacy`
is `true` for a plugin that declares no manifest: it says nothing about
what it needs, so nothing is withheld from it, and `capabilities` is
correctly the full list. `api_version` is the plugin API generation a
manifest declares, and is absent for a legacy plugin.

`capabilities` is the effective set, not the literal declaration:
`db:write` implies `db:read`, so a plugin that declared only the former
lists both. `capability_labels` maps each capability to the human
sentence describing that power; prefer it over the slug when showing
capabilities to a person.

`network` is the declared egress allowlist. When it is absent or empty
the plugin may reach any public host. That is the broadest policy, not
the absence of network access. Private network addresses are blocked
regardless, unless `allow_private_hosts` is `true`, which a manifest may
only declare alongside a `network` list that names its hosts exactly.

`dependencies` lists plugins that must be enabled first, or enabling
this one is refused. `min_app_version` is informational only: it is
recorded and displayed, and never enforced.

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

**Print the human sentence for each capability a plugin is granted**

```bash
mr plugins list | jq -r '.[] | select(.name == "example-plugin") | .capability_labels[]'
```

**Find plugins that may reach any public host (no declared allowlist)**

```bash
mr plugins list | jq -r '.[] | select((.network // []) == []) | .name'
```

**Find plugins running without a manifest**

```bash
mr plugins list | jq -r '.[] | select(.legacy == true) | .name'
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

Array of plugins; each entry has name, version, description, enabled, legacy (bool), capabilities ([]string), capability_labels (object), allow_private_hosts (bool), settings (nullable array of setting descriptors), and the optional api_version (int), network ([]string), dependencies ([]string), min_app_version (string) and values (object of stored configuration values)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr plugin enable`](../plugin/enable.md)
- [`mr plugin disable`](../plugin/disable.md)
- [`mr plugin settings`](../plugin/settings.md)
