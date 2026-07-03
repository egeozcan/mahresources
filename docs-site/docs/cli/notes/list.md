---
title: mr notes list
description: List notes
sidebar_label: list
---

# mr notes list

List Notes, optionally filtered. Filter flags combine with AND.
Comma-separated ID lists on `--tags` and `--groups` match any of the
given IDs. Date flags (`--created-before`, `--created-after`) expect
`YYYY-MM-DD`. The `--name` and `--description` flags match substrings.
Use `--owner-id` and `--note-type-id` to scope by owner group or note
type. Pagination is via the global `--page` flag (default page size 50).

`--mrql` applies an MRQL filter expression, with `type = "note"`
implied (the same expression the list-page filter bar accepts). It uses
the WHERE-clause grammar only — no `ORDER BY`, `LIMIT`, `GROUP BY`,
`SCOPE`, or `$name` parameters — and composes with the other filter
flags via AND. Example: `--mrql 'tags = "todo" AND created > -7d'`.

## Usage

```bash
mr notes list
```

## Examples

**List all notes (first page)**

```bash
mr notes list
```

**Filter by name substring and owner**

```bash
mr notes list --name meeting --owner-id 42
```

**Filter by tag + date**

```bash
mr notes list --tags 5 --created-after 2026-01-01 --json | jq -r '.[].Name'
```

**Filter with an MRQL expression (type = "note" implied)**

```bash
mr notes list --mrql 'tags = "todo" AND created > -7d'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Filter by name |
| `--description` | string | `` | Filter by description |
| `--tags` | string | `` | Comma-separated tag IDs to filter by |
| `--groups` | string | `` | Comma-separated group IDs to filter by |
| `--owner-id` | uint | `0` | Filter by owner group ID |
| `--note-type-id` | uint | `0` | Filter by note type ID |
| `--created-before` | string | `` | Filter by creation date (before) |
| `--created-after` | string | `` | Filter by creation date (after) |
| `--mrql` | string | `` | MRQL filter expression (type = "note" implied), e.g. 'tags = "todo" AND created &gt; -7d' |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Array of Note objects with ID, Name, Description, Meta, Tags, OwnerId, NoteTypeId, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note get`](../note/get.md)
- [`mr notes timeline`](./timeline.md)
- [`mr mrql`](../mrql/index.md)
