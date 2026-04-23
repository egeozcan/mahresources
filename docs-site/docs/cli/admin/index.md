---
title: mr admin
description: Server administration commands
sidebar_label: admin
---

# mr admin

Server administration commands. The default subcommand is `stats`, which prints a full health and data overview. The `settings` subgroup lets you view and change runtime configuration overrides without restarting the server.

Run `mr admin stats --help` for the full stats flags, or `mr admin settings --help` for the settings subcommands.

## Usage

```bash
mr admin
```

## Examples

**Show server stats (same as `mr admin stats`)**

```bash
mr admin
```

**Show help for all subcommands**

```bash
mr admin --help
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data-only` | bool | `false` | Show data stats only |
| `--server-only` | bool | `false` | Show server stats only |
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

- [`mr admin stats`](./stats.md)
- [`mr admin settings list`](./settings/list.md)
