---
title: mr admin settings get
description: Show a single runtime setting by key
sidebar_label: get
---

# mr admin settings get

Show a single runtime setting by key. The output includes the effective current value, the boot-time default, whether an override is active, and when it was last changed.

Pass `--json` to emit the raw JSON object for scripting.

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

Single setting object with key, label, group, type, current, bootDefault, overridden, updatedAt, reason

## Exit Codes

0 on success; 1 on unknown key or error

## See Also

- [`mr admin settings list`](./list.md)
- [`mr admin settings set`](./set.md)
- [`mr admin settings reset`](./reset.md)
