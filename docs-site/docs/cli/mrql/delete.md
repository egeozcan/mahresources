---
title: mr mrql delete
description: Delete a saved MRQL query by ID
sidebar_label: delete
---

# mr mrql delete

Delete a saved MRQL query by numeric ID. Destructive: removes the
database row for the saved query. Any downstream references (bookmarks,
dashboards, or `[mrql saved="..."]` shortcodes) must be updated
separately — the server does not rewrite them. Deleting a nonexistent
ID returns exit code 1.

Unlike `mrql run`, the delete subcommand only accepts a numeric ID; pass
`mrql list --json | jq -r '.[] | select(.name == "...") | .id'` to
resolve a name to its ID first.

## Usage

```bash
mr mrql delete <id>
```

Positional arguments:

- `<id>`


## Examples

**Delete a saved query by ID**

```bash
mr mrql delete 42
```

**Delete and inspect the response with jq**

```bash
mr mrql delete 42 --json | jq .
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

Object with id (uint) of the deleted saved MRQL query

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr mrql save`](./save.md)
- [`mr mrql list`](./list.md)
- [`mr mrql run`](./run.md)
