---
outputShape: MRQL result object with entityType (string) and resources/notes/groups arrays, or a grouped result with mode + rows/groups for GROUP BY queries
exitCodes: 0 on success; 1 on any error
relatedCmds: mrql save, mrql list, query run
---

# Long

Execute a saved MRQL query by name or numeric ID. The argument is tried
as an ID first and then as a name, so a name that happens to be numeric
can still be resolved. Returns the same shape as a one-off `mrql` call:
either a standard result with an `entityType` plus the matching entity
arrays, or — for `GROUP BY` queries — a grouped result with `mode`
(`aggregated` or `bucketed`) and `rows` / `groups`.

Pagination and shaping flags (`--limit`, `--buckets`, `--offset`, plus
the global `--page`) apply to the stored query exactly as they would to
an inline `mrql` invocation. Pass `--render` to request server-side
template rendering via the `CustomMRQLResult` template. A missing ID or
name returns HTTP 404.

This is distinct from `query run`, which executes SQL-backed Query
records rather than MRQL DSL expressions.

# Example

  # Run a saved query by ID
  mr mrql run 42

  # Run by name with bucketed GROUP BY pagination
  mr mrql run "resources-by-type" --buckets 5

  # Run and extract result ids with jq
  mr mrql run "recent-photos" --json | jq -r '.resources[].ID'

  # mr-doctest: save a query, run it by id, assert the expected entityType
  NAME="doctest-mrql-run-$$-$RANDOM"
  ID=$(mr mrql save "$NAME" 'type = resource' --json | jq -r '.id')
  mr mrql run $ID --json | jq -e '.entityType == "resource"'
