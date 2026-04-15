---
outputShape: Array of query objects with ID, Name, Text, Template, Description, CreatedAt, UpdatedAt
exitCodes: 0 on success; 1 on any error
relatedCmds: query get, query run, queries timeline
---

# Long

List saved Queries, optionally filtered by name. Pagination is
controlled via the global `--page` flag (default page size 50). The
`--name` flag does a substring match on query names (SQL `LIKE`
under the hood). Use the global `--json` flag to retrieve the raw
array of query records for scripting; the default table output
truncates long Name/Description cells for readability.

# Example

  # List all queries (first page)
  mr queries list

  # Filter by a name substring
  mr queries list --name "count"

  # JSON + jq: print each query's ID and name
  mr queries list --json | jq -r '.[] | "\(.ID)\t\(.Name)"'

  # mr-doctest: create a uniquely-named query and verify list returns at least one match
  NAME="doctest-list-$$-$RANDOM"
  mr query create --name "$NAME" --text "select 1 as x" --json >/dev/null
  mr queries list --name "$NAME" --json | jq -e 'length >= 1'
