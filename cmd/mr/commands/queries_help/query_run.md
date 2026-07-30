---
outputShape: An object with `columns` (the selected column names, in SELECT order) and `rows` (one array of values per row, index-aligned with columns)
exitCodes: 0 on success; 1 on any error
relatedCmds: query run-by-name, query schema, query get
---

# Long

Execute a saved query by ID and return the result set. The query
runs against a read-only database handle: any attempt to write
(INSERT/UPDATE/DELETE/DDL) is rejected. Column names come verbatim
from the SELECT list, so use explicit column aliases (`select
count(*) as n ...`) to produce predictable ones.

The response is `{"columns": [...], "rows": [[...], ...]}`. Columns
keep the order the SELECT list names them, a repeated column name
appears twice rather than being merged, and `columns` is populated
even when no rows matched. Without `--json` the columns become the
header of a text table.

Returns `400 Bad Request` if the SQL fails to execute and `404 Not
Found` if the given ID does not exist. For templated queries, the
request body/form values are bound as named SQL parameters.

# Example

  # Run a query by ID and print a text table
  mr query run 42

  # Run and extract the first row's first column with jq
  mr query run 42 --json | jq '.rows[0][0]'

  # Address a column by name rather than by position
  mr query run 42 --json | jq '.rows[0][(.columns|index("n"))]'

  # mr-doctest: create a trivial query, run it, assert the columns and the row
  NAME="doctest-run-$$-$RANDOM"
  ID=$(mr query create --name "$NAME" --text "select 1 as x, 2 as y" --json | jq -r '.ID')
  mr query run $ID --json | jq -e '.columns == ["x","y"] and .rows[0] == [1,2]'
