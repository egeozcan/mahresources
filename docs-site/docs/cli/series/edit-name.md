---
title: mr series edit-name
description: Edit a series name
sidebar_label: edit-name
---

# mr series edit-name

Update only the name of an existing series. Shorthand for `mr series
edit <id> --name <value>` when the name is the only change. Takes two
positional arguments: the series ID and the new name. The slug is
derived from the original name at creation time and is not changed by
this command.

## Usage

```bash
mr series edit-name <id> <new-name>
```

Positional arguments:

- `<id>`
- `<new-name>`


## Examples

**Rename series 42**

```bash
mr series edit-name 42 "volume-1-final"
```

**Rename and confirm with a follow-up get**

```bash
mr series edit-name 42 "renamed" && mr series get 42 --json | jq -r .Name
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
## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr series edit`](./edit.md)
- [`mr series get`](./get.md)
- [`mr series list`](./list.md)
