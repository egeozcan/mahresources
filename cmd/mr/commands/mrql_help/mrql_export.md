---
outputShape: Raw CSV or JSON stream written to stdout (or --output file); not a table
exitCodes: 0 on success; 1 on any error
relatedCmds: mrql, mrql run, mrql explain
---

# Long

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

# Example

  # Export all resources as CSV to stdout
  mr mrql export 'type = resource'

  # Export a saved query as JSON to a file
  mr mrql export --saved my-report --format json --output report.json

  # Export a parameterized query
  mr mrql export 'type = note AND name ~ $needle' --param needle=meeting

  # mr-doctest: CSV export starts with the resource header row
  mr mrql export 'type = resource' --format csv | head -1 | grep -q '^id,name,'

  # mr-doctest: JSON export carries the entityType
  mr mrql export 'type = resource' --format json | jq -e '.entityType == "resource"'
