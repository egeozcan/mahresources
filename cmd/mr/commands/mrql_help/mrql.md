---
exitCodes: 0 on success; 1 on any error
relatedCmds: mrql list, mrql run, query run, search
---

# Long

MRQL (Mahresources Query Language) is a small DSL for querying the
mahresources data model across Resources, Notes, and Groups. A single
expression selects an entity type and applies filters, scope, ordering,
limit, and optional `GROUP BY` aggregations — for example
`type = resource AND tags = "photo"` or
`type = resource GROUP BY contentType COUNT()`.

The top-level `mrql` command executes a one-off query supplied as a
positional argument, via `-f <file>`, or on stdin with `-`. Use the
subcommands to manage saved queries: `save` to register a named query,
`list` to discover them, `run` to execute a saved query by name or ID,
and `delete` to remove one. Saved MRQL queries differ from SQL-based
`query` records (see `query run`): MRQL is the high-level DSL, whereas
`query` executes raw read-only SQL.
