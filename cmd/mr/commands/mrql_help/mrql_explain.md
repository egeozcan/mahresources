---
outputShape: Human-readable label headers plus interpolated SQL, or with --json the raw explain response {entityType, statements[], warnings, default_limit_applied, applied_limit}
exitCodes: 0 on success; 1 on any error
relatedCmds: mrql, mrql run, mrql export
---

# Long

Show the SQL statement(s) an MRQL query would run, without executing it.
Accepts an inline query (positional argument, `-f <file>`, or stdin `-`),
or a saved query via `--saved <name-or-id>`. Bind any `$name` parameter
placeholders with repeatable `--param name=value` flags.

The reported SQL reflects what would actually run: the default `LIMIT`
is applied (and noted on stderr), `SCOPE` is resolved, and RBAC forced
scoping for group-limited users is included. Flat single-entity queries
produce one statement; cross-entity queries produce one per entity table
(resources/notes/groups); aggregated `GROUP BY` produces one statement;
bucketed `GROUP BY` shows the key-discovery query with a note that the
per-bucket item query repeats once per group key.

By default the interpolated SQL is printed under a `-- <label> --`
header for each statement. Pass `--json` (or the global `--json`) to emit
the raw response, which additionally carries the parameterized `sql` and
its `vars` per statement.

# Example

  # Explain an inline query
  mr mrql explain 'type = resource AND fileSize > 1mb'

  # Explain a parameterized query
  mr mrql explain 'type = note AND name ~ $needle' --param needle=meeting

  # Explain a saved query as JSON
  mr mrql explain --saved my-report --json

  # mr-doctest: explain returns at least one statement
  mr mrql explain 'type = resource' --json | jq -e '.statements | length >= 1'

  # mr-doctest: a parameter binds and shows up interpolated
  mr mrql explain 'type = resource AND name ~ $n' --param n=demo --json \
    | jq -e '.statements[0].interpolated | test("demo")'
