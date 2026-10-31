---
title: mr resource-categories revisions diff
description: Diff two revisions, or a revision against now
sidebar_label: diff
---

# mr resource-categories revisions diff

Diff the templates of two revisions of a resource category. `--id` and `--from` are
required; `--to` defaults to the resource category's current state. Only fields
that differ are shown. Text output is a unified diff per field, with three
lines of context and `-` and `+` marking removed and added lines.
MetaSchema and AutoDetectRules are compared as
pretty-printed JSON with sorted keys, so key order never shows as a
change; text that does not parse is compared as raw text. SectionConfig is
always compared as the server resolves it, so configs that resolve to the
same sections show no change, while a config that turns a section off differs from `{}`. Under `--json` the server's hunks are printed unchanged.

## Usage

```bash
mr resource-categories revisions diff
```

## Examples

**Compare revision 1 with the current state**

```bash
mr resource-categories revisions diff --id 3 --from 1
```

**Compare two revisions as JSON**

```bash
mr resource-categories revisions diff --id 3 --from 1 --to 4 --json | jq '.slots[].slot'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--id` | uint | `0` | ID of the resource category **(required)** |
| `--from` | uint | `0` | Revision number for the left side **(required)** |
| `--to` | uint | `0` | Revision number for the right side (default: the current state) |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Object with carrierType, carrierId, from, optional to, and slots, each slot holding fields with equal, insert and delete hunks of lines; text mode prints a unified diff

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr resource-categories revisions list`](./list.md)
- [`mr resource-categories revisions show`](./show.md)
- [`mr resource-categories revisions restore`](./restore.md)
