---
title: mr search
description: Search across all entities
sidebar_label: search
---

# mr search

Search across resources, notes, groups, tags, categories, saved queries, saved MRQL queries, relation types, note types and resource categories. Results are ranked by the database's full-text score (bm25 on SQLite, `ts_rank` on PostgreSQL), and the server falls back to a LIKE scan when full-text search is disabled. `total` counts the matches the server collected, which stops at 50, and `totalCapped` is set when it did, so treat the number as a floor. There is no paging; narrow the query instead.

Use `--types` to restrict the search to a comma-separated list of entity types. The names are singular: `resource`, `note`, `group`, `tag`, `category`, `query`, `relationType`, `noteType`, `resourceCategory`, `mrqlQuery`. A name the server does not recognise is dropped, and if none of the names given survives, every type is searched. Use `--limit` to cap the number of rows returned (default 20); the server clamps it to 50.

The query string is not FTS5 syntax. Everything but letters, digits, spaces, underscore and dot is stripped before the search runs, so boolean operators match as ordinary words. A trailing `*` forces a prefix match, a leading `~` (or `~2`, `~3`) runs a fuzzy match at that edit distance, and wrapping the whole query in double quotes or prefixing it with `=` forces an exact match.

## Usage

```bash
mr search <query>
```

Positional arguments:

- `<query>`


## Examples

**Simple keyword search across all entities**

```bash
mr search "invoice"
```

**Restrict to resources only**

```bash
mr search "invoice" --types resource --json
```

**Cap results and pipe into jq to read the total**

```bash
mr search "report" --limit 5 --json | jq '.total'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--types` | string | `` | Comma-separated entity types to search (e.g. resource,note) |
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

Search response &#123;query (string), total (int), totalCapped (bool, present when total is a floor), results (array of &#123;id, type, name, score, description, url, extra&#125;)&#125;

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr mrql run`](./mrql/run.md)
- [`mr resources list`](./resources/list.md)
- [`mr notes list`](./notes/list.md)
- [`mr groups list`](./groups/list.md)
