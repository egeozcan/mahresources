# MRQL v2 Plans

Features and improvements deferred from v1.

## Language Features

- **Aggregations / GROUP BY / COUNT()** — summary queries like `type = resource GROUP BY contentType` or `type = resource GROUP BY tags ORDER BY COUNT DESC LIMIT 10`
- **Sub-queries** — reference one query's results inside another: `type = resource AND group IN (SELECT groups WHERE category = "Active")`
- **Recursive traversal** — `ancestors.category = "Archive"` (any parent up the chain) and `descendants.tags = "photo"` (any child down the chain), using the existing CTE-based hierarchical queries
- **Perceptual hash similarity** — `SIMILAR TO resource(1234)` for finding visually similar images
- **Three-segment traversal for meta** — `parent.meta.key` and `children.meta.key` (requires parser change to allow 3 segments for parent/children.meta paths)
- **Cursor-based (keyset) pagination** — for deep result sets where OFFSET degrades

## Query Page & Editor

- **Integration into existing list views** — query bar on `/resources`, `/notes`, `/groups` pages as an alternative to filter forms, auto-scoped to that entity type
- **Global search (Cmd+K) enhancement** — accept MRQL syntax in the global search modal
- **Date-picker widget** — optional date picker in autocompletion for date fields (currently shows format hints and relative date shortcuts)
- **Query sharing** — shareable URLs already work via `?q=` param; add a "Copy link" button
- **Result export** — export query results as CSV/JSON

## Cross-Entity Improvements

- **True UNION ALL queries** — replace the current per-entity fan-out + in-memory sort with a single UNION ALL SQL query for better performance and correct deep pagination
- **Cross-entity ORDER BY on entity-specific fields** — currently only common fields (name, created, updated) are sortable globally; UNION ALL would enable sorting on any shared field

## PostgreSQL

- **Native JSON path operators** — use `jsonb_path_query` for more expressive meta queries
- **Meta field type inference** — detect whether a meta key holds numeric or string values and choose the right comparison/sort strategy automatically

## Performance

- **Query plan analysis** — warn users when a query will cause a full table scan (leading wildcard, missing FTS)
- **Result count estimation** — show approximate count before executing expensive queries
- **Query caching** — cache recent query results for repeat executions

## API

- **Server API tests for Postgres paths** — current tests only exercise SQLite; need Postgres-specific integration tests for meta JSON, FTS, and type casting
- **Streaming results** — for large result sets, stream JSON instead of buffering the entire response
- **Query explain endpoint** — `POST /v1/mrql/explain` returning the generated SQL and estimated cost
