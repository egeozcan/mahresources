---
outputShape: Array of row objects; each object's keys are the query's selected column names
exitCodes: 0 on success; 1 on any error
relatedCmds: query run-by-name, query schema, query get
---

# Long

Execute a saved query by ID and return the rows as JSON. The query
runs against a read-only database handle: any attempt to write
(INSERT/UPDATE/DELETE/DDL) is rejected. Column names in the result
come verbatim from the SELECT list, so use explicit column aliases
(`select count(*) as n ...`) to produce predictable keys.

Returns `400 Bad Request` if the SQL fails to execute and `404 Not
Found` if the given ID does not exist. For templated queries, the
request body/form values are bound as named SQL parameters.

# Example

  # Run a query by ID and print the raw JSON array
  mr query run 42

  # Run and extract the first row's count column with jq
  mr query run 42 --json | jq '.[0].n'

  # mr-doctest: create a trivial query, run it, assert the expected row shape
  NAME="doctest-run-$$-$RANDOM"
  ID=$(mr query create --name "$NAME" --text "select 1 as x, 2 as y" --json | jq -r '.ID')
  mr query run $ID --json | jq -e '.[0].x == 1 and .[0].y == 2'
