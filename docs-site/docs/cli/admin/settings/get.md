---
title: mr admin settings get
description: Show a single runtime setting by key
sidebar_label: get
---

# mr admin settings get

Show a single runtime setting by key. The output includes the effective current value, the boot-time default, whether an override is active, and when it was last changed.

Pass `--json` to emit the setting as a JSON object for scripting. It always carries `key`, `label`, `description`, `group`, `type`, `current`, `bootDefault` and `overridden`, adds `updatedAt` while an override is active and `reason` when the override recorded one. Secret settings also preserve `secret`, `configured` and `bootConfigured`; their current and boot values are null. The numeric bounds `minNumeric`, `maxNumeric` and `allowZero` are dropped here, while `admin settings list`, `set` and `reset` print the server's response unchanged and keep them.

The table describes a secret as `configured` or `not configured`, without printing the saved key.

## Usage

```bash
mr admin settings get <key>
```

Positional arguments:

- `<key>`


## Examples

**Show max_upload_size in table form**

```bash
mr admin settings get max_upload_size
```

**Get as JSON and extract the current value**

```bash
mr admin settings get max_upload_size --json | jq -r .current
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

Single setting object with key, label, description, group, type, current, bootDefault, overridden, updatedAt, reason; secret settings also include secret, configured and bootConfigured, with null current and bootDefault

## Exit Codes

0 on success; 1 on unknown key or error

## See Also

- [`mr admin settings list`](./list.md)
- [`mr admin settings set`](./set.md)
- [`mr admin settings reset`](./reset.md)
