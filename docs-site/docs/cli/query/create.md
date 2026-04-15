---
title: mr query create
description: Create a new query
sidebar_label: create
---

# mr query create

Create a new saved query. Requires `--name` (unique label) and
`--text` (the SQL body). `--template` is optional and lets you embed
a Pongo2 template that receives the query's result rows for custom
rendering in the web UI. Query Text runs against a read-only handle
when executed; writes to the database via `query run` are rejected.

## Usage

```bash
mr query create
```

## Examples

**Create a minimal query**

```bash
mr query create --name "count-resources" --text "select count(*) as n from resources"
```

**Create with a template for custom display**

```bash
mr query create --name "recent-notes" --text "select id, name from notes order by created_at desc limit 10" --template "{{ rows|length }} rows"
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Query name (required) **(required)** |
| `--text` | string | `` | Query text/SQL (required) **(required)** |
| `--template` | string | `` | Query template |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Created query object with ID, Name, Text, Template, Description, CreatedAt, UpdatedAt

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr query get`](./get.md)
- [`mr query run`](./run.md)
- [`mr query delete`](./delete.md)
- [`mr queries list`](../queries/list.md)
