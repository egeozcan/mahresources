---
exitCodes: 0 on success; 1 on any error
relatedCmds: mrql list, mrql run, query run, search
---

# Long

For the complete DSL syntax reference (operators, fields, GROUP BY, SCOPE, traversal), see the [MRQL Reference](https://egeozcan.github.io/mahresources/features/mrql-reference) docs-site page.

MRQL (Mahresources Query Language) is a small DSL for querying the
mahresources data model across Resources, Notes, and Groups. A single
expression selects an entity type and applies filters, scope, ordering,
limit, and optional `GROUP BY` aggregations with `HAVING` — for example
`type = resource AND tags = "photo"`,
`type = resource GROUP BY contentType COUNT()`, or
`type = resource GROUP BY hash COUNT() HAVING COUNT() > 1`. Relation
counts (`tags.count = 0`, `resources.count >= 100`) and date buckets
(`GROUP BY created.month`) are supported as dotted pseudo-fields.

The top-level `mrql` command executes a one-off query supplied as a
positional argument, via `-f <file>`, or on stdin with `-`. Use the
subcommands to manage saved queries: `save` to register a named query,
`list` to discover them, `run` to execute a saved query by name or ID,
`explain` to preview the SQL, `export` to download results as CSV/JSON,
and `delete` to remove one. Saved MRQL queries differ from SQL-based
`query` records (see `query run`): MRQL is the high-level DSL, whereas
`query` executes raw read-only SQL.

Queries may contain `$name` parameter placeholders in value positions
(for example `type = resource AND tags = $tag AND created > $since`).
Bind them with repeatable `--param name=value` flags. Values are coerced
the same way a typed literal would be (`--param since=-7d` is a relative
date; wrap in quotes like `--param n='"42"'` to force a string). Every
placeholder must be supplied; unknown `--param` names are rejected.
