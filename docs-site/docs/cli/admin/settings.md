---
title: mr admin settings
description: View and manage runtime configuration overrides
sidebar_label: settings
sidebar_position: 3
---

# mr admin settings

Runtime configuration management. See [Runtime Settings](../../configuration/runtime-settings.md) for the conceptual overview.

## list {#list}

```bash
mr admin settings list [--json]
```

Show all 11 runtime-editable settings with their current value, boot default, override status, and last-updated timestamp. Pass `--json` to emit the raw JSON array for scripting.

## get {#get}

```bash
mr admin settings get <key> [--json]
```

Show a single setting by key. The output includes the effective current value, the boot-time default, whether an override is active, and when it was last changed.

## set {#set}

```bash
mr admin settings set <key> <value> [--reason <text>]
```

Override a setting. Size values accept K/M/G/T suffixes (base 2). Duration
values use Go's `time.ParseDuration` format.

Out-of-bounds values exit nonzero with a stderr message describing the valid
range.

## reset {#reset}

```bash
mr admin settings reset <key> [--reason <text>]
```

Remove the override and revert to boot default.

## Examples

```bash
mr admin settings list
mr admin settings set max_upload_size 2G --reason "increase for video workflow"
mr admin settings set mrql_query_timeout 30s
mr admin settings reset max_upload_size
```

## Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr admin stats`](./stats.md)
- [Runtime Settings](../../configuration/runtime-settings.md)
