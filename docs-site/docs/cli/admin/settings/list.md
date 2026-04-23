---
title: mr admin settings list
description: List all runtime settings
sidebar_label: list
---

# mr admin settings list

List all runtime-editable settings with their current value, boot default, override status, and last-updated timestamp. Overridden settings show the effective value alongside the original boot default so you can see what changed.

Pass `--json` to emit the raw JSON array for scripting or to inspect fields like `minNumeric`, `maxNumeric`, and `allowZero`.

## Usage

```bash
mr admin settings list
```

## Examples

**Show all settings in a table**

```bash
mr admin settings list
```

**Emit raw JSON for scripting**

```bash
mr admin settings list --json
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

Array of setting objects with key, label, group, type, current, bootDefault, overridden, updatedAt, reason

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr admin settings get`](./get.md)
- [`mr admin settings set`](./set.md)
- [`mr admin settings reset`](./reset.md)
