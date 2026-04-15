---
title: mr search
description: Search across all entities
sidebar_label: search
---

# mr search

Search across resources, notes, and groups using the server's full-text index. Results are ranked by FTS5 score; the response reports the total number of matches so callers can decide whether to broaden the query or page.

Use `--types` to restrict to a comma-separated subset of entity types (e.g. `--types resources,notes`). Use `--limit` to cap the number of rows returned (default 20). The query string supports FTS5 syntax — phrase queries with double-quoted tokens, boolean operators, and prefix matching with `*`.

## Usage

    mr search <query>

Positional arguments:

- `<query>`


## Examples

**Simple keyword search across all entities**

    mr search "invoice"

**Restrict to resources only**

    mr search "invoice" --types resources --json

**Cap results and pipe into jq to read the total**

    mr search "report" --limit 5 --json | jq '.total'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--types` | string | `` | Comma-separated entity types to search (e.g. resources,notes) |
| `--limit` | int | `20` | Maximum number of results |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Search response {query (string), total (int), results (array of {id, type, name, score, description, url, extra})}

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr mrql run`](./mrql/run.md)
- [`mr resources list`](./resources/list.md)
- [`mr notes list`](./notes/list.md)
- [`mr groups list`](./groups/list.md)
