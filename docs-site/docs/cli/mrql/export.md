---
title: mr mrql export
description: Export MRQL query results as CSV or JSON
sidebar_label: export
---

# mr mrql export

Export MRQL query results as a downloadable CSV or JSON stream. Accepts
an inline query (positional argument, `-f <file>`, or stdin `-`), or a
saved query via `--saved <name-or-id>`. Bind any `$name` parameter
placeholders with repeatable `--param name=value` flags.

`--format csv` (the default) writes one header row plus one row per
result. The columns depend on the result mode: aggregated `GROUP BY`
emits the group keys followed by the aggregate aliases; a flat query
emits a fixed scalar column set for the entity (with `meta` as a JSON
string); a bucketed `GROUP BY` prepends the bucket-key columns to the
flat item columns. CSV export requires a single entity type — use
`--format json` for cross-entity results, which streams the exact
`/v1/mrql` response body.

Output goes to stdout unless `--output <file>` is given. Pagination flags
(`--limit`, `--buckets`, `--offset`, and the global `--page`) apply as
they do for `mrql run`. When no explicit `LIMIT` is present the server
default is applied.

## Usage

```bash
mr mrql export [query]
```

Positional arguments:

- `<query>` (optional)


## Examples

**Export all resources as CSV to stdout**

```bash
mr mrql export 'type = resource'
```

**Export a saved query as JSON to a file**

```bash
mr mrql export --saved my-report --format json --output report.json
```

**Export a parameterized query**

```bash
mr mrql export 'type = note AND name ~ $needle' --param needle=meeting
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--saved` | string | `` | Export a saved query by name or ID instead of an inline query |
| `--file` | string | `` | Read query from file |
| `--format` | string | `csv` | Export format: csv or json |
| `--output` | string | `` | Write to a file instead of stdout |
| `--param` | stringArray | `[]` | Bind a query parameter placeholder, repeatable: --param name=value |
| `--limit` | int | `0` | Items per bucket for GROUP BY, or total items for regular queries |
| `--buckets` | int | `0` | Groups per page for bucketed GROUP BY queries |
| `--offset` | int | `0` | Bucket offset for cursor-based GROUP BY pagination |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Raw CSV or JSON stream written to stdout (or --output file); not a table

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr mrql`](./index.md)
- [`mr mrql run`](./run.md)
- [`mr mrql explain`](./explain.md)
