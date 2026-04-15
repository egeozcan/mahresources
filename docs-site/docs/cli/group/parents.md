---
title: mr group parents
description: List parent groups of a group
sidebar_label: parents
---

# mr group parents

Walk up the owner chain from a Group to its top-level ancestor. Returns
an array of Group objects ordered from outermost ancestor down to the
queried Group itself (so the last element is always the group you asked
about, and root groups return a single-element array containing just
themselves). The walk is bounded to 20 levels to defend against cycles
in corrupted data.

Use this to render breadcrumbs or to detect whether a group lives under
a particular root.

## Usage

```bash
mr group parents <id>
```

Positional arguments:

- `<id>`


## Examples

**Show the ancestor chain for group 42**

```bash
mr group parents 42
```

**Extract ancestor IDs as CSV**

```bash
mr group parents 42 --json | jq -r 'map(.ID) | join(",")'
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

Array of Group objects representing the ancestor chain (up to 20 levels deep), including the queried group itself

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr group get`](./get.md)
- [`mr group children`](./children.md)
- [`mr groups list`](../groups/list.md)
