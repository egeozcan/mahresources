---
title: mr categories revisions list
description: List the revisions of a category
sidebar_label: list
---

# mr categories revisions list

List the revisions of a category, newest first. `--id` is required.
The table shows the revision NUMBER (#1, #2, ...), the SOURCE that
recorded it (`create`, `form`, `api`, `cli`, `plugin`, `import`,
`restore` or `baseline`), the slots it CHANGED compared with the
previous revision, its LABEL, the USER ID and when it was CREATED. Pass
`--json` for the full array, including the `applied` list. With
`--quiet` the output is the revision numbers, one per line, which are the
numbers `show`, `diff`, `restore` and `label` take; it is not the revision
`id`.

## Usage

```bash
mr categories revisions list
```

## Examples

**List the revisions of a category**

```bash
mr categories revisions list --id 3
```

**Print only the revision numbers**

```bash
mr categories revisions list --id 3 --json | jq -r '.[].number'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--id` | uint | `0` | ID of the category **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Array of revision summaries, newest first, with id, number, carrierType, carrierId, createdAt, createdByUserId, label, source, applied, changedSlots and restoredFrom

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr categories revisions show`](./show.md)
- [`mr categories revisions diff`](./diff.md)
- [`mr category edit`](../../category/edit.md)
