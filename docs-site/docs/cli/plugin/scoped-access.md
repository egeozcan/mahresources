---
title: mr plugin scoped-access
description: Open or close a plugin to group-limited accounts
sidebar_label: scoped-access
---

# mr plugin scoped-access

Decide whether group-limited users and guests may reach one installed
plugin. This is a separate decision from enabling: enabling is the
operator's consent to what the plugin may do, while this is consent to
who may ask it to. It is off by default, so a confined account is
refused the plugin's pages, API endpoints, shortcodes, slots and
actions, and the actions it may not run are not offered to it either.
Guests are read-only, and running an action is a write, so a guest is
refused every action run however this is set: opening a plugin gives a
guest its pages, shortcodes and slots, and it is offered no actions
either way.

Pass the decision explicitly via the required `--allowed` flag; a bare
call would otherwise read as a revocation. Opening a plugin says
nothing about what it may then do on a confined caller's behalf,
because that caller's database access stays bound to its own group
subtree and role. Accounts with no group limit (admins, editors and
unscoped users) are unaffected either way. Naming a plugin the server
does not have returns a non-zero exit code and the server's message.

## Usage

```bash
mr plugin scoped-access <name>
```

Positional arguments:

- `<name>`


## Examples

**Let group-limited users and guests reach a plugin**

```bash
mr plugin scoped-access my-plugin --allowed=true
```

**Close it again and confirm via the JSON response**

```bash
mr plugin scoped-access my-plugin --allowed=false --json | jq -e '.allow_scoped_principals == false'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--allowed` | bool | `false` | Whether group-limited users and guests may reach this plugin (required) **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Object with name, allow_scoped_principals, and ok=true on success

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr plugin enable`](./enable.md)
- [`mr plugin disable`](./disable.md)
- [`mr plugins list`](../plugins/list.md)
