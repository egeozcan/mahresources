---
title: mr query edit-name
description: Edit a query's name
sidebar_label: edit-name
---

# mr query edit-name

Update the name of an existing saved query. Query names are used by
`query run-by-name`, so renaming a query breaks callers that reference
it by the old name. Shorthand for a direct field update; does not
modify the query Text or Template.

## Usage

```bash
mr query edit-name <id> <value>
```

Positional arguments:

- `<id>`
- `<value>`


## Examples

**Rename query 42**

```bash
mr query edit-name 42 "count-resources-v2"
```

**Rename and confirm with a follow-up get**

```bash
mr query edit-name 42 "renamed" && mr query get 42 --json | jq -r .Name
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

- [`mr query get`](./get.md)
- [`mr query edit-description`](./edit-description.md)
- [`mr queries list`](../queries/list.md)
