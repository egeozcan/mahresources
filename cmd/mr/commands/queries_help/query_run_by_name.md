---
outputShape: An object with `columns` (the selected column names, in SELECT order) and `rows` (one array of values per row, index-aligned with columns)
exitCodes: 0 on success; 1 on any error
relatedCmds: query run, query get, queries list
---

# Long

Execute a saved query by its unique `Name` instead of its numeric
ID. Same semantics as `query run`: the query runs on the connection
`-db-readonly-dsn` names, `400` when the statement fails to prepare
or execute, `500` when it fails part-way through the result rows,
`404` when the name does not resolve, and the same
`{"columns": [...], "rows": [[...], ...]}` response. Useful in
scripts where the ID is not known ahead of time but the name is a
stable contract.

Renaming a query via `query edit-name` invalidates callers that
pointed at the old name, so prefer `query run <id>` for
long-running integrations.

# Example

  # Run by name
  mr query run-by-name --name "count-resources"

  # Run by name and extract the first row's first column
  mr query run-by-name --name "count-resources" --json | jq '.rows[0][0]'

  # mr-doctest: create a named query, run it by name, verify the expected row
  NAME="doctest-runbyname-$$-$RANDOM"
  mr query create --name "$NAME" --text "select 42 as answer" --json >/dev/null
  mr query run-by-name --name "$NAME" --json | jq -e '.columns == ["answer"] and .rows[0] == [42]'
